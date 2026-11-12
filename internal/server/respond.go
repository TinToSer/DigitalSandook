package server

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"syscall"
	"time"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeJSONz gzips large responses (big folder listings, search, audit log),
// typically shrinking them 8-10x on slow links.
func writeJSONz(w http.ResponseWriter, r *http.Request, code int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		jerr(w, 500, "internal error")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Add("Vary", "Accept-Encoding")
	if len(b) < 8<<10 || !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.WriteHeader(code)
		w.Write(b)
		return
	}
	h.Set("Content-Encoding", "gzip")
	w.WriteHeader(code)
	zw, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
	zw.Write(b)
	zw.Close()
}

func jerr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeOK(w http.ResponseWriter) { writeJSON(w, 200, map[string]bool{"ok": true}) }

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		jerr(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func fsErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		jerr(w, 404, "not found")
	case errors.Is(err, fs.ErrExist):
		jerr(w, 409, "already exists")
	case errors.Is(err, fs.ErrPermission):
		jerr(w, 403, "permission denied by the operating system")
	case errors.Is(err, syscall.ENOSPC):
		jerr(w, 507, "not enough disk space")
	case strings.Contains(err.Error(), "escapes"):
		jerr(w, 403, "path not allowed")
	default:
		log.Printf("fs error: %v", err)
		jerr(w, 500, "operation failed")
	}
}

// acquire takes a slot of sem, giving up after wait or when the client leaves.
func acquire(ctx context.Context, sem chan struct{}, wait time.Duration) (release func(), ok bool) {
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case sem <- struct{}{}:
		return func() { <-sem }, true
	case <-t.C:
	case <-ctx.Done():
	}
	return nil, false
}

// hashSlot reserves CPU for one password hash. It never waits unboundedly, so
// a flood of (possibly anonymous) password attempts cannot pile up goroutines
// or starve legitimate logins; the caller gets a 503 instead.
func (a *App) hashSlot(w http.ResponseWriter, r *http.Request) (release func(), ok bool) {
	release, ok = acquire(r.Context(), a.hashSem, 5*time.Second)
	if !ok {
		w.Header().Set("Retry-After", "5")
		jerr(w, 503, "server busy, try again")
	}
	return
}
