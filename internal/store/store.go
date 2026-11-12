// Package store is the single source of truth for users, shared folders,
// access grants, public links and server settings. It is a small JSON file
// persisted atomically (write temp, fsync, rename, keep a .bak copy).
package store

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/TinToSer/DigitalSandook/internal/randx"
)

// Access levels. A higher level includes everything below it.
const (
	LvlRead  = 1 // browse, preview, download
	LvlWrite = 2 // upload, create, rename, move, delete, share (up to own level)
	LvlOwner = 3 // like write, plus manage/revoke any grant beneath the path
)

var (
	ErrNoUser     = errors.New("user not found")
	ErrUserExists = errors.New("user already exists")
	ErrLastAdmin  = errors.New("at least one enabled administrator must remain")
)

type User struct {
	Name       string    `json:"name"`
	Hash       string    `json:"hash"`
	Admin      bool      `json:"admin"`
	Disabled   bool      `json:"disabled"`
	MustChange bool      `json:"mustChange"`
	Created    time.Time `json:"created"`
	LastLogin  time.Time `json:"lastLogin"`
}

func (u *User) activeAdmin() bool { return u.Admin && !u.Disabled }

type Mount struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	ReadOnly bool   `json:"readOnly"`
}

type Grant struct {
	ID      string    `json:"id"`
	Mount   string    `json:"mount"`
	Path    string    `json:"path"`
	User    string    `json:"user"`
	Level   int       `json:"level"`
	By      string    `json:"by"`
	Created time.Time `json:"created"`
}

type Link struct {
	Token    string    `json:"token"`
	Mount    string    `json:"mount"`
	Path     string    `json:"path"`
	Owner    string    `json:"owner"`
	Created  time.Time `json:"created"`
	Expires  int64     `json:"expires"` // unix seconds, 0 = never
	PassHash string    `json:"passHash,omitempty"`
	Hits     int       `json:"hits"`
}

// Settings are server-wide options managed from the admin panel.
type Settings struct {
	TunnelMode  string `json:"tunnelMode"` // "", "quick" or "token"
	TunnelToken string `json:"tunnelToken,omitempty"`
	TunnelPort  int    `json:"tunnelPort,omitempty"`
	PublicURL   string `json:"publicUrl,omitempty"` // base URL used when building public links
}

type storeData struct {
	Settings Settings          `json:"settings"`
	Secret   string            `json:"secret"`
	Users    map[string]*User  `json:"users"`
	Mounts   map[string]*Mount `json:"mounts"`
	Grants   map[string]*Grant `json:"grants"`
	Links    map[string]*Link  `json:"links"`
}

type Store struct {
	mu    sync.RWMutex
	file  string
	d     storeData
	idx   map[string][]Grant // grants by grantee
	dirty bool               // in-memory changes not yet on disk (link hit counters)
}

// FileName is the name of the state file inside the data folder.
const FileName = "state.json"

// Open loads (or creates) the state file in dir, falling back to the .bak copy
// if the main file is damaged.
func Open(dir string) (*Store, error) {
	s := &Store{file: filepath.Join(dir, FileName)}
	var loaded bool
	var firstErr error
	for _, f := range []string{s.file, s.file + ".bak"} {
		b, err := os.ReadFile(f)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err == nil {
			err = json.Unmarshal(b, &s.d)
		}
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", f, err)
			}
			s.d = storeData{}
			continue
		}
		loaded = true
		break
	}
	if !loaded && firstErr != nil {
		return nil, firstErr
	}
	if s.d.Users == nil {
		s.d.Users = map[string]*User{}
	}
	if s.d.Mounts == nil {
		s.d.Mounts = map[string]*Mount{}
	}
	if s.d.Grants == nil {
		s.d.Grants = map[string]*Grant{}
	}
	if s.d.Links == nil {
		s.d.Links = map[string]*Link{}
	}
	if s.d.Secret == "" {
		s.d.Secret = randx.Token(32)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) reindex() {
	s.idx = map[string][]Grant{}
	for _, g := range s.d.Grants {
		s.idx[g.User] = append(s.idx[g.User], *g)
	}
}

func (s *Store) saveLocked() error {
	s.reindex()
	b, err := json.MarshalIndent(&s.d, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.file + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if _, err := os.Stat(s.file); err == nil {
		_ = os.Rename(s.file, s.file+".bak")
	}
	if err := os.Rename(tmp, s.file); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

// Flush writes pending in-memory changes (such as link hit counters) to disk.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return nil
	}
	return s.saveLocked()
}

func (s *Store) Secret() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, _ := base64.RawURLEncoding.DecodeString(s.d.Secret)
	return b
}

// ---- settings ----

