package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/TinToSer/DigitalSandook/internal/auth"
	"github.com/TinToSer/DigitalSandook/internal/fsjail"
	"github.com/TinToSer/DigitalSandook/internal/randx"
	"github.com/TinToSer/DigitalSandook/internal/store"
)

const maxDropUploadSize = 100 << 20 // 100 MiB per file on anonymous file-drop links

// ---------- user-to-user grants ----------

// mountPath validates an existing item for sharing and returns the caller's
// level there (admins always manage access, even on read-only folders).
func (a *App) mountPath(c *Ctx, mount, p string) (rel string, lvl int, code int, msg string) {
	rel, ok := fsjail.CleanRel(p)
	if !ok {
		return "", 0, 400, "invalid path"
	}
	m, ok := a.store.Mount(mount)
	if !ok {
		return "", 0, 404, "unknown folder"
	}
	root, err := a.root(m)
	if err != nil {
		return "", 0, 503, "shared folder is unavailable"
	}
	if _, err := root.Stat(fsjail.OSPath(rel)); err != nil {
		return "", 0, 404, "not found"
	}
	lvl = a.store.Level(c.User, mount, rel)
	if c.User.Admin {
		lvl = store.LvlOwner
	}
	return rel, lvl, 0, ""
}

func (a *App) hGrantsGet(w http.ResponseWriter, r *http.Request, c *Ctx) {
	q := r.URL.Query()
	rel, lvl, code, msg := a.mountPath(c, q.Get("m"), q.Get("p"))
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	if lvl < store.LvlWrite {
		jerr(w, 403, "you cannot manage sharing here")
		return
	}
	direct, inherited := a.store.GrantsAround(q.Get("m"), rel)
	writeJSON(w, 200, map[string]any{"direct": direct, "inherited": inherited, "level": lvl})
}

func (a *App) hGrantSet(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct {
		Mount, Path, User string
		Level             int
	}
	if !readJSON(w, r, &in) {
		return
	}
	rel, lvl, code, msg := a.mountPath(c, in.Mount, in.Path)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	if lvl < store.LvlWrite {
		jerr(w, 403, "you cannot share this")
		return
	}
	if in.Level < store.LvlRead || in.Level > store.LvlOwner || (in.Level > lvl && !c.User.Admin) {
		jerr(w, 403, "you cannot grant more access than you have")
		return
	}
	in.User = strings.ToLower(strings.TrimSpace(in.User))
	grantee, ok := a.store.User(in.User)
	if !ok || grantee.Disabled {
		jerr(w, 404, "user not found")
		return
	}
	if in.User == c.User.Name {
		jerr(w, 400, "you cannot share with yourself")
		return
	}
	if !c.User.Admin && lvl < store.LvlOwner {
		for _, g := range a.store.Grants() {
			if g.Mount == in.Mount && store.PathEqual(g.Path, rel) && g.User == in.User && g.By != c.User.Name {
				jerr(w, 403, "this person's access was set by someone else")
				return
			}
		}
	}
	g, err := a.store.SetGrant(store.Grant{Mount: in.Mount, Path: rel, User: in.User, Level: in.Level, By: c.User.Name})
	if err != nil {
		jerr(w, 500, "could not save")
		return
	}
	a.auditf(c, "grant", "%s:%s -> %s level %d", in.Mount, rel, in.User, in.Level)
	writeJSON(w, 200, g)
}

func (a *App) hGrantDelete(w http.ResponseWriter, r *http.Request, c *Ctx) {
	g, ok := a.store.Grant(r.PathValue("id"))
	if !ok {
		jerr(w, 404, "not found")
		return
	}
	allowed := c.User.Admin || g.By == c.User.Name || g.User == c.User.Name ||
		a.store.Level(c.User, g.Mount, g.Path) >= store.LvlOwner
	if !allowed {
		jerr(w, 403, "you cannot revoke this")
		return
	}
	if err := a.store.DeleteGrant(g.ID); err != nil {
		jerr(w, 500, "could not save")
		return
	}
	a.auditf(c, "revoke", "%s:%s from %s", g.Mount, g.Path, g.User)
	writeOK(w)
}

// ---------- public links ----------

type linkView struct {
	Token      string `json:"token"`
	Mount      string `json:"mount"`
	Path       string `json:"path"`
	Owner      string `json:"owner"`
	Created    int64  `json:"created"`
	Expires    int64  `json:"expires"`
	Protected  bool   `json:"protected"`
	Hits       int    `json:"hits"`
	MaxHits    int    `json:"maxHits,omitempty"`
	UploadOnly bool   `json:"uploadOnly,omitempty"`
}

