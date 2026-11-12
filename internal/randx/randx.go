// Package randx produces cryptographically random identifiers.
package randx

import (
	"crypto/rand"
	"encoding/base64"
)

// Token returns n random bytes encoded as unpadded URL-safe base64.
func Token(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
