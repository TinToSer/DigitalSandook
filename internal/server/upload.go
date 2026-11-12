package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/TinToSer/DigitalSandook/internal/fsjail"
	"github.com/TinToSer/DigitalSandook/internal/randx"
	"github.com/TinToSer/DigitalSandook/internal/store"
)

const (
	chunkSize   = 8 << 20
	maxChunks   = 200000
	maxActiveUp = 256 // concurrent chunked uploads per user
	uploadIdle  = 6 * time.Hour
)

// Uploads are written to a hidden scratch dir inside the shared folder (same
// filesystem => atomic rename), so a file only appears once it is complete.
// Big files are split into chunks that the browser sends in parallel; each
// chunk is written at its offset, so order does not matter and every chunk can
// be retried or resumed independently.
type upload struct {
	id, user, mount, mountPath, dir, name, conflict string
	size, chunk                                     int64
	n                                               int
	tmp                                             string
	f                                               *os.File
	root                                            *os.Root
	mtime                                           int64

	mu   sync.Mutex
	done []bool
	got  int
	last time.Time
}

type uploads struct {
	mu sync.Mutex
	m  map[string]*upload
}

func newUploads() *uploads { return &uploads{m: map[string]*upload{}} }

func (u *upload) abort() {
	u.f.Close()
	u.root.Remove(u.tmp)
}

func (us *uploads) gc() {
	us.mu.Lock()
	defer us.mu.Unlock()
	for id, u := range us.m {
		u.mu.Lock()
		idle := time.Since(u.last) > uploadIdle
		u.mu.Unlock()
		if idle {
			u.abort()
			delete(us.m, id)
		}
	}
}

func (us *uploads) count() int {
	us.mu.Lock()
	defer us.mu.Unlock()
	return len(us.m)
}

// deadlineReader pushes the read deadline forward while data keeps flowing,
// so large uploads are never cut off but stalled clients are.
type deadlineReader struct {
	r    io.Reader
	rc   *http.ResponseController
	n    int
	idle time.Duration
}

func (d *deadlineReader) Read(p []byte) (int, error) {
	if d.n%64 == 0 {
		_ = d.rc.SetReadDeadline(time.Now().Add(d.idle))
	}
	d.n++
	return d.r.Read(p)
}

func bodyReader(w http.ResponseWriter, r *http.Request) io.Reader {
	return &deadlineReader{r: r.Body, rc: http.NewResponseController(w), idle: 60 * time.Second}
}

// ensureSubdirs creates the relative folder structure of a folder upload.
// Every segment must be a name the user could also create by hand.
func ensureSubdirs(t *target, rel string) (string, bool) {
	clean, ok := fsjail.CleanNewRel(rel)
	if !ok {
		return "", false
	}
	if clean == "/" {
		return t.rel, true
	}
	full := fsjail.Join(t.rel, clean)
	if err := t.root.MkdirAll(fsjail.OSPath(full), 0o755); err != nil {
		return "", false
	}
	return full, true
}

func parseMtime(ms int64) (time.Time, bool) {
	if ms <= 0 || ms > time.Now().Add(24*time.Hour).UnixMilli() {
		return time.Time{}, false
	}
	return time.UnixMilli(ms), true
}

// hPut: single-request upload for small files (one round trip per file).
func (a *App) hPut(w http.ResponseWriter, r *http.Request, c *Ctx) {
	q := r.URL.Query()
	t, code, msg := a.target(c, q.Get("m"), q.Get("p"), store.LvlWrite)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	name := q.Get("name")
	if !fsjail.ValidNewName(name) {
		jerr(w, 400, "invalid file name")
		return
	}
	dir, ok := ensureSubdirs(t, q.Get("rel"))
	if !ok {
		jerr(w, 400, "invalid folder path")
		return
	}
	if err := fsjail.EnsureTmp(t.root); err != nil {
		fsErr(w, err)
		return
	}
	tmp := filepath.Join(fsjail.TmpDir, randx.Token(12)+".part")
	f, err := t.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		fsErr(w, err)
		return
	}
	n, err := fsjail.CopyBuffered(f, bodyReader(w, r))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.root.Remove(tmp)
		jerr(w, 400, "upload interrupted")
		return
	}
	final, err := fsjail.Place(t.root, tmp, dir, name, q.Get("conflict"))
	if err != nil {
		t.root.Remove(tmp)
		fsErr(w, err)
		return
	}
	if ms, err := strconv.ParseInt(q.Get("mtime"), 10, 64); err == nil {
		if mt, ok := parseMtime(ms); ok {
			_ = t.root.Chtimes(filepath.Join(fsjail.OSPath(dir), final), mt, mt)
		}
	}
	a.auditf(c, "upload", "%s:%s (%d bytes)", t.m.Name, fsjail.Join(dir, final), n)
	writeJSON(w, 200, map[string]any{"name": final})
}

