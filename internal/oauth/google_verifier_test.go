package oauth

// Internal test (package oauth, not oauth_test) so it can point
// GoogleVerifier at a local test server instead of the real
// googleJWKSURL -- mirrors internal/email/smtp_internal_test.go's reason
// for being an internal test.

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testKID = "test-key-1"

func newTestVerifier(t *testing.T, clientID string, key *rsa.PrivateKey) *GoogleVerifier {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(googleJWKSResponse{Keys: []googleJWK{{
			Kty: "RSA",
			Kid: testKID,
			N:   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(bigEndianMinimal(key.PublicKey.E)),
		}}})
	}))
	t.Cleanup(server.Close)
	return &GoogleVerifier{clientID: clientID, httpClient: server.Client(), jwksURL: server.URL}
}

func bigEndianMinimal(n int) []byte {
	if n == 0 {
		return []byte{0}
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte(n & 0xff)}, b...)
		n >>= 8
	}
	return b
}

// signedToken builds a compact RS256 JWT (or, if alg is overridden, a
// token whose header just *claims* a different algorithm without a real
// matching signature -- for the "algorithm confusion" rejection test).
func signedToken(t *testing.T, key *rsa.PrivateKey, alg string, claims googleClaimsJSON) string {
	t.Helper()
	header, err := json.Marshal(jwtHeader{Alg: alg, Kid: testKID})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	signedInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)

	hashed := sha256.Sum256([]byte(signedInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hashed[:])
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signedInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func validClaims(clientID string) googleClaimsJSON {
	return googleClaimsJSON{
		Iss: "https://accounts.google.com", Aud: clientID, Sub: "12345",
		Email: "owner@example.com", EmailVerified: true, Name: "Ada Obi",
		Exp: time.Now().Add(time.Hour).Unix(),
	}
}

func TestGoogleVerifierAcceptsAValidToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	verifier := newTestVerifier(t, "test-client", key)
	token := signedToken(t, key, "RS256", validClaims("test-client"))

	claims, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Sub != "12345" || claims.Email != "owner@example.com" || !claims.EmailVerified || claims.Name != "Ada Obi" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestGoogleVerifierRejectsWrongAudience(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	verifier := newTestVerifier(t, "test-client", key)
	token := signedToken(t, key, "RS256", validClaims("some-other-client"))

	if _, err := verifier.Verify(context.Background(), token); err == nil {
		t.Fatal("expected an error for a token issued for a different audience, got nil")
	}
}

func TestGoogleVerifierRejectsUntrustedIssuer(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	verifier := newTestVerifier(t, "test-client", key)
	claims := validClaims("test-client")
	claims.Iss = "https://evil.example.com"
	token := signedToken(t, key, "RS256", claims)

	if _, err := verifier.Verify(context.Background(), token); err == nil {
		t.Fatal("expected an error for an untrusted issuer, got nil")
	}
}

func TestGoogleVerifierRejectsExpiredToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	verifier := newTestVerifier(t, "test-client", key)
	claims := validClaims("test-client")
	claims.Exp = time.Now().Add(-time.Hour).Unix()
	token := signedToken(t, key, "RS256", claims)

	if _, err := verifier.Verify(context.Background(), token); err == nil {
		t.Fatal("expected an error for an expired token, got nil")
	}
}

func TestGoogleVerifierRejectsWrongSigningKey(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	// Signed with a key the JWKS endpoint never publishes -- simulates a
	// forged token, not just a stale cache.
	verifier := newTestVerifier(t, "test-client", key)
	token := signedToken(t, otherKey, "RS256", validClaims("test-client"))

	if _, err := verifier.Verify(context.Background(), token); err == nil {
		t.Fatal("expected an error for a token signed with an unpublished key, got nil")
	}
}

func TestGoogleVerifierRejectsNonRS256Algorithm(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	verifier := newTestVerifier(t, "test-client", key)
	// The header claims HS256 (which would let an attacker treat the
	// public key material as an HMAC secret if this weren't rejected
	// before any key lookup happens) but is still "signed" with the RSA
	// key so this only tests the algorithm check, not signature format.
	token := signedToken(t, key, "HS256", validClaims("test-client"))

	if _, err := verifier.Verify(context.Background(), token); err == nil {
		t.Fatal("expected an error for a non-RS256 token, got nil")
	}
}

func TestGoogleVerifierRejectsMalformedToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	verifier := newTestVerifier(t, "test-client", key)

	for _, token := range []string{"", "not-a-jwt", "a.b", "a.b.c.d"} {
		if _, err := verifier.Verify(context.Background(), token); err == nil {
			t.Fatalf("expected an error for malformed token %q, got nil", token)
		}
	}
}
