package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	h := HashPassword("correct horse battery")
	if !VerifyPassword("correct horse battery", h) {
		t.Fatal("correct password rejected")
	}
	if VerifyPassword("wrong", h) || VerifyPassword("x", "") || VerifyPassword("x", "pbkdf2-sha256$1$a$b") {
		t.Fatal("wrong password or malformed hash accepted")
	}
}

func TestRandomPassword(t *testing.T) {
	const alpha = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		p := RandomPassword()
		if len(p) != 18 || strings.Trim(p, alpha) != "" {
			t.Fatalf("bad password %q", p)
		}
		if seen[p] {
			t.Fatal("duplicate password")
		}
		seen[p] = true
	}
}

func TestSessionLifecycle(t *testing.T) {
	ss := NewSessions()
	tok, s := ss.Create("alice", "1.2.3.4", "ua")
	if ss.Lookup(tok) != s {
		t.Fatal("fresh session not found")
	}
	if ss.Lookup(tok+"x") != nil || ss.Lookup("short") != nil {
		t.Fatal("wrong token accepted")
	}
	s.last.Store(time.Now().Add(-SessionIdle - time.Minute).UnixNano())
	if ss.Lookup(tok) != nil {
		t.Fatal("idle session still valid")
	}
	if ss.Len() != 0 {
		t.Fatal("expired session not removed")
	}
	tok2, s2 := ss.Create("bob", "", "")
	ss.Create("bob", "", "")
	ss.DropUser("bob", s2)
	if ss.Len() != 1 || ss.Lookup(tok2) != s2 {
		t.Fatal("DropUser must keep only the excepted session")
	}
}

func TestLimiterLocksOut(t *testing.T) {
	l := NewLimiter()
	for i := 0; i < 2; i++ {
		l.Fail("k", 3, time.Minute, time.Hour)
	}
	if l.Wait("k") != 0 {
		t.Fatal("locked out before the free attempts were used")
	}
	l.Fail("k", 3, time.Minute, time.Hour)
	if l.Wait("k") <= 0 {
		t.Fatal("not locked out after the free attempts")
	}
	l.Reset("k")
	if l.Wait("k") != 0 {
		t.Fatal("reset did not unlock")
	}
}