func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.d.Settings
}

func (s *Store) SetSettings(v Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.d.Settings = v
	return s.saveLocked()
}

// ---- users ----

func (s *Store) User(name string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.d.Users[name]
	if !ok {
		return User{}, false
	}
	return *u, true
}

func (s *Store) Users() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]User, 0, len(s.d.Users))
	for _, u := range s.d.Users {
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// CreateUser adds a new user; it fails with ErrUserExists instead of
// overwriting an existing account.
func (s *Store) CreateUser(u User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.d.Users[u.Name]; ok {
		return ErrUserExists
	}
	s.d.Users[u.Name] = &u
	return s.saveLocked()
}

// UpdateUser atomically applies fn to the current stored record of a user.
// Read-modify-write under the store lock means a slow caller (e.g. one that
// hashed a password in between) can never write back a stale copy and undo a
// concurrent change such as a disable or a password reset. If fn returns an
// error nothing is changed. The change is refused with ErrLastAdmin if it
// would leave no enabled administrator.
func (s *Store) UpdateUser(name string, fn func(u *User) error) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.d.Users[name]
	if !ok {
		return User{}, ErrNoUser
	}
	u := *cur
	if err := fn(&u); err != nil {
		return User{}, err
	}
	u.Name = cur.Name // the key never changes
	if cur.activeAdmin() && !u.activeAdmin() && s.activeAdminsLocked() <= 1 {
		return User{}, ErrLastAdmin
	}
	s.d.Users[name] = &u
	return u, s.saveLocked()
}

func (s *Store) activeAdminsLocked() int {
	n := 0
	for _, u := range s.d.Users {
		if u.activeAdmin() {
			n++
		}
	}
	return n
}

// DeleteUser removes a user and every grant and link they hold.
func (s *Store) DeleteUser(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.d.Users[name]
	if !ok {
		return ErrNoUser
	}
	if u.activeAdmin() && s.activeAdminsLocked() <= 1 {
		return ErrLastAdmin
	}
	delete(s.d.Users, name)
	for id, g := range s.d.Grants {
		if g.User == name {
			delete(s.d.Grants, id)
		}
	}
	for t, l := range s.d.Links {
		if l.Owner == name {
			delete(s.d.Links, t)
		}
	}
	return s.saveLocked()
}

// ---- mounts ----

func (s *Store) Mount(name string) (Mount, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.d.Mounts[name]
	if !ok {
		return Mount{}, false
	}
	return *m, true
}

func (s *Store) Mounts() []Mount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Mount, 0, len(s.d.Mounts))
	for _, m := range s.d.Mounts {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Store) PutMount(m Mount) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.d.Mounts[m.Name] = &m
	return s.saveLocked()
}

func (s *Store) DeleteMount(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.d.Mounts, name)
	for id, g := range s.d.Grants {
		if g.Mount == name {
			delete(s.d.Grants, id)
		}
	}
	for t, l := range s.d.Links {
		if l.Mount == name {
			delete(s.d.Links, t)
		}
	}
	return s.saveLocked()
}

// ---- grants ----

// Under reports whether the clean path p equals base or lies beneath it.
func Under(base, p string) bool {
	return base == "/" || p == base || strings.HasPrefix(p, base+"/")
}

// Level returns the effective access level of u for mount/rel (rel is a clean
// "/a/b" path). Grants inherit downwards; there are no deny rules.
func (s *Store) Level(u User, mount, rel string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.d.Mounts[mount]
	if !ok {
		return 0
	}
	lvl := s.levelLocked(u.Name, mount, rel, 0)
	if m.ReadOnly && lvl > LvlRead {
		lvl = LvlRead
	}
	return lvl
}

// levelLocked follows the chain of trust: a grant only counts up to the level
// its granter still holds at the granted path. Revoking, disabling or deleting
// a user therefore also revokes everything they re-shared. Admins are the root.
func (s *Store) levelLocked(name, mount, rel string, depth int) int {
	u, ok := s.d.Users[name]
	if !ok || u.Disabled {
		return 0
	}
	if u.Admin {
		return LvlOwner
	}
	if depth >= 8 { // re-share chains deeper than this (or cycles) grant nothing
		return 0
	}
	lvl := 0
	for _, g := range s.idx[name] {
		if g.Mount != mount || g.Level <= lvl || !Under(g.Path, rel) {
			continue
		}
		lvl = max(lvl, min(g.Level, s.levelLocked(g.By, mount, g.Path, depth+1)))
	}
	return lvl
}

type RootEntry struct {
	Mount string `json:"mount"`
	Path  string `json:"path"`
	Level int    `json:"level"`
	Label string `json:"label"`
	By    string `json:"by,omitempty"`
}

