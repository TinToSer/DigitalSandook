// Package auth implements password hashing, login sessions and the
// brute-force limiter. It has no knowledge of HTTP routing or storage.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PBKDF2-HMAC-SHA256 with OWASP's recommended iteration count (stdlib only).
const pbkdfIter = 600_000

func HashPassword(pw string) string {
	salt := make([]byte, 16)
	rand.Read(salt)
	dk, err := pbkdf2.Key(sha256.New, pw, salt, pbkdfIter, 32)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdfIter,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(dk))
}

func VerifyPassword(pw, enc string) bool {
	p := strings.Split(enc, "$")
	if len(p) != 4 || p[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(p[1])
	if err != nil || iter < 1000 || iter > 10_000_000 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(p[2])
	want, err2 := base64.RawStdEncoding.DecodeString(p[3])
	if err1 != nil || err2 != nil {
		return false
	}
	dk, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(dk, want) == 1
}

// DummyHash lets a login for an unknown user take as long as a real one, so
// response times do not reveal which accounts exist.
var DummyHash = HashPassword("dummy-password-for-timing")

// ConstantTimeEqual compares two secrets without leaking where they differ.
func ConstantTimeEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

var userRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,31}$`)

// ValidUsername reports whether name is an acceptable (lower-case) username.
func ValidUsername(name string) bool { return userRe.MatchString(name) }

// CheckPasswordPolicy returns a user-facing reason why pw is unacceptable, or "".
func CheckPasswordPolicy(pw string) string {
	if len(pw) < 10 {
		return "password must be at least 10 characters"
	}
	if len(pw) > 256 {
		return "password is too long"
	}
	return ""
}

// RandomPassword returns an 18-character password from an alphabet without
// look-alike characters (~104 bits of entropy).
func RandomPassword() string {
	const alpha = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	// rejection sampling: bytes >= limit would make some letters more likely
	const limit = 256 - 256%len(alpha)
	out := make([]byte, 0, 18)
	buf := make([]byte, 32)
	for len(out) < cap(out) {
		rand.Read(buf)
		for _, b := range buf {
			if int(b) < limit && len(out) < cap(out) {
				out = append(out, alpha[int(b)%len(alpha)])
			}
		}
	}
	return string(out)
}
