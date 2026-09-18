package whatsapp_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/mananuf/justbarme/internal/whatsapp"
)

func sign(secret string, ts int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(fmt.Appendf(nil, "%d.", ts))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignatureAcceptsValidSignature(t *testing.T) {
	secret := "whsec_test"
	body := []byte(`{"event":"message.delivered"}`)
	ts := time.Now().Unix()
	header := fmt.Sprintf("t=%d,v2=%s", ts, sign(secret, ts, body))

	if err := whatsapp.VerifySignature(header, body, secret); err != nil {
		t.Fatalf("VerifySignature: %v", err)
	}
}

func TestVerifySignatureRejectsWrongSecret(t *testing.T) {
	body := []byte(`{"event":"message.delivered"}`)
	ts := time.Now().Unix()
	header := fmt.Sprintf("t=%d,v2=%s", ts, sign("whsec_correct", ts, body))

	if err := whatsapp.VerifySignature(header, body, "whsec_wrong"); err == nil {
		t.Fatal("expected an error for a signature made with the wrong secret")
	}
}

func TestVerifySignatureRejectsTamperedBody(t *testing.T) {
	secret := "whsec_test"
	ts := time.Now().Unix()
	header := fmt.Sprintf("t=%d,v2=%s", ts, sign(secret, ts, []byte(`{"event":"message.delivered"}`)))

	if err := whatsapp.VerifySignature(header, []byte(`{"event":"message.failed"}`), secret); err == nil {
		t.Fatal("expected an error for a tampered body")
	}
}

func TestVerifySignatureRejectsStaleTimestamp(t *testing.T) {
	secret := "whsec_test"
	body := []byte(`{"event":"message.delivered"}`)
	ts := time.Now().Add(-10 * time.Minute).Unix()
	header := fmt.Sprintf("t=%d,v2=%s", ts, sign(secret, ts, body))

	if err := whatsapp.VerifySignature(header, body, secret); err == nil {
		t.Fatal("expected an error for a timestamp older than the 5-minute window")
	}
}

func TestVerifySignatureRejectsFutureTimestamp(t *testing.T) {
	secret := "whsec_test"
	body := []byte(`{"event":"message.delivered"}`)
	ts := time.Now().Add(10 * time.Minute).Unix()
	header := fmt.Sprintf("t=%d,v2=%s", ts, sign(secret, ts, body))

	if err := whatsapp.VerifySignature(header, body, secret); err == nil {
		t.Fatal("expected an error for a timestamp too far in the future")
	}
}

func TestVerifySignatureRejectsMalformedHeader(t *testing.T) {
	if err := whatsapp.VerifySignature("not-a-valid-header", []byte("{}"), "secret"); err == nil {
		t.Fatal("expected an error for a malformed header")
	}
}
