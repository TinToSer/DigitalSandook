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
	Has2FA     bool   `json:"has2fa"`
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
	if !a.sameOrigin(r) {
		jerr(w, 403, "request rejected")
		return
	}
	var in struct{ Username, Password, TOTP string }
	if !readJSON(w, r, &in) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(in.Username))
	ip := a.clientIP(r)
	ipKey, pairKey, userKey := "ip:"+ip, "iu:"+ip+":"+name, "u:"+name
	if d := max(a.lim.Wait(ipKey), max(a.lim.Wait(pairKey), a.lim.Wait(userKey))); d > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		jerr(w, 429, fmt.Sprintf("too many attempts, try again in %d seconds", int(d.Seconds())+1))
		return
	}
	if len(in.Password) > 1024 || len(name) > 64 || len(in.TOTP) > 32 {
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
	if !good || !exists || u.Disabled {
		a.lim.Fail(ipKey, 6, 20*time.Second, 15*time.Minute)
		a.lim.Fail(pairKey, 6, 20*time.Second, 15*time.Minute)
		// High threshold on global userKey prevents single-IP account lockout DoS
		// while still throttling distributed credential stuffing.
		a.lim.Fail(userKey, 40, 10*time.Second, 2*time.Minute)
		a.aud.Write(name, ip, "login-failed", "")
		jerr(w, 401, "invalid username or password")
		return
	}
	if u.Has2FA() {
		code := strings.TrimSpace(in.TOTP)
		if code == "" {
			writeJSON(w, 200, map[string]any{"needs2fa": true})
			return
		}
		if !a.verifyUser2FA(u, code) {
			a.lim.Fail(ipKey, 6, 20*time.Second, 15*time.Minute)
			a.lim.Fail(pairKey, 5, 30*time.Second, 15*time.Minute)
			a.aud.Write(name, ip, "login-2fa-failed", "")
			jerr(w, 401, "invalid authentication code")
			return
		}
	}
	// Record the login on the *current* record (in memory, flushed periodically
	// so logins never stall on an fsync).
	u, loggedIn := a.store.RecordLogin(u.Name, hash)
	if !loggedIn {
		a.aud.Write(name, ip, "login-failed", "changed")
		jerr(w, 401, "invalid username or password")
		return
	}
	a.lim.Reset(ipKey)
	a.lim.Reset(pairKey)
	token, s := a.sess.Create(u.Name, ip, r.UserAgent())
	a.setCookie(w, r, token, int(auth.SessionMax.Seconds()))
	a.aud.Write(u.Name, ip, "login", "")
	writeJSON(w, 200, map[string]any{
		"user": userView{u.Name, u.Admin, u.MustChange, u.Has2FA()}, "csrf": s.CSRF,
	})
}

// verifyUser2FA checks a 6-digit TOTP code (updating TOTPLastStep to prevent
// replay) or consumes a single-use recovery code.
func (a *App) verifyUser2FA(u store.User, code string) bool {
	if step, ok := auth.VerifyTOTP(u.TOTPSecret, code, time.Now(), u.TOTPLastStep); ok {
		_, err := a.store.UpdateUser(u.Name, func(cur *store.User) error {
			if cur.TOTPLastStep >= step {
				return errChanged // replayed concurrently
			}
			cur.TOTPLastStep = step
			return nil
		})
		return err == nil
	}
	// Try single-use recovery code
	wantHash := auth.HashRecoveryCode(code)
	_, err := a.store.UpdateUser(u.Name, func(cur *store.User) error {
		for i, h := range cur.RecoveryCodes {
			if auth.ConstantTimeEqual(h, wantHash) {
				cur.RecoveryCodes = append(cur.RecoveryCodes[:i], cur.RecoveryCodes[i+1:]...)
				return nil
			}
		}
		return errChanged
	})
	return err == nil
}

