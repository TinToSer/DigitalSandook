package server

import (
	"os"

	"github.com/TinToSer/DigitalSandook/internal/fsjail"
	"github.com/TinToSer/DigitalSandook/internal/store"
)

// target is a validated, permission-checked location inside a shared folder.
type target struct {
	m     store.Mount
	root  *os.Root
	rel   string // clean "/a/b" path inside the shared folder
	level int
}

func (t *target) os() string { return fsjail.OSPath(t.rel) }

func (a *App) root(m store.Mount) (*os.Root, error) { return a.roots.Get(m.Name, m.Path) }

// target resolves mount+path for the caller and enforces the needed level.
func (a *App) target(c *Ctx, mount, p string, need int) (*target, int, string) {
	rel, ok := fsjail.CleanRel(p)
	if !ok {
		return nil, 400, "invalid path"
	}
	m, ok := a.store.Mount(mount)
	if !ok {
		return nil, 404, "unknown folder"
	}
	lvl := a.store.Level(c.User, mount, rel)
	if lvl == 0 {
		return nil, 403, "access denied"
	}
	if lvl < need {
		if m.ReadOnly && need >= store.LvlWrite {
			return nil, 403, "this folder is read-only"
		}
		return nil, 403, "you do not have permission for this action"
	}
	root, err := a.root(m)
	if err != nil {
		return nil, 503, "shared folder is unavailable"
	}
	return &target{m: m, root: root, rel: rel, level: lvl}, 0, ""
}
