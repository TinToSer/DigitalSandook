package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const totpPeriod = 30

// GenerateTOTPSecret returns a random 160-bit Base32-encoded secret (unpadded).
func GenerateTOTPSecret() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
}

// TOTPURI returns an otpauth:// URI compatible with Google Authenticator,
// Authy, 1Password, Bitwarden, etc.
func TOTPURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", "30")
	return "otpauth://totp/" + label + "?" + v.Encode()
}

func totpCode(key []byte, counter int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(counter))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[off]&0x7f) << 24) |
		(uint32(sum[off+1]) << 16) |
		(uint32(sum[off+2]) << 8) |
		uint32(sum[off+3])
	return fmt.Sprintf("%06d", code%1_000_000)
}

// VerifyTOTP checks a 6-digit RFC 6238 TOTP code allowing ±1 time step skew.
// lastStep prevents replay of an already-used code; the matched step is returned.
func VerifyTOTP(secret, code string, now time.Time, lastStep int64) (matchedStep int64, ok bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return 0, false
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(key) < 10 {
		return 0, false
	}
	step := now.Unix() / totpPeriod
	for delta := int64(-1); delta <= 1; delta++ {
		s := step + delta
		if s <= lastStep {
			continue
		}
		if ConstantTimeEqual(code, totpCode(key, s)) {
			return s, true
		}
	}
	return 0, false
}

// GenerateRecoveryCodes returns n single-use recovery codes (plain) and their
// SHA-256 hex digests (for storage).
func GenerateRecoveryCodes(n int) (plain []string, hashes []string) {
	const alpha = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	const limit = 256 - 256%len(alpha)
	plain = make([]string, 0, n)
	hashes = make([]string, 0, n)
	buf := make([]byte, 32)
	for len(plain) < n {
		var out [8]byte
		i := 0
		for i < 8 {
			if _, err := rand.Read(buf); err != nil {
				panic(err)
			}
			for _, b := range buf {
				if int(b) < limit && i < 8 {
					out[i] = alpha[int(b)%len(alpha)]
					i++
				}
			}
		}
		code := string(out[:4]) + "-" + string(out[4:])
		plain = append(plain, code)
		hashes = append(hashes, HashRecoveryCode(code))
	}
	return plain, hashes
}

// HashRecoveryCode normalizes and hashes a recovery code with SHA-256.
func HashRecoveryCode(code string) string {
	norm := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(code), "-", ""), " ", ""))
	sum := sha256.Sum256([]byte("dsd-recovery:" + norm))
	return hex.EncodeToString(sum[:])
}
