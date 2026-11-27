package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/TinToSer/DigitalSandook/internal/fsjail"
	"github.com/TinToSer/DigitalSandook/internal/randx"
	"github.com/TinToSer/DigitalSandook/internal/store"
)

const maxSaveTextBytes = 4 << 20 // 4 MiB cap for in-browser text editor

type entry struct {
	Name  string `json:"n"`
	Dir   bool   `json:"d"`
	Size  int64  `json:"s"`
	Mtime int64  `json:"m"`
}

// lessFold compares two strings case-insensitively with zero allocations on
// the common ASCII fast path.
func lessFold(a, b string) bool {
	la, lb := len(a), len(b)
	n := min(la, lb)
	for i := 0; i < n; i++ {
		ca, cb := a[i], b[i]
		if ca >= 0x80 || cb >= 0x80 {
			return strings.ToLower(a) < strings.ToLower(b)
		}
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return ca < cb
		}
	}
	return la < lb
}

// listDir returns the visible entries of a directory (dirs first, by name).
func listDir(root *os.Root, osPath string) ([]entry, error) {
	f, err := root.Open(osPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ents, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	out := make([]entry, 0, len(ents))
	for _, e := range ents {
		if fsjail.SkipEntry(e) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		en := entry{Name: e.Name(), Dir: e.IsDir(), Mtime: info.ModTime().Unix()}
		if !en.Dir {
			en.Size = info.Size()
		}
		out = append(out, en)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return lessFold(out[i].Name, out[j].Name)
	})
	return out, nil
}

// listFolder writes the listing of t, which must be a folder.
func listFolder(w http.ResponseWriter, r *http.Request, t *target, extra map[string]any) {
	st, err := t.root.Stat(t.os())
	if err != nil {
		fsErr(w, err)
		return
	}
	if !st.IsDir() {
		jerr(w, 400, "not a folder")
		return
	}
	ents, err := listDir(t.root, t.os())
	if err != nil {
		fsErr(w, err)
		return
	}
	out := map[string]any{"entries": ents}
	for k, v := range extra {
		out[k] = v
	}
	writeJSONz(w, r, 200, out)
}

func (a *App) hList(w http.ResponseWriter, r *http.Request, c *Ctx) {
	q := r.URL.Query()
	t, code, msg := a.target(c, q.Get("m"), q.Get("p"), store.LvlRead)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	listFolder(w, r, t, map[string]any{"path": t.rel, "level": t.level})
}

func (a *App) hMkdir(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct{ Mount, Path, Name string }
	if !readJSON(w, r, &in) {
		return
	}
	t, code, msg := a.target(c, in.Mount, in.Path, store.LvlWrite)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	if !fsjail.ValidNewName(in.Name) {
		jerr(w, 400, "invalid folder name")
		return
	}
	if err := t.root.Mkdir(filepath.Join(t.os(), in.Name), 0o755); err != nil {
		fsErr(w, err)
		return
	}
	a.auditf(c, "mkdir", "%s:%s", in.Mount, fsjail.Join(t.rel, in.Name))
	writeOK(w)
}

func (a *App) hDelete(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct {
		Mount, Path string
		Names       []string
		Permanent   bool
	}
	if !readJSON(w, r, &in) {
		return
	}
	t, code, msg := a.target(c, in.Mount, in.Path, store.LvlWrite)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	if len(in.Names) == 0 || len(in.Names) > 10000 {
		jerr(w, 400, "nothing to delete")
		return
	}
	var failed int
	for _, n := range in.Names {
		if !fsjail.ValidSeg(n) {
			failed++
			continue
		}
		var err error
		if in.Permanent {
			err = t.root.RemoveAll(filepath.Join(t.os(), n))
		} else {
			err = a.moveToTrash(t, n, c.User.Name)
		}
		if err != nil {
			failed++
			continue
		}
		p := fsjail.Join(t.rel, n)
		a.forgetSharing(t.m.Name, p)
		action := "trash"
		if in.Permanent {
			action = "delete"
		}
		a.auditf(c, action, "%s:%s", in.Mount, p)
	}
	if failed > 0 {
		jerr(w, 500, fmt.Sprintf("%d item(s) could not be deleted", failed))
		return
	}
	writeOK(w)
}

// forgetSharing drops grants and links of a deleted item, so that whoever
// creates something new under the same name does not inherit its audience.
func (a *App) forgetSharing(mount, p string) {
	if err := a.store.DropPath(mount, p); err != nil {
		log.Printf("dropping grants of %s:%s: %v", mount, p, err)
	}
}

// moveSharing makes grants and links follow a renamed or moved item.
func (a *App) moveSharing(mount, from, to string) {
	if err := a.store.MovePath(mount, from, to); err != nil {
		log.Printf("moving grants %s:%s -> %s: %v", mount, from, to, err)
	}
}

func (a *App) hRename(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct{ Mount, Path, From, To string }
	if !readJSON(w, r, &in) {
		return
	}
	t, code, msg := a.target(c, in.Mount, in.Path, store.LvlWrite)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	if !fsjail.ValidSeg(in.From) || !fsjail.ValidNewName(in.To) {
		jerr(w, 400, "invalid name")
		return
	}
	dst := filepath.Join(t.os(), in.To)
	// Refuse to overwrite an existing item unless it is the exact same file
	// being case-renamed on a case-insensitive filesystem.
	if _, err := t.root.Lstat(dst); err == nil && !store.PathEqual(in.From, in.To) {
		jerr(w, 409, "an item with that name already exists")
		return
	}
	if err := t.root.Rename(filepath.Join(t.os(), in.From), dst); err != nil {
		fsErr(w, err)
		return
	}
	from, to := fsjail.Join(t.rel, in.From), fsjail.Join(t.rel, in.To)
	a.moveSharing(t.m.Name, from, to)
	a.auditf(c, "rename", "%s:%s -> %s", in.Mount, from, in.To)
	writeOK(w)
}

