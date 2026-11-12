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
	"github.com/TinToSer/DigitalSandook/internal/store"
)

type entry struct {
	Name  string `json:"n"`
	Dir   bool   `json:"d"`
	Size  int64  `json:"s"`
	Mtime int64  `json:"m"`
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
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
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
		if err := t.root.RemoveAll(filepath.Join(t.os(), n)); err != nil {
			failed++
			continue
		}
		p := fsjail.Join(t.rel, n)
		a.forgetSharing(t.m.Name, p)
		a.auditf(c, "delete", "%s:%s", in.Mount, p)
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
	if _, err := t.root.Lstat(dst); err == nil && !strings.EqualFold(in.From, in.To) {
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

// hPaste moves or copies items within one shared folder.
func (a *App) hPaste(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct {
		Mount, From, To, Op string
		Names               []string
	}
	if !readJSON(w, r, &in) {
		return
	}
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
	dst, code, msg := a.target(c, in.Mount, in.To, store.LvlWrite)
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
		// never move/copy a folder into itself (case-insensitively: on Windows
		// and macOS "/A" and "/a" are the same folder)
		if fsjail.Within(from, dst.rel) {
			failed++
			continue
		}
		if move && src.rel == dst.rel {
			continue
		}
		name := fsjail.UniqueName(dst.root, dst.rel, n)
		to := fsjail.Join(dst.rel, name)
		var err error
		if move {
			err = dst.root.Rename(fsjail.OSPath(from), fsjail.OSPath(to))
		} else {
			err = fsjail.CopyTree(dst.root, fsjail.FSPath(from), fsjail.FSPath(to))
		}
		if err != nil {
			failed++
			continue
		}
		if move {
			a.moveSharing(in.Mount, from, to)
		}
		a.auditf(c, in.Op, "%s:%s -> %s", in.Mount, from, to)
	}
	if failed > 0 {
		jerr(w, 500, fmt.Sprintf("%d item(s) could not be processed", failed))
		return
	}
	writeOK(w)
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
