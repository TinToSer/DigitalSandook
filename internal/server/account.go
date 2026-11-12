package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TinToSer/DigitalSandook/internal/auth"
	"github.com/TinToSer/DigitalSandook/internal/store"
)

type userView struct {
	Name       string `json:"name"`
	Admin      bool   `json:"admin"`
	MustChange bool   `json:"mustChange"`
}

func (a *App) cookieName(r *http.Request) string {
	if a.isSecure(r) {
		return "__Host-dsd"
	}
	return "dsd"
}

func (a *App) setCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: a.cookieName(r), Value: token, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: a.isSecure(r), SameSite: http.SameSiteStrictMode,
	})
}

func (a *App) sessionFrom(r *http.Request) *auth.Session {
	c, err := r.Cookie(a.cookieName(r))
	if err != nil {
		return nil
	}
	return a.sess.Lookup(c.Value)
}

// errChanged aborts a store update whose premise no longer holds because the
// record changed while the request was hashing a password.
var errChanged = errors.New("account changed meanwhile")

func (a *App) hLogin(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		jerr(w, 403, "request rejected")
		return
	}
	var in struct{ Username, Password string }
	if !readJSON(w, r, &in) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(in.Username))
	ip := a.clientIP(r)
	ipKey, userKey := "ip:"+ip, "u:"+name
	if d := max(a.lim.Wait(ipKey), a.lim.Wait(userKey)); d > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		jerr(w, 429, fmt.Sprintf("too many attempts, try again in %d seconds", int(d.Seconds())+1))
		return
	}
	if len(in.Password) > 1024 || len(name) > 64 {
		jerr(w, 400, "invalid credentials")
		return
	}
	release, ok := a.hashSlot(w, r)
	if !ok {
		return
	}
	u, exists := a.store.User(name)
	hash := auth.DummyHash
	if exists {
		hash = u.Hash
	}
	good := auth.VerifyPassword(in.Password, hash)
	release()
	loggedIn := false
	if good && exists && !u.Disabled {
		// Record the login on the *current* record. The account may have been
		// disabled or its password reset while we were hashing.
		u, loggedIn = a.loginRecorded(u.Name, hash)
	}
	if !loggedIn {
		a.lim.Fail(ipKey, 6, 20*time.Second, 15*time.Minute)
		a.lim.Fail(userKey, 12, 30*time.Second, 10*time.Minute)
		a.aud.Write(name, ip, "login-failed", "")
		jerr(w, 401, "invalid username or password")
		return
	}
	a.lim.Reset(ipKey)
	a.lim.Reset(userKey)
	token, s := a.sess.Create(u.Name, ip, r.UserAgent())
	a.setCookie(w, r, token, int(auth.SessionMax.Seconds()))
	a.aud.Write(u.Name, ip, "login", "")
	writeJSON(w, 200, map[string]any{
		"user": userView{u.Name, u.Admin, u.MustChange}, "csrf": s.CSRF,
	})
}

func (a *App) loginRecorded(name, verifiedHash string) (store.User, bool) {
	u, err := a.store.UpdateUser(name, func(cur *store.User) error {
		if cur.Disabled || cur.Hash != verifiedHash {
			return errChanged
		}
		cur.LastLogin = time.Now()
		return nil
	})
	return u, err == nil
}

func (a *App) hLogout(w http.ResponseWriter, r *http.Request, c *Ctx) {
	a.sess.Drop(c.Sess)
	a.setCookie(w, r, "", -1)
	a.auditf(c, "logout", "")
	writeOK(w)
}

func (a *App) hMe(w http.ResponseWriter, r *http.Request, c *Ctx) {
	writeJSON(w, 200, map[string]any{
		"user":       userView{c.User.Name, c.User.Admin, c.User.MustChange},
		"csrf":       c.Sess.CSRF,
		"roots":      a.store.Roots(c.User),
		"publicBase": a.tunnel.PublicBase(),
	})
}

func (a *App) hPassword(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct{ Old, New string }
	if !readJSON(w, r, &in) {
		return
	}
	if m := auth.CheckPasswordPolicy(in.New); m != "" {
		jerr(w, 400, m)
		return
	}
	limKey := "pw:" + c.User.Name
	if d := a.lim.Wait(limKey); d > 0 {
		jerr(w, 429, "too many attempts, try later")
		return
	}
	release, ok := a.hashSlot(w, r)
	if !ok {
		return
	}
	good := auth.VerifyPassword(in.Old, c.User.Hash)
	hash := ""
	if good {
		hash = auth.HashPassword(in.New)
	}
	release()
	if !good {
		a.lim.Fail(limKey, 5, 30*time.Second, 10*time.Minute)
		jerr(w, 403, "current password is incorrect")
		return
	}
	_, err := a.store.UpdateUser(c.User.Name, func(u *store.User) error {
		if u.Hash != c.User.Hash { // reset by an admin in the meantime
			return errChanged
		}
		u.Hash, u.MustChange = hash, false
		return nil
	})
	if errors.Is(err, errChanged) {
		jerr(w, 409, "your password was changed meanwhile; sign in again")
		return
	}
	if err != nil {
		jerr(w, 500, "could not save")
		return
	}
	a.sess.DropUser(c.User.Name, c.Sess) // sign out other devices
	a.auditf(c, "password-change", "")
	writeOK(w)
}

func (a *App) hUsersList(w http.ResponseWriter, r *http.Request, c *Ctx) {
	names := []string{}
	for _, u := range a.store.Users() {
		if !u.Disabled && u.Name != c.User.Name {
			names = append(names, u.Name)
		}
	}
	writeJSON(w, 200, names)
}
