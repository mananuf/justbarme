package auth

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIssueAndVerifyLease(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	payload := LeasePayload{
		BusinessID:   uuid.New(),
		UserID:       uuid.New(),
		DeviceID:     uuid.New(),
		LocationID:   uuid.New(),
		Role:         "staff",
		Capabilities: []string{"sales:record", "bills:read"},
		IssuedAt:     time.Now().UTC().Truncate(time.Second),
		ExpiresAt:    time.Now().UTC().Add(7 * 24 * time.Hour).Truncate(time.Second),
	}

	token, err := IssueLease(priv, payload)
	if err != nil {
		t.Fatalf("IssueLease: %v", err)
	}

	got, err := VerifyLease(pub, token)
	if err != nil {
		t.Fatalf("VerifyLease: %v", err)
	}
	if got.BusinessID != payload.BusinessID || got.UserID != payload.UserID || got.DeviceID != payload.DeviceID {
		t.Fatalf("lease payload mismatch: got %+v, want %+v", got, payload)
	}
	if len(got.Capabilities) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(got.Capabilities))
	}
}

func TestVerifyLeaseRejectsTampering(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	payload := LeasePayload{
		BusinessID: uuid.New(),
		UserID:     uuid.New(),
		IssuedAt:   time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
	}
	token, err := IssueLease(priv, payload)
	if err != nil {
		t.Fatalf("IssueLease: %v", err)
	}

	tampered := token[:len(token)-4] + "abcd"
	if _, err := VerifyLease(pub, tampered); err == nil {
		t.Fatal("expected tampered lease to fail verification")
	}
}

func TestVerifyLeaseRejectsWrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	otherPub, _, _ := ed25519.GenerateKey(nil)

	token, err := IssueLease(priv, LeasePayload{ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatalf("IssueLease: %v", err)
	}
	if _, err := VerifyLease(otherPub, token); err != ErrLeaseSignature {
		t.Fatalf("expected ErrLeaseSignature, got %v", err)
	}
}

func TestVerifyLeaseRejectsExpired(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	token, err := IssueLease(priv, LeasePayload{ExpiresAt: time.Now().Add(-time.Minute)})
	if err != nil {
		t.Fatalf("IssueLease: %v", err)
	}
	if _, err := VerifyLease(pub, token); err != ErrLeaseExpired {
		t.Fatalf("expected ErrLeaseExpired, got %v", err)
	}
}

func TestVerifyLeaseRejectsMalformedToken(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	for _, token := range []string{"", "not-a-lease", "v1.onlyonepart", "v2.aa.bb"} {
		if _, err := VerifyLease(pub, token); err != ErrLeaseMalformed {
			t.Errorf("token %q: expected ErrLeaseMalformed, got %v", token, err)
		}
	}
}
