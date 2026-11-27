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

func TestSessionRotateAndCap(t *testing.T) {
	ss := NewSessions()
	tok1, s1 := ss.Create("alice", "1.2.3.4", "ua")
	tok2, s2 := ss.Rotate(s1)
	if tok1 == tok2 || s1.CSRF == s2.CSRF {
		t.Fatal("Rotate did not issue fresh token and CSRF")
	}
	if ss.Lookup(tok1) != nil || ss.Lookup(tok2) != s2 {
		t.Fatal("Rotate did not replace old session")
	}
	for i := 0; i < MaxSessionsPerUser+5; i++ {
		ss.Create("bob", "1.2.3.4", "ua")
	}
	bobCount := 0
	for _, info := range ss.List() {
		if info.User == "bob" {
			bobCount++
		}
	}
	if bobCount > MaxSessionsPerUser {
		t.Fatalf("bob has %d sessions, want <= %d", bobCount, MaxSessionsPerUser)
	}
}

func TestTOTPAndRecoveryCodes(t *testing.T) {
	secret := GenerateTOTPSecret()
	uri := TOTPURI("DigitalSanDook", "alice", secret)
	if !strings.HasPrefix(uri, "otpauth://totp/") || !strings.Contains(uri, secret) {
		t.Fatalf("bad URI: %s", uri)
	}
	now := time.Unix(1_700_000_000, 0)
	step := now.Unix() / totpPeriod
	// Decode secret and generate exact code
	_, ok := VerifyTOTP(secret, "000000", now, 0)
	_ = ok // random chance 1/1e6, test replay with known secret below
	const testSecret = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	// Compute via internal helper for deterministic check
	code := totpCode([]byte("Hello!\xde\xad\xbe\xefHello!\xde\xad\xbe\xef"), step)
	matched, valid := VerifyTOTP(testSecret, code, now, 0)
	if !valid || matched != step {
		t.Fatalf("valid TOTP rejected: matched=%d valid=%v", matched, valid)
	}
	// Replay at same step must fail
	if _, valid := VerifyTOTP(testSecret, code, now, matched); valid {
		t.Fatal("replayed TOTP code was accepted")
	}
	plain, hashes := GenerateRecoveryCodes(8)
	if len(plain) != 8 || len(hashes) != 8 {
		t.Fatalf("got %d recovery codes", len(plain))
	}
	if HashRecoveryCode(plain[0]) != hashes[0] {
		t.Fatal("recovery code hash mismatch")
	}
}
