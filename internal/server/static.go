package server

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/TinToSer/DigitalSandook/web"
)

type asset struct {
	raw, gz []byte
	etag    string
	ctype   string
}

// staticSite serves the embedded UI: pre-compressed once at startup, strong
// ETags, no per-request work beyond a map lookup.
type staticSite struct{ files map[string]*asset }

func newStatic() (*staticSite, error) {
	s := &staticSite{files: map[string]*asset{}}
	err := fs.WalkDir(web.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := web.FS.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		a := &asset{raw: b, etag: `"` + hex.EncodeToString(sum[:8]) + `"`}
		a.ctype = mime.TypeByExtension(path.Ext(p))
		switch path.Ext(p) {
		case ".js":
			a.ctype = "text/javascript; charset=utf-8"
		case ".css":
			a.ctype = "text/css; charset=utf-8"
		case ".html":
			a.ctype = "text/html; charset=utf-8"
		case ".svg":
			a.ctype = "image/svg+xml"
		}
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		zw.Write(b)
		zw.Close()
		if buf.Len() < len(b) {
			a.gz = buf.Bytes()
		}
		s.files[p] = a
		return nil
	})
	return s, err
}

func (s *staticSite) serve(w http.ResponseWriter, r *http.Request, name string) {
	a := s.files[name]
	if a == nil {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", a.ctype)
	h.Set("ETag", a.etag)
	h.Set("Cache-Control", "no-cache")
	h.Add("Vary", "Accept-Encoding")
	if r.Header.Get("If-None-Match") == a.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	body := a.raw
	if a.gz != nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		h.Set("Content-Encoding", "gzip")
		body = a.gz
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method != http.MethodHead {
		w.Write(body)
	}
}
