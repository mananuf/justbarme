package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params are the versioned cost parameters used to hash a password.
// They are embedded in every encoded hash so historical hashes keep
// verifying correctly after the configured defaults change.
type Argon2Params struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
}

const (
	saltLength = 16
	keyLength  = 32
)

var (
	ErrInvalidHashFormat    = errors.New("invalid password hash format")
	ErrUnsupportedAlgorithm = errors.New("unsupported password hash algorithm")
)

// HashPassword encodes the result as a self-describing string:
// $argon2id$v=<version>$m=<memoryKiB>,t=<iterations>,p=<parallelism>$<salt>$<hash>
func HashPassword(password string, params Argon2Params) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKiB, params.Parallelism, keyLength)

	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, params.MemoryKiB, params.Iterations, params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

// VerifyPassword reports whether password matches encoded, using the cost
// parameters recorded in encoded itself rather than the caller's current
// defaults, so a rotated JBM_ARGON2_* configuration never breaks existing
// accounts. Use NeedsRehash to decide whether to re-hash on successful login.
func VerifyPassword(encoded, password string) (bool, error) {
	params, salt, hash, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}

	candidate := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKiB, params.Parallelism, uint32(len(hash)))
	return subtle.ConstantTimeCompare(hash, candidate) == 1, nil
}

// NeedsRehash reports whether encoded was hashed with parameters other than
// want, so callers can transparently upgrade a stored hash after a
// successful login.
func NeedsRehash(encoded string, want Argon2Params) bool {
	params, _, _, err := decodeHash(encoded)
	if err != nil {
		return true
	}
	return params != want
}

func decodeHash(encoded string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, ErrInvalidHashFormat
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHashFormat
	}
	if version != argon2.Version {
		return Argon2Params{}, nil, nil, ErrUnsupportedAlgorithm
	}

	var params Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.MemoryKiB, &params.Iterations, &params.Parallelism); err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHashFormat
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHashFormat
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHashFormat
	}
	return params, salt, hash, nil
}
