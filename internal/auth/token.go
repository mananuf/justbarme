package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// tokenByteLength exceeds the 32-byte minimum called for in
// docs/API_CONTRACT.md for both session and CSRF tokens.
const tokenByteLength = 32

// GenerateToken returns a cryptographically random, unpadded base64url
// string suitable as an opaque session or CSRF token. Only its hash is ever
// persisted — see HashToken.
func GenerateToken() (string, error) {
	buf := make([]byte, tokenByteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashToken returns the hex-encoded SHA-256 hash of a raw token, the only
// form ever stored server-side.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
