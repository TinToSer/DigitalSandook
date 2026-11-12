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
	"time"

	"github.com/TinToSer/DigitalSandook/internal/fsjail"
	"github.com/TinToSer/DigitalSandook/internal/randx"
	"github.com/TinToSer/DigitalSandook/internal/store"
	"github.com/TinToSer/DigitalSandook/internal/thumb"
)

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
	if b, err := os.ReadFile(file); err == nil {
		now := time.Now()
		_ = os.Chtimes(file, now, now) // LRU for thumb.CleanCache
		sendThumb(w, b, etag)
		return
	}
	if _, err := os.Stat(file + ".bad"); err == nil {
		http.NotFound(w, r)
		return
	}
	select { // bounded CPU/RAM: queue instead of decoding everything at once
	case a.thumbSem <- struct{}{}:
		defer func() { <-a.thumbSem }()
	case <-r.Context().Done():
		return
	}
	b, err := thumb.Make(f, ext)
	_ = os.MkdirAll(dir, 0o700)
	if err != nil {
		_ = os.WriteFile(file+".bad", nil, 0o600) // don't retry broken/huge images
		http.NotFound(w, r)
		return
	}
	tmp := file + "." + randx.Token(6) + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, file)
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