// hPaste moves or copies items within one shared folder or across shared folders.
func (a *App) hPaste(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct {
		Mount, DestMount, From, To, Op string
		Names                          []string
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.DestMount == "" {
		in.DestMount = in.Mount
	}
	sameMount := in.Mount == in.DestMount
	move := in.Op == "move"
	srcNeed := store.LvlRead
	if move {
		srcNeed = store.LvlWrite
	}
	src, code, msg := a.target(c, in.Mount, in.From, srcNeed)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	dst, code, msg := a.target(c, in.DestMount, in.To, store.LvlWrite)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	if len(in.Names) == 0 || len(in.Names) > 10000 {
		jerr(w, 400, "nothing selected")
		return
	}
	var failed int
	for _, n := range in.Names {
		if !fsjail.ValidSeg(n) {
			failed++
			continue
		}
		from := fsjail.Join(src.rel, n)
		// Never move/copy a folder into itself (case-insensitively on same mount)
		if sameMount && fsjail.Within(from, dst.rel) {
			failed++
			continue
		}
		if sameMount && move && store.PathEqual(src.rel, dst.rel) {
			continue
		}
		name := fsjail.UniqueName(dst.root, dst.rel, n)
		to := fsjail.Join(dst.rel, name)
		var err error
		if sameMount && move {
			err = dst.root.Rename(fsjail.OSPath(from), fsjail.OSPath(to))
		} else {
			err = fsjail.CopyTreeBetween(src.root, dst.root, fsjail.FSPath(from), fsjail.FSPath(to))
			if err == nil && move {
				err = src.root.RemoveAll(fsjail.OSPath(from))
			}
		}
		if err != nil {
			failed++
			continue
		}
		if move {
			if sameMount {
				a.moveSharing(in.Mount, from, to)
			} else {
				a.forgetSharing(in.Mount, from)
			}
		}
		a.auditf(c, in.Op, "%s:%s -> %s:%s", in.Mount, from, in.DestMount, to)
	}
	if failed > 0 {
		jerr(w, 500, fmt.Sprintf("%d item(s) could not be processed", failed))
		return
	}
	writeOK(w)
}

// hSaveText saves edited text/code/markdown files atomically with optional
// optimistic-locking conflict detection via expectedMtime.
func (a *App) hSaveText(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct {
		Mount         string `json:"mount"`
		Path          string `json:"path"`
		Content       string `json:"content"`
		ExpectedMtime int64  `json:"expectedMtime"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSaveTextBytes+64<<10)
	if !readJSON(w, r, &in) {
		return
	}
	if len(in.Content) > maxSaveTextBytes {
		jerr(w, 413, "file is too large to edit in browser (max 4 MB)")
		return
	}
	t, code, msg := a.target(c, in.Mount, in.Path, store.LvlWrite)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	f, st, err := fsjail.OpenRegular(t.root, t.rel)
	if err != nil {
		fsErr(w, err)
		return
	}
	f.Close()
	if in.ExpectedMtime > 0 && st.ModTime().Unix() != in.ExpectedMtime {
		jerr(w, 409, "file was modified on disk since you opened it; reload before saving")
		return
	}
	if err := fsjail.EnsureTmp(t.root); err != nil {
		fsErr(w, err)
		return
	}
	tmp := filepath.Join(fsjail.TmpDir, randx.Token(12)+".edit")
	wf, err := t.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		fsErr(w, err)
		return
	}
	_, err = wf.WriteString(in.Content)
	if cerr := wf.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.root.Remove(tmp)
		fsErr(w, err)
		return
	}
	if err := t.root.Rename(tmp, t.os()); err != nil {
		t.root.Remove(tmp)
		fsErr(w, err)
		return
	}
	var newMtime int64
	if nst, err := t.root.Stat(t.os()); err == nil {
		newMtime = nst.ModTime().Unix()
	}
	a.auditf(c, "edit", "%s:%s (%d bytes)", t.m.Name, t.rel, len(in.Content))
	writeJSON(w, 200, map[string]any{"ok": true, "mtime": newMtime, "size": len(in.Content)})
}

func (a *App) hSearch(w http.ResponseWriter, r *http.Request, c *Ctx) {
	q := r.URL.Query()
	t, code, msg := a.target(c, q.Get("m"), q.Get("p"), store.LvlRead)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	term := strings.ToLower(strings.TrimSpace(q.Get("q")))
	if len(term) < 2 {
		jerr(w, 400, "type at least 2 characters")
		return
	}
	select {
	case a.searchSem <- struct{}{}:
		defer func() { <-a.searchSem }()
	default:
		jerr(w, 429, "search is busy, try again")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	type hit struct {
		Path string `json:"p"`
		entry
	}
	hits := []hit{}
	stop := errors.New("stop")
	base := fsjail.FSPath(t.rel)
	err := fs.WalkDir(t.root.FS(), base, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return stop
		}
		if err != nil {
			return nil
		}
		if fsjail.SkipEntry(d) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if p != base && strings.Contains(strings.ToLower(d.Name()), term) {
			info, ierr := d.Info()
			if ierr == nil {
				h := hit{Path: "/" + p, entry: entry{Name: d.Name(), Dir: d.IsDir(), Mtime: info.ModTime().Unix()}}
				if !h.Dir {
					h.Size = info.Size()
				}
				hits = append(hits, h)
			}
			if len(hits) >= 300 {
				return stop
			}
		}
		return nil
	})
	writeJSONz(w, r, 200, map[string]any{"hits": hits, "truncated": errors.Is(err, stop)})
}
