package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// leaseFormatVersion prefixes every issued lease so a future serialization
// change can be introduced without breaking leases already on a device.
const leaseFormatVersion = "v1"

// LeasePayload is the signed, bounded offline authorization lease described
// in docs/ARCHITECTURE.md §11.2. It is never accepted as proof of identity
// online — only the session cookie is — but lets an enrolled device operate
// for up to its ExpiresAt while disconnected.
type LeasePayload struct {
	BusinessID   uuid.UUID `json:"business_id"`
	UserID       uuid.UUID `json:"user_id"`
	DeviceID     uuid.UUID `json:"device_id"`
	LocationID   uuid.UUID `json:"location_id"`
	Role         string    `json:"role"`
	Capabilities []string  `json:"capabilities"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

var (
	ErrLeaseMalformed = errors.New("malformed offline lease")
	ErrLeaseSignature = errors.New("offline lease signature invalid")
	ErrLeaseExpired   = errors.New("offline lease expired")
)

// IssueLease signs payload with privateKey, producing a compact
// "v1.<payload>.<signature>" token (base64url, unpadded, dot-separated —
// deliberately not JWT, since JWT's algorithm-negotiation surface is
// unneeded complexity for a token this codebase both issues and verifies).
func IssueLease(privateKey ed25519.PrivateKey, payload LeasePayload) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode lease payload: %w", err)
	}
	signature := ed25519.Sign(privateKey, body)
	return strings.Join([]string{
		leaseFormatVersion,
		base64.RawURLEncoding.EncodeToString(body),
		base64.RawURLEncoding.EncodeToString(signature),
	}, "."), nil
}

// VerifyLease checks token's signature against publicKey and its expiry,
// returning the decoded payload only if both hold.
func VerifyLease(publicKey ed25519.PublicKey, token string) (*LeasePayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != leaseFormatVersion {
		return nil, ErrLeaseMalformed
	}

	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrLeaseMalformed
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrLeaseMalformed
	}
	if !ed25519.Verify(publicKey, body, signature) {
		return nil, ErrLeaseSignature
	}

	var payload LeasePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, ErrLeaseMalformed
	}
	if time.Now().After(payload.ExpiresAt) {
		return nil, ErrLeaseExpired
	}
	return &payload, nil
}
