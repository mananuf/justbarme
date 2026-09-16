package auth

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

// GenerateOTP returns a cryptographically random 6-digit numeric code,
// zero-padded, for one-time email verification (see internal/signup). Only
// its hash (HashToken) is ever persisted, exactly like a session token.
func GenerateOTP() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate otp: %w", err)
	}
	n := binary.BigEndian.Uint32(buf) % 1_000_000
	return fmt.Sprintf("%06d", n), nil
}
