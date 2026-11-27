package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TinToSer/DigitalSandook/internal/fsjail"
	"github.com/TinToSer/DigitalSandook/internal/randx"
	"github.com/TinToSer/DigitalSandook/internal/store"
	"github.com/TinToSer/DigitalSandook/internal/thumb"
)

// thumbGroup coalesces concurrent requests for the same uncached thumbnail so
// a single image is never decoded twice in parallel.
type thumbCall struct {
	wg  sync.WaitGroup
	val []byte
	err error
}

type thumbGroup struct {
	mu sync.Mutex
	m  map[string]*thumbCall
}

func newThumbGroup() *thumbGroup { return &thumbGroup{m: map[string]*thumbCall{}} }

func (g *thumbGroup) do(key string, fn func() ([]byte, error)) ([]byte, error) {
	g.mu.Lock()
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := &thumbCall{}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		delete(g.m, key)
		g.mu.Unlock()
		c.wg.Done()
	}()
	c.val, c.err = fn()
	return c.val, c.err
}

// hThumb serves a grid-view thumbnail. Thumbnails are cached as small JPEGs
// in <data>/thumbs (never inside a shared folder), keyed by path+size+mtime so
// an edited file gets a fresh one.
func (a *App) hThumb(w http.ResponseWriter, r *http.Request, c *Ctx) {
	q := r.URL.Query()
	t, code, msg := a.target(c, q.Get("m"), q.Get("p"), store.LvlRead)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	ext := strings.ToLower(path.Ext(t.rel))
	if !thumb.Supported(ext) {
		http.NotFound(w, r)
		return
	}
	f, st, err := fsjail.OpenRegular(t.root, t.rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	if st.Size() > thumb.MaxBytes {
		http.NotFound(w, r)
		return
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%s\x00%d\x00%d\x00%d", t.m.Name, t.rel, st.Size(), st.ModTime().UnixNano(), thumb.Edge))
	key := hex.EncodeToString(sum[:])
	etag := `"` + key[:24] + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	dir := filepath.Join(a.dataDir, "thumbs", key[:2])
	file := filepath.Join(dir, key+".jpg")
	if info, err := os.Stat(file); err == nil {
		if b, err := os.ReadFile(file); err == nil {
			// Lazy LRU touch: avoid a synchronous metadata write on every scroll hit
			if time.Since(info.ModTime()) > 24*time.Hour {
				now := time.Now()
				_ = os.Chtimes(file, now, now)
			}
			sendThumb(w, b, etag)
			return
		}
	}
	if _, err := os.Stat(file + ".bad"); err == nil {
		http.NotFound(w, r)
		return
	}
	b, err := a.thumbFlight.do(key, func() ([]byte, error) {
		select { // bounded CPU/RAM: queue instead of decoding everything at once
		case a.thumbSem <- struct{}{}:
			defer func() { <-a.thumbSem }()
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		out, err := thumb.Make(f, ext)
		_ = os.MkdirAll(dir, 0o700)
		if err != nil {
			_ = os.WriteFile(file+".bad", nil, 0o600) // don't retry broken/huge images
			return nil, err
		}
		tmp := file + "." + randx.Token(6) + ".tmp"
		if os.WriteFile(tmp, out, 0o600) == nil {
			_ = os.Rename(tmp, file)
		}
		return out, nil
	})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sendThumb(w, b, etag)
}

func sendThumb(w http.ResponseWriter, b []byte, etag string) {
	h := w.Header()
	h.Set("Content-Type", "image/jpeg")
	h.Set("Cache-Control", "private, max-age=604800")
	h.Set("ETag", etag)
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Content-Length", fmt.Sprint(len(b)))
	w.Write(b)
}