// Roots lists the entry points a user can browse.
func (s *Store) Roots(u User) []RootEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []RootEntry{}
	if u.Admin {
		for _, m := range s.d.Mounts {
			l := LvlOwner
			if m.ReadOnly {
				l = LvlRead
			}
			out = append(out, RootEntry{Mount: m.Name, Path: "/", Level: l, Label: m.Name})
		}
	} else {
		for _, g := range s.idx[u.Name] {
			m, ok := s.d.Mounts[g.Mount]
			if !ok {
				continue
			}
			l := min(g.Level, s.levelLocked(g.By, g.Mount, g.Path, 1))
			if l == 0 {
				continue // the person who shared it has lost access
			}
			if m.ReadOnly {
				l = LvlRead
			}
			label := g.Mount
			if g.Path != "/" {
				label = g.Mount + g.Path
			}
			out = append(out, RootEntry{Mount: g.Mount, Path: g.Path, Level: l, Label: label, By: g.By})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

func (s *Store) Grant(id string) (Grant, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.d.Grants[id]
	if !ok {
		return Grant{}, false
	}
	return *g, true
}

func (s *Store) Grants() []Grant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Grant, 0, len(s.d.Grants))
	for _, g := range s.d.Grants {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Mount != b.Mount {
			return a.Mount < b.Mount
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.User < b.User
	})
	return out
}

// GrantsAround returns grants on exactly rel (direct) and on its ancestors.
func (s *Store) GrantsAround(mount, rel string) (direct, inherited []Grant) {
	direct, inherited = []Grant{}, []Grant{}
	for _, g := range s.Grants() {
		if g.Mount != mount {
			continue
		}
		if g.Path == rel {
			direct = append(direct, g)
		} else if Under(g.Path, rel) {
			inherited = append(inherited, g)
		}
	}
	return
}

// SetGrant creates or updates the grant for (mount, path, user).
func (s *Store) SetGrant(g Grant) (Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.d.Grants {
		if e.Mount == g.Mount && e.Path == g.Path && e.User == g.User {
			e.Level = g.Level
			return *e, s.saveLocked()
		}
	}
	g.ID = randx.Token(9)
	g.Created = time.Now()
	s.d.Grants[g.ID] = &g
	return g, s.saveLocked()
}

func (s *Store) DeleteGrant(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.d.Grants, id)
	return s.saveLocked()
}

// ---- keeping grants and links attached to files ----

// DropPath removes every grant and link on p or beneath it. It is called after
// p was deleted so that a new item created later under the same name does not
// silently inherit the old item's audience.
func (s *Store) DropPath(mount, p string) error {
	if p == "/" {
		return nil // a shared folder's root is never deleted through the app
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for id, g := range s.d.Grants {
		if g.Mount == mount && Under(p, g.Path) {
			delete(s.d.Grants, id)
			changed = true
		}
	}
	for t, l := range s.d.Links {
		if l.Mount == mount && Under(p, l.Path) {
			delete(s.d.Links, t)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}

// MovePath re-targets grants and links on from (or beneath it) to the same
// place under to, so sharing follows an item when it is renamed or moved.
func (s *Store) MovePath(mount, from, to string) error {
	if from == "/" || from == to || Under(from, to) {
		return nil // the root never moves; nothing can move into itself
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	moved := func(p string) string { return to + strings.TrimPrefix(p, from) }
	changed := false
	for _, g := range s.d.Grants {
		if g.Mount != mount || !Under(from, g.Path) {
			continue
		}
		np := moved(g.Path)
		// a stale grant for the same person at the destination is superseded
		for id, e := range s.d.Grants {
			if e != g && e.Mount == mount && e.Path == np && e.User == g.User {
				delete(s.d.Grants, id)
			}
		}
		g.Path = np
		changed = true
	}
	for _, l := range s.d.Links {
		if l.Mount == mount && Under(from, l.Path) {
			l.Path = moved(l.Path)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}

// ---- links ----

func (s *Store) Link(token string) (Link, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.d.Links[token]
	if !ok {
		return Link{}, false
	}
	return *l, true
}

func (s *Store) Links() []Link {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Link, 0, len(s.d.Links))
	for _, l := range s.d.Links {
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

func (s *Store) PutLink(l Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.d.Links[l.Token] = &l
	return s.saveLocked()
}

func (s *Store) DeleteLink(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.d.Links, token)
	return s.saveLocked()
}

// HitLink counts a download. The counter is kept in memory and written by the
// next Flush (or any other save), so downloads never wait for an fsync.
func (s *Store) HitLink(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.d.Links[token]; ok {
		l.Hits++
		s.dirty = true
	}
}
