package oauth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// googleJWKSURL is Google's published, stable RS256 signing-key set for ID
// tokens -- documented at https://developers.google.com/identity/openid-connect/openid-connect#discovery,
// hardcoded rather than fetched from the OIDC discovery document, since it
// has not changed in years and one fewer network round trip per key
// refresh is worth it here.
const googleJWKSURL = "https://www.googleapis.com/oauth2/v3/certs"

// jwksCacheTTL and fetchTimeout bound how stale a cached signing key can
// get and how long a Verify call can hang fetching a fresh set,
// respectively -- the same "explicit timeout, no unbounded hang" pattern
// internal/email.SMTPProvider's dialTimeout uses.
const (
	jwksCacheTTL = time.Hour
	fetchTimeout = 5 * time.Second
)

// googleIssuers are the two values Google's own documentation says a real
// ID token's "iss" claim may carry.
var googleIssuers = map[string]bool{
	"accounts.google.com":         true,
	"https://accounts.google.com": true,
}

// GoogleVerifier implements IDTokenVerifier by validating a Google-issued
// ID token entirely by hand -- RS256 signature against Google's published
// JWKS, plus issuer/audience/expiry checks -- rather than pulling in a
// general-purpose JWT/OIDC library. This mirrors internal/auth.VerifyLease:
// the format is externally fixed (we cannot design it away, unlike this
// codebase's own offline lease), but the *verification surface* is kept
// deliberately narrow -- RS256 only, no algorithm negotiation, no "alg:
// none" foot-gun a general library's flexibility can otherwise reintroduce.
type GoogleVerifier struct {
	clientID   string
	httpClient *http.Client
	jwksURL    string

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

func NewGoogleVerifier(clientID string) *GoogleVerifier {
	return &GoogleVerifier{
		clientID:   clientID,
		httpClient: &http.Client{Timeout: fetchTimeout},
		jwksURL:    googleJWKSURL,
	}
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type googleClaimsJSON struct {
	Iss           string `json:"iss"`
	Aud           string `json:"aud"`
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Exp           int64  `json:"exp"`
}

// Verify checks rawToken's RS256 signature against Google's current
// signing keys (refreshing its cache once if the token's "kid" isn't
// found, in case Google rotated keys since the last fetch) and validates
// issuer, audience, and expiry. It deliberately does not check
// EmailVerified -- that's Service.SignInWithGoogle's call, since whether an
// unverified email is acceptable is a caller policy decision, not a token-
// validity one.
func (v *GoogleVerifier) Verify(ctx context.Context, rawToken string) (GoogleClaims, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return GoogleClaims{}, errors.New("malformed google id token")
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return GoogleClaims{}, errors.New("malformed google id token header")
	}
	var header jwtHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return GoogleClaims{}, errors.New("malformed google id token header")
	}
	if header.Alg != "RS256" {
		return GoogleClaims{}, fmt.Errorf("unsupported google id token algorithm %q", header.Alg)
	}

	key, err := v.publicKey(ctx, header.Kid)
	if err != nil {
		return GoogleClaims{}, err
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return GoogleClaims{}, errors.New("malformed google id token signature")
	}
	hashed := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hashed[:], signature); err != nil {
		return GoogleClaims{}, errors.New("google id token signature invalid")
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return GoogleClaims{}, errors.New("malformed google id token payload")
	}
	var claims googleClaimsJSON
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return GoogleClaims{}, errors.New("malformed google id token payload")
	}

	if !googleIssuers[claims.Iss] {
		return GoogleClaims{}, fmt.Errorf("untrusted google id token issuer %q", claims.Iss)
	}
	if claims.Aud != v.clientID {
		return GoogleClaims{}, errors.New("google id token was not issued for this application")
	}
	if time.Now().Unix() >= claims.Exp {
		return GoogleClaims{}, errors.New("google id token expired")
	}

	return GoogleClaims{
		Sub:           claims.Sub,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
		Name:          claims.Name,
	}, nil
}

func (v *GoogleVerifier) publicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	fresh := time.Since(v.fetchedAt) < jwksCacheTTL
	v.mu.RUnlock()
	if ok && fresh {
		return key, nil
	}

	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	key, ok = v.keys[kid]
	if !ok {
		return nil, fmt.Errorf("no google signing key found for kid %q", kid)
	}
	return key, nil
}

type googleJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type googleJWKSResponse struct {
	Keys []googleJWK `json:"keys"`
}

func (v *GoogleVerifier) refreshKeys(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("build google jwks request: %w", err)
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch google jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch google jwks: unexpected status %d", resp.StatusCode)
	}

	var parsed googleJWKSResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("decode google jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(parsed.Keys))
	for _, k := range parsed.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(k)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}

	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

func rsaPublicKeyFromJWK(k googleJWK) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode exponent: %w", err)
	}
	var e int
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}