// hUploadInit starts a chunked upload session.
func (a *App) hUploadInit(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct {
		Mount, Path, Rel, Name, Conflict string
		Size                             int64
		Mtime                            int64
	}
	if !readJSON(w, r, &in) {
		return
	}
	t, code, msg := a.target(c, in.Mount, in.Path, store.LvlWrite)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	if !fsjail.ValidNewName(in.Name) || in.Size < 0 || in.Size > 1<<50 {
		jerr(w, 400, "invalid file")
		return
	}
	dir, ok := ensureSubdirs(t, in.Rel)
	if !ok {
		jerr(w, 400, "invalid folder path")
		return
	}
	chunk := int64(chunkSize)
	for (in.Size+chunk-1)/chunk > maxChunks {
		chunk *= 2
	}
	n := int((in.Size + chunk - 1) / chunk)
	if err := fsjail.EnsureTmp(t.root); err != nil {
		fsErr(w, err)
		return
	}
	id := randx.Token(15)
	tmp := filepath.Join(fsjail.TmpDir, id+".part")
	f, err := t.root.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		fsErr(w, err)
		return
	}
	if err := f.Truncate(in.Size); err != nil {
		f.Close()
		t.root.Remove(tmp)
		fsErr(w, err)
		return
	}
	u := &upload{id: id, user: c.User.Name, mount: t.m.Name, mountPath: t.m.Path, dir: dir, name: in.Name,
		conflict: in.Conflict, size: in.Size, chunk: chunk, n: n, tmp: tmp, f: f, root: t.root, mtime: in.Mtime,
		done: make([]bool, n), last: time.Now()}

	// count and register under one lock, so parallel requests cannot overshoot the cap
	a.ups.mu.Lock()
	active := 0
	for _, e := range a.ups.m {
		if e.user == c.User.Name {
			active++
		}
	}
	if active < maxActiveUp {
		a.ups.m[id] = u
	}
	a.ups.mu.Unlock()
	if active >= maxActiveUp {
		u.abort()
		jerr(w, 429, "too many uploads in progress")
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "chunk": chunk, "n": n})
}

func (a *App) getUpload(w http.ResponseWriter, c *Ctx, id string) *upload {
	a.ups.mu.Lock()
	u := a.ups.m[id]
	a.ups.mu.Unlock()
	if u == nil || u.user != c.User.Name {
		jerr(w, 404, "upload session not found")
		return nil
	}
	return u
}

// hUploadChunk: PUT /api/upload/{id}/{idx}
func (a *App) hUploadChunk(w http.ResponseWriter, r *http.Request, c *Ctx) {
	u := a.getUpload(w, c, r.PathValue("id"))
	if u == nil {
		return
	}
	idx, err := strconv.Atoi(r.PathValue("idx"))
	if err != nil || idx < 0 || idx >= u.n {
		jerr(w, 400, "bad chunk index")
		return
	}
	off := int64(idx) * u.chunk
	want := min(u.chunk, u.size-off)
	if r.ContentLength != want {
		jerr(w, 400, fmt.Sprintf("chunk must be %d bytes", want))
		return
	}
	n, err := fsjail.CopyBuffered(io.NewOffsetWriter(u.f, off), io.LimitReader(bodyReader(w, r), want))
	if err != nil || n != want {
		jerr(w, 400, "chunk interrupted")
		return
	}
	u.mu.Lock()
	if !u.done[idx] {
		u.done[idx] = true
		u.got++
	}
	u.last = time.Now()
	u.mu.Unlock()
	w.WriteHeader(204)
}

// hUploadStatus lets a client resume: it learns which chunks are complete.
func (a *App) hUploadStatus(w http.ResponseWriter, r *http.Request, c *Ctx) {
	u := a.getUpload(w, c, r.PathValue("id"))
	if u == nil {
		return
	}
	u.mu.Lock()
	have := []int{}
	for i, d := range u.done {
		if d {
			have = append(have, i)
		}
	}
	u.mu.Unlock()
	writeJSON(w, 200, map[string]any{"chunk": u.chunk, "n": u.n, "done": have})
}

func (a *App) hUploadFinish(w http.ResponseWriter, r *http.Request, c *Ctx) {
	u := a.getUpload(w, c, r.PathValue("id"))
	if u == nil {
		return
	}
	// claim the session exclusively so concurrent finish/abort calls cannot
	// close or delete the temp file underneath us
	a.ups.mu.Lock()
	if a.ups.m[u.id] != u {
		a.ups.mu.Unlock()
		jerr(w, 409, "upload is already being finished")
		return
	}
	u.mu.Lock()
	complete := u.got == u.n
	u.mu.Unlock()
	if !complete {
		a.ups.mu.Unlock()
		jerr(w, 409, "upload incomplete")
		return
	}
	delete(a.ups.m, u.id)
	a.ups.mu.Unlock()
	err := u.f.Sync()
	if cerr := u.f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		u.root.Remove(u.tmp)
		fsErr(w, err)
		return
	}
	// the shared folder may have been removed or re-pointed while the upload ran
	if m, ok := a.store.Mount(u.mount); !ok || m.Path != u.mountPath {
		u.root.Remove(u.tmp)
		jerr(w, 409, "the shared folder changed during the upload")
		return
	}
	// permissions may have changed while the upload ran
	if lvl := a.store.Level(c.User, u.mount, u.dir); lvl < store.LvlWrite {
		u.root.Remove(u.tmp)
		jerr(w, 403, "you no longer have permission to write here")
		return
	}
	final, err := fsjail.Place(u.root, u.tmp, u.dir, u.name, u.conflict)
	if err != nil {
		u.root.Remove(u.tmp)
		fsErr(w, err)
		return
	}
	if mt, ok := parseMtime(u.mtime); ok {
		_ = u.root.Chtimes(filepath.Join(fsjail.OSPath(u.dir), final), mt, mt)
	}
	a.auditf(c, "upload", "%s:%s (%d bytes)", u.mount, fsjail.Join(u.dir, final), u.size)
	writeJSON(w, 200, map[string]any{"name": final})
}

func (a *App) hUploadAbort(w http.ResponseWriter, r *http.Request, c *Ctx) {
	u := a.getUpload(w, c, r.PathValue("id"))
	if u == nil {
		return
	}
	a.ups.mu.Lock()
	mine := a.ups.m[u.id] == u
	if mine {
		delete(a.ups.m, u.id)
	}
	a.ups.mu.Unlock()
	if mine {
		u.abort()
	}
	w.WriteHeader(204)
}