func toLinkView(l store.Link) linkView {
	return linkView{l.Token, l.Mount, l.Path, l.Owner, l.Created.Unix(), l.Expires, l.PassHash != "", l.Hits, l.MaxHits, l.UploadOnly}
}

func (a *App) hLinksGet(w http.ResponseWriter, r *http.Request, c *Ctx) {
	q := r.URL.Query()
	rel, lvl, code, msg := a.mountPath(c, q.Get("m"), q.Get("p"))
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	if lvl < store.LvlWrite {
		jerr(w, 403, "you cannot manage sharing here")
		return
	}
	out := []linkView{}
	for _, l := range a.store.Links() {
		if l.Mount == q.Get("m") && store.PathEqual(l.Path, rel) {
			out = append(out, toLinkView(l))
		}
	}
	writeJSON(w, 200, out)
}

func (a *App) hLinkCreate(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct {
		Mount, Path, Password string
		Days                  int
		MaxHits               int
		UploadOnly            bool
	}
	if !readJSON(w, r, &in) {
		return
	}
	rel, lvl, code, msg := a.mountPath(c, in.Mount, in.Path)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	if lvl < store.LvlWrite {
		jerr(w, 403, "you cannot share this")
		return
	}
	if in.Days < 0 || in.Days > 3650 || in.MaxHits < 0 || in.MaxHits > 1_000_000 || len(in.Password) > 256 {
		jerr(w, 400, "invalid options")
		return
	}
	if in.UploadOnly {
		m, _ := a.store.Mount(in.Mount)
		if m.ReadOnly {
			jerr(w, 400, "cannot create a file-drop link on a read-only folder")
			return
		}
		root, err := a.root(m)
		if err != nil {
			jerr(w, 503, "shared folder is unavailable")
			return
		}
		if st, err := root.Stat(fsjail.OSPath(rel)); err != nil || !st.IsDir() {
			jerr(w, 400, "file-drop links can only target a folder")
			return
		}
	}
	l := store.Link{
		Token: randx.Token(24), Mount: in.Mount, Path: rel, Owner: c.User.Name,
		Created: time.Now(), MaxHits: in.MaxHits, UploadOnly: in.UploadOnly,
	}
	if in.Days > 0 {
		l.Expires = time.Now().Add(time.Duration(in.Days) * 24 * time.Hour).Unix()
	}
	if in.Password != "" {
		if len(in.Password) < 6 {
			jerr(w, 400, "link password must be at least 6 characters")
			return
		}
		release, ok := a.hashSlot(w, r)
		if !ok {
			return
		}
		l.PassHash = auth.HashPassword(in.Password)
		release()
	}
	if err := a.store.PutLink(l); err != nil {
		jerr(w, 500, "could not save")
		return
	}
	a.auditf(c, "link-create", "%s:%s uploadOnly=%v maxHits=%d", in.Mount, rel, l.UploadOnly, l.MaxHits)
	writeJSON(w, 200, toLinkView(l))
}

func (a *App) hLinkDelete(w http.ResponseWriter, r *http.Request, c *Ctx) {
	l, ok := a.store.Link(r.PathValue("token"))
	if !ok {
		jerr(w, 404, "not found")
		return
	}
	if !(c.User.Admin || l.Owner == c.User.Name || a.store.Level(c.User, l.Mount, l.Path) >= store.LvlOwner) {
		jerr(w, 403, "you cannot remove this link")
		return
	}
	if err := a.store.DeleteLink(l.Token); err != nil {
		jerr(w, 500, "could not save")
		return
	}
	a.auditf(c, "link-delete", "%s:%s", l.Mount, l.Path)
	writeOK(w)
}

func (a *App) hAdminLinks(w http.ResponseWriter, r *http.Request, c *Ctx) {
	out := []linkView{}
	for _, l := range a.store.Links() {
		out = append(out, toLinkView(l))
	}
	writeJSON(w, 200, out)
}

func (a *App) hAdminGrants(w http.ResponseWriter, r *http.Request, c *Ctx) {
	writeJSON(w, 200, a.store.Grants())
}

// ---------- anonymous access through a link ----------

const unlockTTL = 8 * time.Hour

// unlockValue signs "this browser knew the link password" with an expiry, so
// a stolen cookie value stops working after unlockTTL. Changing the password
// (PassHash) invalidates it immediately.
func (a *App) unlockValue(l store.Link, exp int64) string {
	m := hmac.New(sha256.New, a.store.Secret())
	fmt.Fprintf(m, "link:%s:%s:%d", l.Token, l.PassHash, exp)
	return strconv.FormatInt(exp, 10) + "." + hex.EncodeToString(m.Sum(nil))
}