func (a *App) hLogout(w http.ResponseWriter, r *http.Request, c *Ctx) {
	a.sess.Drop(c.Sess)
	a.setCookie(w, r, "", -1)
	a.auditf(c, "logout", "")
	writeOK(w)
}

func (a *App) hMe(w http.ResponseWriter, r *http.Request, c *Ctx) {
	writeJSON(w, 200, map[string]any{
		"user":       userView{c.User.Name, c.User.Admin, c.User.MustChange, c.User.Has2FA()},
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
	// Rotate current session token & CSRF token and sign out all other devices
	newTok, newSess := a.sess.Rotate(c.Sess)
	a.sess.DropUser(c.User.Name, newSess)
	a.setCookie(w, r, newTok, int(auth.SessionMax.Seconds()))
	a.auditf(c, "password-change", "")
	writeJSON(w, 200, map[string]any{"ok": true, "csrf": newSess.CSRF})
}

// ---------- TOTP 2FA ----------

func (a *App) h2FASetup(w http.ResponseWriter, r *http.Request, c *Ctx) {
	secret := auth.GenerateTOTPSecret()
	uri := auth.TOTPURI(AppName, c.User.Name, secret)
	writeJSON(w, 200, map[string]any{"secret": secret, "uri": uri})
}

func (a *App) h2FAEnable(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct{ Password, Secret, Code string }
	if !readJSON(w, r, &in) {
		return
	}
	limKey := "2fa:" + c.User.Name
	if d := a.lim.Wait(limKey); d > 0 {
		jerr(w, 429, "too many attempts, try later")
		return
	}
	release, ok := a.hashSlot(w, r)
	if !ok {
		return
	}
	good := auth.VerifyPassword(in.Password, c.User.Hash)
	release()
	if !good {
		a.lim.Fail(limKey, 5, 30*time.Second, 10*time.Minute)
		jerr(w, 403, "current password is incorrect")
		return
	}
	step, valid := auth.VerifyTOTP(in.Secret, in.Code, time.Now(), 0)
	if !valid {
		a.lim.Fail(limKey, 5, 30*time.Second, 10*time.Minute)
		jerr(w, 400, "invalid 6-digit code; check your authenticator clock")
		return
	}
	plain, hashes := auth.GenerateRecoveryCodes(8)
	_, err := a.store.UpdateUser(c.User.Name, func(u *store.User) error {
		u.TOTPSecret = strings.ToUpper(strings.TrimSpace(in.Secret))
		u.TOTPLastStep = step
		u.RecoveryCodes = hashes
		return nil
	})
	if err != nil {
		jerr(w, 500, "could not save")
		return
	}
	a.auditf(c, "2fa-enable", "")
	writeJSON(w, 200, map[string]any{"ok": true, "recoveryCodes": plain})
}

func (a *App) h2FADisable(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct{ Password, Code string }
	if !readJSON(w, r, &in) {
		return
	}
	if !c.User.Has2FA() {
		writeOK(w)
		return
	}
	limKey := "2fa:" + c.User.Name
	if d := a.lim.Wait(limKey); d > 0 {
		jerr(w, 429, "too many attempts, try later")
		return
	}
	release, ok := a.hashSlot(w, r)
	if !ok {
		return
	}
	good := auth.VerifyPassword(in.Password, c.User.Hash)
	release()
	if !good {
		a.lim.Fail(limKey, 5, 30*time.Second, 10*time.Minute)
		jerr(w, 403, "current password is incorrect")
		return
	}
	if !a.verifyUser2FA(c.User, in.Code) {
		a.lim.Fail(limKey, 5, 30*time.Second, 10*time.Minute)
		jerr(w, 403, "invalid authentication code")
		return
	}
	_, err := a.store.UpdateUser(c.User.Name, func(u *store.User) error {
		u.TOTPSecret, u.TOTPLastStep, u.RecoveryCodes = "", 0, nil
		return nil
	})
	if err != nil {
		jerr(w, 500, "could not save")
		return
	}
	a.auditf(c, "2fa-disable", "")
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
