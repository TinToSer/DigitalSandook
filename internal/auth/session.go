package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TinToSer/DigitalSandook/internal/randx"
)

const (
	SessionIdle = 12 * time.Hour      // signed out after this much inactivity
	SessionMax  = 14 * 24 * time.Hour // absolute lifetime
)

// Session is a signed-in browser. Only the SHA-256 of its bearer token is
// kept, so a memory dump or the admin listing never reveals usable cookies.
type Session struct {
	ID      string // public short id (for the admin listing)
	User    string
	CSRF    string
	Created time.Time
	IP      string
	UA      string

	key  [32]byte
	last atomic.Int64 // unix nanoseconds of the latest request
}

// LastSeen is the time of the session's most recent request.
func (s *Session) LastSeen() time.Time { return time.Unix(0, s.last.Load()) }

func (s *Session) expired(now time.Time) bool {
	return now.Sub(s.LastSeen()) > SessionIdle || now.Sub(s.Created) > SessionMax
}

// SessionInfo is a point-in-time copy of a session for listings.
type SessionInfo struct {
	ID, User, IP, UA string
	Created, Last    time.Time
}

type Sessions struct {
	mu sync.RWMutex
	m  map[[32]byte]*Session
}

func NewSessions() *Sessions { return &Sessions{m: map[[32]byte]*Session{}} }

// Create starts a session and returns its bearer token (for the cookie).
func (ss *Sessions) Create(user, ip, ua string) (token string, s *Session) {
	raw := make([]byte, 32)
	rand.Read(raw)
	token = base64.RawURLEncoding.EncodeToString(raw)
	key := sha256.Sum256([]byte(token))
	now := time.Now()
	if len(ua) > 200 {
		ua = ua[:200]
	}
	s = &Session{ID: hex.EncodeToString(key[:6]), User: user, CSRF: randx.Token(24),
		Created: now, IP: ip, UA: ua, key: key}
	s.last.Store(now.UnixNano())
	ss.mu.Lock()
	ss.m[key] = s
	ss.mu.Unlock()
	return
}

// Lookup returns the live session for a bearer token and marks it as used.
// The common path only takes a read lock, so many parallel requests (e.g.
// chunked uploads) do not serialise on it.
func (ss *Sessions) Lookup(token string) *Session {
	if len(token) < 20 || len(token) > 100 {
		return nil
	}
	key := sha256.Sum256([]byte(token))
	ss.mu.RLock()
	s := ss.m[key]
	ss.mu.RUnlock()
	if s == nil {
		return nil
	}
	now := time.Now()
	if s.expired(now) {
		ss.Drop(s)
		return nil
	}
	s.last.Store(now.UnixNano())
	return s
}

func (ss *Sessions) Drop(s *Session) {
	ss.mu.Lock()
	if ss.m[s.key] == s {
		delete(ss.m, s.key)
	}
	ss.mu.Unlock()
}

// DropUser ends every session of user except `except` (which may be nil).
func (ss *Sessions) DropUser(user string, except *Session) {
	ss.mu.Lock()
	for k, s := range ss.m {
		if s.User == user && s != except {
			delete(ss.m, k)
		}
	}
	ss.mu.Unlock()
}

// DropID ends the session with the given public id.
func (ss *Sessions) DropID(id string) (SessionInfo, bool) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	for k, s := range ss.m {
		if s.ID == id {
			delete(ss.m, k)
			return s.info(), true
		}
	}
	return SessionInfo{}, false
}

func (s *Session) info() SessionInfo {
	return SessionInfo{ID: s.ID, User: s.User, IP: s.IP, UA: s.UA, Created: s.Created, Last: s.LastSeen()}
}

func (ss *Sessions) List() []SessionInfo {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	out := make([]SessionInfo, 0, len(ss.m))
	for _, s := range ss.m {
		out = append(out, s.info())
	}
	return out
}

func (ss *Sessions) Len() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return len(ss.m)
}

// GC removes expired sessions.
func (ss *Sessions) GC() {
	now := time.Now()
	ss.mu.Lock()
	for k, s := range ss.m {
		if s.expired(now) {
			delete(ss.m, k)
		}
	}
	ss.mu.Unlock()
}