func (a *App) unlocked(r *http.Request, l store.Link) bool {
	if l.PassHash == "" {
		return true
	}
	ck, err := r.Cookie(unlockCookie(l.Token))
	if err != nil {
		return false
	}
	expStr, _, ok := strings.Cut(ck.Value, ".")
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if !ok || err != nil || time.Now().Unix() > exp {
		return false
	}
	return auth.ConstantTimeEqual(ck.Value, a.unlockValue(l, exp))
}

func unlockCookie(token string) string { return "dsd_l_" + token[:min(8, len(token))] }

func (a *App) pubSameOrigin(r *http.Request) bool {
	if viaTunnel(r) {
		sf := r.Header.Get("Sec-Fetch-Site")
		return sf == "" || sf == "same-origin" || sf == "none"
	}
	return a.sameOrigin(r)
}

type pubHandler func(w http.ResponseWriter, r *http.Request, l store.Link, m store.Mount, c *Ctx)

// pub resolves a share link, enforces expiry/max-hits/owner validity, and
// (unless open=true) the link password.
func (a *App) pub(h pubHandler, open bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l, ok := a.store.Link(r.PathValue("token"))
		if !ok || (l.Expires > 0 && time.Now().Unix() > l.Expires) || (l.MaxHits > 0 && l.Hits >= l.MaxHits) {
			jerr(w, 404, "this link is invalid or has expired")
			return
		}
		owner, ok := a.store.User(l.Owner)
		m, mok := a.store.Mount(l.Mount)
		needLvl := store.LvlRead
		if l.UploadOnly {
			needLvl = store.LvlWrite
		}
		if !ok || !mok || owner.Disabled || a.store.Level(owner, l.Mount, l.Path) < needLvl {
			jerr(w, 404, "this link is invalid or has expired")
			return
		}
		if !open && !a.unlocked(r, l) {
			writeJSON(w, 401, map[string]any{"error": "password required", "needsPassword": true})
			return
		}
		h(w, r, l, m, &Ctx{User: store.User{Name: "link:" + l.Owner}, IP: a.clientIP(r)})
	}
}

func (a *App) hPubInfo(w http.ResponseWriter, r *http.Request, l store.Link, m store.Mount, c *Ctx) {
	// Never leak filename, directory status or expiry before the password is verified
	if !a.unlocked(r, l) {
		writeJSON(w, 200, map[string]any{"needsPassword": true, "uploadOnly": l.UploadOnly})
		return
	}
	isDir := false
	if root, err := a.root(m); err == nil {
		if st, err := root.Stat(fsjail.OSPath(l.Path)); err == nil {
			isDir = st.IsDir()
		}
	}
	name := path.Base(l.Path)
	if l.Path == "/" {
		name = m.Name
	}
	writeJSON(w, 200, map[string]any{
		"name": name, "isDir": isDir, "needsPassword": false,
		"expires": l.Expires, "uploadOnly": l.UploadOnly,
		"hits": l.Hits, "maxHits": l.MaxHits,
	})
}

func (a *App) hPubUnlock(w http.ResponseWriter, r *http.Request, l store.Link, m store.Mount, c *Ctx) {
	if !a.pubSameOrigin(r) {
		jerr(w, 403, "request rejected")
		return
	}
	if l.PassHash == "" { // nothing to unlock; don't burn a hash on it
		writeOK(w)
		return
	}
	var in struct{ Password string }
	if !readJSON(w, r, &in) {
		return
	}
	ipKey := "lp:" + c.IP + ":" + l.Token
	tokKey := "lpt:" + l.Token
	if d := max(a.lim.Wait(ipKey), a.lim.Wait(tokKey)); d > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		jerr(w, 429, "too many attempts, try again later")
		return
	}
	release, ok := a.hashSlot(w, r)
	if !ok {
		return
	}
	good := len(in.Password) <= 1024 && auth.VerifyPassword(in.Password, l.PassHash)
	release()
	if !good {
		a.lim.Fail(ipKey, 5, 30*time.Second, 15*time.Minute)
		a.lim.Fail(tokKey, 30, 15*time.Second, 10*time.Minute)
		jerr(w, 403, "wrong password")
		return
	}
	a.lim.Reset(ipKey)
	exp := time.Now().Add(unlockTTL).Unix()
	http.SetCookie(w, &http.Cookie{
		Name: unlockCookie(l.Token), Value: a.unlockValue(l, exp), Path: "/api/public/" + l.Token,
		MaxAge: int(unlockTTL.Seconds()), HttpOnly: true, Secure: a.isSecure(r), SameSite: http.SameSiteStrictMode,
	})
	writeOK(w)
}

