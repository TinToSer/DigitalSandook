package auth

import (
	"sync"
	"time"
)

type limEntry struct {
	fails int
	until time.Time
	last  time.Time
}

// Limiter implements exponential lockouts per key (an IP, an account, ...).
type Limiter struct {
	mu sync.Mutex
	m  map[string]*limEntry
}

func NewLimiter() *Limiter { return &Limiter{m: map[string]*limEntry{}} }

// Wait returns how long key is still locked out (0 = allowed).
func (l *Limiter) Wait(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e := l.m[key]; e != nil && time.Now().Before(e.until) {
		return time.Until(e.until)
	}
	return 0
}

// Fail records a failure; after `free` failures lockouts grow exponentially
// from base up to maxWait.
func (l *Limiter) Fail(key string, free int, base, maxWait time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	e := l.m[key]
	if e == nil {
		if len(l.m) >= 100_000 { // bound memory under a distributed attack
			for k, v := range l.m {
				if now.After(v.until) {
					delete(l.m, k)
				}
			}
		}
		e = &limEntry{}
		l.m[key] = e
	}
	if now.Sub(e.last) > 30*time.Minute {
		e.fails = 0
	}
	e.fails++
	e.last = now
	if e.fails >= free {
		d := base << uint(min(e.fails-free, 10))
		if d > maxWait || d <= 0 {
			d = maxWait
		}
		e.until = now.Add(d)
	}
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.m, key)
	l.mu.Unlock()
}

// GC forgets keys that have been quiet for an hour.
func (l *Limiter) GC() {
	l.mu.Lock()
	for k, e := range l.m {
		if time.Since(e.last) > time.Hour {
			delete(l.m, k)
		}
	}
	l.mu.Unlock()
}
