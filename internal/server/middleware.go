package server

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/TinToSer/DigitalSandook/internal/auth"
	"github.com/TinToSer/DigitalSandook/internal/store"
)

// Ctx is the authenticated caller of an API request.
type Ctx struct {
	User store.User
	Sess *auth.Session
	IP   string
}

type handler func(w http.ResponseWriter, r *http.Request, c *Ctx)

// ---------- client info ----------

type ctxKey int

const tunnelKey ctxKey = 1

// viaTunnel reports whether r arrived on the Cloudflare Tunnel listener.
func viaTunnel(r *http.Request) bool { v, _ := r.Context().Value(tunnelKey).(bool); return v }

func (a *App) clientIP(r *http.Request) string {
	if viaTunnel(r) { // loopback-only listener fed by cloudflared
		if ip := strings.TrimSpace(r.Header.Get("Cf-Connecting-Ip")); net.ParseIP(ip) != nil {
			return ip
		}
	}
	if a.trustProxy {
		if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
			parts := strings.Split(xf, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *App) isSecure(r *http.Request) bool {
	if r.TLS != nil || viaTunnel(r) { // Cloudflare terminates TLS for tunnel traffic
		return true
	}
	return a.trustProxy && r.Header.Get("X-Forwarded-Proto") == "https"
}

// ---------- security middleware ----------

const appCSP = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; " +
	"media-src 'self' blob:; connect-src 'self'; frame-src 'self'; font-src 'self'; " +
	"base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// secure sets the browser hardening headers and turns handler panics into 500s.
func (a *App) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Set("Content-Security-Policy", appCSP)
		if a.isSecure(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		defer func() {
			if e := recover(); e != nil {
				if e == http.ErrAbortHandler {
					panic(e)
				}
				log.Printf("panic: %v on %s %s", e, r.Method, r.URL.Path)
				jerr(w, 500, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// sameOrigin rejects cross-site state-changing requests (CSRF defence in depth
// on top of SameSite=Strict cookies and the X-CSRF-Token header).
func sameOrigin(r *http.Request) bool {
	if sf := r.Header.Get("Sec-Fetch-Site"); sf != "" && sf != "same-origin" && sf != "none" {
		return false
	}
	o := r.Header.Get("Origin")
	if o == "null" {
		return false
	}
	if o != "" {
		i := strings.Index(o, "://")
		if i < 0 || o[i+3:] != r.Host {
			return false
		}
	}
	return true
}

func isSafeMethod(m string) bool { return m == "GET" || m == "HEAD" || m == "OPTIONS" }

// api wraps a handler with authentication, CSRF protection and role checks.
func (a *App) api(h handler, adminOnly bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := a.sessionFrom(r)
		if s == nil {
			jerr(w, 401, "authentication required")
			return
		}
		u, ok := a.store.User(s.User)
		if !ok || u.Disabled {
			a.sess.Drop(s)
			jerr(w, 401, "authentication required")
			return
		}
		if !isSafeMethod(r.Method) {
			if !sameOrigin(r) || !auth.ConstantTimeEqual(r.Header.Get("X-CSRF-Token"), s.CSRF) {
				jerr(w, 403, "request rejected (CSRF)")
				return
			}
		}
		if adminOnly && !u.Admin {
			jerr(w, 403, "administrator only")
			return
		}
		if u.MustChange && r.URL.Path != "/api/password" && r.URL.Path != "/api/me" && r.URL.Path != "/api/logout" {
			jerr(w, 403, "password change required")
			return
		}
		h(w, r, &Ctx{User: u, Sess: s, IP: a.clientIP(r)})
	}
}

func (a *App) auditf(c *Ctx, action, format string, args ...any) {
	a.aud.Write(c.User.Name, c.IP, action, fmt.Sprintf(format, args...))
}