// pubTarget maps a path inside the link to a read-only target.
func (a *App) pubTarget(w http.ResponseWriter, r *http.Request, l store.Link, m store.Mount) *target {
	sub, ok := fsjail.CleanRel(r.URL.Query().Get("p"))
	if !ok {
		jerr(w, 400, "invalid path")
		return nil
	}
	root, err := a.root(m)
	if err != nil {
		jerr(w, 503, "unavailable")
		return nil
	}
	return &target{m: m, root: root, rel: fsjail.Join(l.Path, sub), level: store.LvlRead}
}

func (a *App) hPubList(w http.ResponseWriter, r *http.Request, l store.Link, m store.Mount, c *Ctx) {
	if l.UploadOnly {
		jerr(w, 403, "this link is upload-only")
		return
	}
	if t := a.pubTarget(w, r, l, m); t != nil {
		listFolder(w, r, t, nil)
	}
}

func (a *App) hPubDownload(w http.ResponseWriter, r *http.Request, l store.Link, m store.Mount, c *Ctx) {
	if l.UploadOnly {
		jerr(w, 403, "this link is upload-only")
		return
	}
	t := a.pubTarget(w, r, l, m)
	if t == nil {
		return
	}
	f, st, good := openForDownload(w, t)
	if !good {
		return
	}
	defer f.Close()
	if firstRequest(r) {
		if !a.store.HitLink(l.Token) {
			jerr(w, 404, "this link has reached its download limit")
			return
		}
		a.aud.Write(c.User.Name, c.IP, "link-download", t.m.Name+":"+t.rel)
	}
	serveFile(w, r, f, st, path.Base(t.rel), false)
}

func (a *App) hPubZip(w http.ResponseWriter, r *http.Request, l store.Link, m store.Mount, c *Ctx) {
	if l.UploadOnly {
		jerr(w, 403, "this link is upload-only")
		return
	}
	t := a.pubTarget(w, r, l, m)
	if t == nil {
		return
	}
	// Isolated semaphore so anonymous visitors cannot starve authenticated users
	select {
	case a.pubZipSem <- struct{}{}:
		defer func() { <-a.pubZipSem }()
	default:
		jerr(w, 429, "busy, try again")
		return
	}
	if !a.store.HitLink(l.Token) {
		jerr(w, 404, "this link has reached its download limit")
		return
	}
	a.streamZip(w, r, c, t, r.URL.Query()["n"])
}

// hPubUpload accepts anonymous file drops on an UploadOnly share link.
// Files are always renamed on conflict (never overwrite existing files).
func (a *App) hPubUpload(w http.ResponseWriter, r *http.Request, l store.Link, m store.Mount, c *Ctx) {
	if !l.UploadOnly {
		jerr(w, 403, "uploads are not allowed on this link")
		return
	}
	if !a.pubSameOrigin(r) {
		jerr(w, 403, "request rejected")
		return
	}
	name := r.URL.Query().Get("name")
	if !fsjail.ValidNewName(name) {
		jerr(w, 400, "invalid file name")
		return
	}
	if r.ContentLength > maxDropUploadSize {
		jerr(w, 413, "file exceeds upload limit (100 MB)")
		return
	}
	root, err := a.root(m)
	if err != nil {
		jerr(w, 503, "unavailable")
		return
	}
	if st, err := root.Stat(fsjail.OSPath(l.Path)); err != nil || !st.IsDir() {
		jerr(w, 400, "target is not a folder")
		return
	}
	if !a.store.HitLink(l.Token) {
		jerr(w, 404, "this link has reached its upload limit")
		return
	}
	if err := fsjail.EnsureTmp(root); err != nil {
		fsErr(w, err)
		return
	}
	tmp := filepath.Join(fsjail.TmpDir, randx.Token(12)+".part")
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		fsErr(w, err)
		return
	}
	n, err := fsjail.CopyBuffered(f, io.LimitReader(bodyReader(w, r), maxDropUploadSize+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		root.Remove(tmp)
		jerr(w, 400, "upload interrupted")
		return
	}
	if n > maxDropUploadSize {
		root.Remove(tmp)
		jerr(w, 413, "file exceeds upload limit (100 MB)")
		return
	}
	final, err := fsjail.Place(root, tmp, l.Path, name, "rename")
	if err != nil {
		root.Remove(tmp)
		fsErr(w, err)
		return
	}
	a.aud.Write(c.User.Name, c.IP, "link-upload", fmt.Sprintf("%s:%s (%d bytes)", m.Name, fsjail.Join(l.Path, final), n))
	writeJSON(w, 200, map[string]any{"name": final})
}
