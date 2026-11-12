package server

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TinToSer/DigitalSandook/internal/auth"
	"github.com/TinToSer/DigitalSandook/internal/store"
)

var mountRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`)

func within(base, p string) bool {
	rel, err := filepath.Rel(base, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// addMount validates and registers a local folder as a shared folder.
func (a *App) addMount(name, p string, readOnly bool) error {
	if !mountRe.MatchString(name) {
		return errors.New("folder name must be 1-32 letters, digits, '-' or '_'")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("cannot access %q: %w", p, err)
	}
	st, err := os.Stat(real)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("%q is not a folder", p)
	}
	// never expose the application's own state (password hashes, TLS key)
	if dd, err := filepath.EvalSymlinks(a.dataDir); err == nil {
		if within(dd, real) || within(real, dd) {
			return errors.New("a shared folder cannot contain or live inside the " + AppName + " data folder")
		}
	}
	root, err := os.OpenRoot(real)
	if err != nil {
		return err
	}
	root.Close()
	if err := a.store.PutMount(store.Mount{Name: name, Path: real, ReadOnly: readOnly}); err != nil {
		return err
	}
	a.roots.Forget(name)
	return nil
}

// userErr maps store errors of user updates to responses.
func userErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNoUser):
		jerr(w, 404, "user not found")
	case errors.Is(err, store.ErrUserExists):
		jerr(w, 409, "user already exists")
	case errors.Is(err, store.ErrLastAdmin):
		jerr(w, 400, "at least one enabled administrator must remain")
	default:
		jerr(w, 500, "could not save")
	}
}

// ---------- users ----------

type adminUser struct {
	Name      string `json:"name"`
	Admin     bool   `json:"admin"`
	Disabled  bool   `json:"disabled"`
	Created   int64  `json:"created"`
	LastLogin int64  `json:"lastLogin"`
}

func (a *App) hAdminUsers(w http.ResponseWriter, r *http.Request, c *Ctx) {
	out := []adminUser{}
	for _, u := range a.store.Users() {
		out = append(out, adminUser{u.Name, u.Admin, u.Disabled, u.Created.Unix(), u.LastLogin.Unix()})
	}
	writeJSON(w, 200, out)
}

func (a *App) hAdminUserCreate(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct {
		Name, Password string
		Admin          bool
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	if !auth.ValidUsername(in.Name) {
		jerr(w, 400, "username: 2-32 chars, lowercase letters, digits, '.', '_' or '-'")
		return
	}
	if _, exists := a.store.User(in.Name); exists {
		jerr(w, 409, "user already exists")
		return
	}
	generated := ""
	if in.Password == "" {
		in.Password = auth.RandomPassword()
		generated = in.Password
	}
	if m := auth.CheckPasswordPolicy(in.Password); m != "" {
		jerr(w, 400, m)
		return
	}
	release, ok := a.hashSlot(w, r)
	if !ok {
		return
	}
	hash := auth.HashPassword(in.Password)
	release()
	u := store.User{Name: in.Name, Hash: hash, Admin: in.Admin, MustChange: true, Created: time.Now()}
	if err := a.store.CreateUser(u); err != nil {
		userErr(w, err)
		return
	}
	a.auditf(c, "user-create", "%s admin=%v", u.Name, u.Admin)
	writeJSON(w, 200, map[string]any{"name": u.Name, "password": generated})
}

func (a *App) hAdminUserPatch(w http.ResponseWriter, r *http.Request, c *Ctx) {
	name := r.PathValue("name")
	if _, ok := a.store.User(name); !ok {
		jerr(w, 404, "user not found")
		return
	}
	var in struct {
		Password *string
		Generate bool
		Admin    *bool
		Disabled *bool
	}
	if !readJSON(w, r, &in) {
		return
	}
	self := name == c.User.Name
	if self && ((in.Admin != nil && !*in.Admin) || (in.Disabled != nil && *in.Disabled)) {
		jerr(w, 400, "you cannot remove your own admin rights or disable yourself")
		return
	}
	var newPw string
	if in.Generate {
		newPw = auth.RandomPassword()
	} else if in.Password != nil {
		newPw = *in.Password
	}
	newHash := ""
	if newPw != "" {
		if m := auth.CheckPasswordPolicy(newPw); m != "" {
			jerr(w, 400, m)
			return
		}
		release, ok := a.hashSlot(w, r)
		if !ok {
			return
		}
		newHash = auth.HashPassword(newPw)
		release()
	}
	// apply only the requested fields to the current record (no stale write-back)
	u, err := a.store.UpdateUser(name, func(u *store.User) error {
		if newHash != "" {
			u.Hash, u.MustChange = newHash, true
		}
		if in.Admin != nil {
			u.Admin = *in.Admin
		}
		if in.Disabled != nil {
			u.Disabled = *in.Disabled
		}
		return nil
	})
	if err != nil {
		userErr(w, err)
		return
	}
	if newHash != "" || u.Disabled {
		a.sess.DropUser(name, nil)
	}
	a.auditf(c, "user-update", "%s admin=%v disabled=%v pwreset=%v", name, u.Admin, u.Disabled, newHash != "")
	resp := map[string]any{"ok": true}
	if in.Generate {
		resp["password"] = newPw
	}
	writeJSON(w, 200, resp)
}

func (a *App) hAdminUserDelete(w http.ResponseWriter, r *http.Request, c *Ctx) {
	name := r.PathValue("name")
	if name == c.User.Name {
		jerr(w, 400, "you cannot delete yourself")
		return
	}
	if err := a.store.DeleteUser(name); err != nil {
		userErr(w, err)
		return
	}
	a.sess.DropUser(name, nil)
	a.auditf(c, "user-delete", "%s", name)
	writeOK(w)
}

// ---------- shared folders ----------

func (a *App) hAdminMounts(w http.ResponseWriter, r *http.Request, c *Ctx) {
	type mv struct {
		store.Mount
		Online bool `json:"online"`
	}
	out := []mv{}
	for _, m := range a.store.Mounts() {
		_, err := a.root(m)
		out = append(out, mv{m, err == nil})
	}
	writeJSON(w, 200, out)
}

func (a *App) hAdminMountSet(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct {
		Name, Path string
		ReadOnly   bool
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if err := a.addMount(in.Name, strings.TrimSpace(in.Path), in.ReadOnly); err != nil {
		jerr(w, 400, err.Error())
		return
	}
	a.auditf(c, "mount-set", "%s = %s ro=%v", in.Name, in.Path, in.ReadOnly)
	writeOK(w)
}

func (a *App) hAdminMountDelete(w http.ResponseWriter, r *http.Request, c *Ctx) {
	name := r.PathValue("name")
	if _, ok := a.store.Mount(name); !ok {
		jerr(w, 404, "not found")
		return
	}
	if err := a.store.DeleteMount(name); err != nil {
		jerr(w, 500, "could not save")
		return
	}
	a.roots.Forget(name)
	a.auditf(c, "mount-delete", "%s", name)
	writeOK(w)
}

// hAdminBrowse lists local sub-folders so an admin can pick a folder to share.
func (a *App) hAdminBrowse(w http.ResponseWriter, r *http.Request, c *Ctx) {
	p := r.URL.Query().Get("path")
	dirs := []string{}
	if p == "" {
		if runtime.GOOS == "windows" {
			for ch := 'A'; ch <= 'Z'; ch++ {
				d := string(ch) + `:\`
				if _, err := os.Stat(d); err == nil {
					dirs = append(dirs, d)
				}
			}
			writeJSON(w, 200, map[string]any{"path": "", "parent": "", "dirs": dirs, "root": true})
			return
		}
		p = "/"
	}
	if !filepath.IsAbs(p) {
		jerr(w, 400, "path must be absolute")
		return
	}
	p = filepath.Clean(p)
	ents, err := os.ReadDir(p)
	if err != nil {
		jerr(w, 400, "cannot read that folder")
		return
	}
	for _, e := range ents {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i]) < strings.ToLower(dirs[j]) })
	parent := filepath.Dir(p)
	if parent == p {
		parent = ""
	}
	writeJSON(w, 200, map[string]any{"path": p, "parent": parent, "dirs": dirs})
}

// ---------- audit & sessions ----------

func (a *App) hAdminAudit(w http.ResponseWriter, r *http.Request, c *Ctx) {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n <= 0 || n > 2000 {
		n = 300
	}
	e := a.aud.Tail(n)
	if e == nil {
		writeJSON(w, 200, []any{})
		return
	}
	writeJSONz(w, r, 200, e)
}

func (a *App) hAdminSessions(w http.ResponseWriter, r *http.Request, c *Ctx) {
	type sv struct {
		ID      string `json:"id"`
		User    string `json:"user"`
		IP      string `json:"ip"`
		UA      string `json:"ua"`
		Created int64  `json:"created"`
		Last    int64  `json:"last"`
		Current bool   `json:"current"`
	}
	list := a.sess.List()
	sort.Slice(list, func(i, j int) bool { return list[i].Last.After(list[j].Last) })
	out := []sv{}
	for _, s := range list {
		out = append(out, sv{s.ID, s.User, s.IP, s.UA, s.Created.Unix(), s.Last.Unix(), s.ID == c.Sess.ID})
	}
	writeJSON(w, 200, out)
}

func (a *App) hAdminSessionKick(w http.ResponseWriter, r *http.Request, c *Ctx) {
	s, found := a.sess.DropID(r.PathValue("id"))
	if !found {
		jerr(w, 404, "not found")
		return
	}
	a.auditf(c, "session-end", "%s (%s)", s.User, s.IP)
	writeOK(w)
}

func (a *App) hAdminStatus(w http.ResponseWriter, r *http.Request, c *Ctx) {
	writeJSON(w, 200, map[string]any{
		"version": a.cfg.Version, "platform": hostOS(), "uptime": int64(time.Since(a.started).Seconds()),
		"tls": a.tlsOn, "uploads": a.ups.count(), "sessions": a.sess.Len(),
	})
}
