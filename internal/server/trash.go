package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/TinToSer/DigitalSandook/internal/fsjail"
	"github.com/TinToSer/DigitalSandook/internal/randx"
	"github.com/TinToSer/DigitalSandook/internal/store"
)

const trashRetention = 30 * 24 * time.Hour

type trashMeta struct {
	ID        string `json:"id"`
	Mount     string `json:"mount"`
	OrigDir   string `json:"origDir"`
	Name      string `json:"name"`
	Dir       bool   `json:"dir"`
	Size      int64  `json:"size"`
	DeletedAt int64  `json:"deletedAt"`
	DeletedBy string `json:"deletedBy"`
}

func (a *App) moveToTrash(t *target, name, user string) error {
	srcOS := filepath.Join(t.os(), name)
	st, err := t.root.Lstat(srcOS)
	if err != nil {
		return err
	}
	if err := t.root.MkdirAll(fsjail.TrashDir, 0o700); err != nil {
		return err
	}
	id := fmt.Sprintf("%d-%s", time.Now().Unix(), randx.Token(6))
	itemDir := filepath.Join(fsjail.TrashDir, id)
	if err := t.root.Mkdir(itemDir, 0o700); err != nil {
		return err
	}
	dstOS := filepath.Join(itemDir, "data")
	if err := t.root.Rename(srcOS, dstOS); err != nil {
		_ = t.root.RemoveAll(itemDir)
		return err
	}
	var sz int64
	if !st.IsDir() {
		sz = st.Size()
	}
	meta := trashMeta{
		ID: id, Mount: t.m.Name, OrigDir: t.rel, Name: name,
		Dir: st.IsDir(), Size: sz, DeletedAt: time.Now().Unix(), DeletedBy: user,
	}
	b, _ := json.Marshal(meta)
	f, err := t.root.OpenFile(filepath.Join(itemDir, "meta.json"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err == nil {
		_, _ = f.Write(b)
		_ = f.Close()
	}
	return nil
}

func readTrashMeta(root *os.Root, id string) (trashMeta, bool) {
	f, err := root.Open(filepath.Join(fsjail.TrashDir, id, "meta.json"))
	if err != nil {
		return trashMeta{}, false
	}
	defer f.Close()
	var m trashMeta
	if json.NewDecoder(f).Decode(&m) != nil || m.ID == "" {
		return trashMeta{}, false
	}
	return m, true
}

func validTrashID(id string) bool {
	if len(id) < 5 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func (a *App) hTrashList(w http.ResponseWriter, r *http.Request, c *Ctx) {
	mName := r.URL.Query().Get("m")
	t, code, msg := a.target(c, mName, "/", store.LvlWrite)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	out := []trashMeta{}
	d, err := t.root.Open(fsjail.TrashDir)
	if err != nil {
		writeJSON(w, 200, out)
		return
	}
	ents, _ := d.ReadDir(-1)
	d.Close()
	for _, e := range ents {
		if !e.IsDir() || !validTrashID(e.Name()) {
			continue
		}
		if meta, ok := readTrashMeta(t.root, e.Name()); ok {
			if c.User.Admin || a.store.Level(c.User, t.m.Name, meta.OrigDir) >= store.LvlWrite {
				out = append(out, meta)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DeletedAt > out[j].DeletedAt })
	writeJSONz(w, r, 200, out)
}

func (a *App) hTrashRestore(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct {
		Mount string
		IDs   []string
	}
	if !readJSON(w, r, &in) {
		return
	}
	t, code, msg := a.target(c, in.Mount, "/", store.LvlWrite)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > 1000 {
		jerr(w, 400, "nothing selected")
		return
	}
	var failed int
	for _, id := range in.IDs {
		if !validTrashID(id) {
			failed++
			continue
		}
		meta, ok := readTrashMeta(t.root, id)
		if !ok || !fsjail.ValidNewName(meta.Name) {
			failed++
			continue
		}
		origDir, ok := fsjail.CleanRel(meta.OrigDir)
		if !ok || a.store.Level(c.User, t.m.Name, origDir) < store.LvlWrite {
			failed++
			continue
		}
		_ = t.root.MkdirAll(fsjail.OSPath(origDir), 0o755)
		finalName := fsjail.UniqueName(t.root, origDir, meta.Name)
		itemDir := filepath.Join(fsjail.TrashDir, id)
		if err := t.root.Rename(filepath.Join(itemDir, "data"), filepath.Join(fsjail.OSPath(origDir), finalName)); err != nil {
			failed++
			continue
		}
		_ = t.root.RemoveAll(itemDir)
		a.auditf(c, "trash-restore", "%s:%s", t.m.Name, path.Join(origDir, finalName))
	}
	if failed > 0 {
		jerr(w, 500, fmt.Sprintf("%d item(s) could not be restored", failed))
		return
	}
	writeOK(w)
}

func (a *App) hTrashEmpty(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct {
		Mount string
		IDs   []string // empty = empty all visible trash
	}
	if !readJSON(w, r, &in) {
		return
	}
	t, code, msg := a.target(c, in.Mount, "/", store.LvlWrite)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	if len(in.IDs) == 0 {
		d, err := t.root.Open(fsjail.TrashDir)
		if err == nil {
			ents, _ := d.ReadDir(-1)
			d.Close()
			for _, e := range ents {
				if e.IsDir() && validTrashID(e.Name()) {
					if meta, ok := readTrashMeta(t.root, e.Name()); ok {
						if c.User.Admin || a.store.Level(c.User, t.m.Name, meta.OrigDir) >= store.LvlWrite {
							_ = t.root.RemoveAll(filepath.Join(fsjail.TrashDir, e.Name()))
						}
					}
				}
			}
		}
		a.auditf(c, "trash-empty", "%s", t.m.Name)
		writeOK(w)
		return
	}
	for _, id := range in.IDs {
		if !validTrashID(id) {
			continue
		}
		if meta, ok := readTrashMeta(t.root, id); ok {
			if c.User.Admin || a.store.Level(c.User, t.m.Name, meta.OrigDir) >= store.LvlWrite {
				_ = t.root.RemoveAll(filepath.Join(fsjail.TrashDir, id))
			}
		}
	}
	a.auditf(c, "trash-delete", "%s (%d items)", t.m.Name, len(in.IDs))
	writeOK(w)
}

// purgeExpiredTrash deletes recycled items older than trashRetention.
func (a *App) purgeExpiredTrash() {
	cutoff := time.Now().Add(-trashRetention).Unix()
	for _, m := range a.store.Mounts() {
		if m.ReadOnly {
			continue
		}
		root, err := a.root(m)
		if err != nil {
			continue
		}
		d, err := root.Open(fsjail.TrashDir)
		if err != nil {
			continue
		}
		ents, _ := d.ReadDir(-1)
		d.Close()
		for _, e := range ents {
			if !e.IsDir() || !validTrashID(e.Name()) {
				continue
			}
			if meta, ok := readTrashMeta(root, e.Name()); ok && meta.DeletedAt < cutoff {
				if err := root.RemoveAll(filepath.Join(fsjail.TrashDir, e.Name())); err != nil && !strings.Contains(err.Error(), "not exist") {
					log.Printf("purging trash %s/%s: %v", m.Name, e.Name(), err)
				}
			}
		}
	}
}
