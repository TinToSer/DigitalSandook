// Package fsjail confines every file operation to a shared folder.
//
// Each shared folder is opened as an os.Root, which enforces in the kernel
// (openat / RESOLVE_BENEATH style) that no operation can escape the folder via
// "..", absolute paths, symlinks or junctions, even through races. On top of
// that the package validates client-supplied names and implements the
// higher-level operations (atomic placement, recursive copy) the server needs.
package fsjail

import (
	"os"
	"sync"
	"time"
)

const revalidateEvery = 3 * time.Second

// RootCache keeps one os.Root per shared folder.
//
// Roots that are replaced (the folder was remounted, or its path changed) are
// simply dropped from the cache, never closed explicitly: a long ZIP download
// or a chunked upload may still be using them, and os.Root closes its handle
// from a finalizer once the last user is gone.
type RootCache struct {
	mu sync.Mutex
	m  map[string]*rootEntry
}

type rootEntry struct {
	path    string      // immutable
	root    *os.Root    // immutable
	info    os.FileInfo // immutable
	checked time.Time   // guarded by RootCache.mu
}

func NewRootCache() *RootCache { return &RootCache{m: map[string]*rootEntry{}} }

// Get returns the root for shared folder `name` located at dir. Slow file
// system calls (stat of a network share, opening the root) happen outside the
// cache lock, so one unresponsive share cannot stall every other request.
func (c *RootCache) Get(name, dir string) (*os.Root, error) {
	c.mu.Lock()
	e := c.m[name]
	if e != nil && e.path == dir && time.Since(e.checked) < revalidateEvery {
		c.mu.Unlock()
		return e.root, nil
	}
	c.mu.Unlock()

	if e != nil && e.path == dir {
		// re-validate: the folder may have been replaced or remounted
		if st, err := os.Stat(dir); err == nil && os.SameFile(st, e.info) {
			c.mu.Lock()
			e.checked = time.Now()
			c.mu.Unlock()
			return e.root, nil
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		c.Forget(name)
		return nil, err
	}
	st, err := root.Stat(".")
	if err != nil {
		root.Close()
		return nil, err
	}
	c.mu.Lock()
	c.m[name] = &rootEntry{path: dir, root: root, info: st, checked: time.Now()}
	c.mu.Unlock()
	return root, nil
}

// Forget drops the cached root of a shared folder (see RootCache on closing).
func (c *RootCache) Forget(name string) {
	c.mu.Lock()
	delete(c.m, name)
	c.mu.Unlock()
}

// Close closes every cached root. Use it only at shutdown.
func (c *RootCache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, e := range c.m {
		e.root.Close()
		delete(c.m, name)
	}
}
