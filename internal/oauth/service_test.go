package oauth_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/oauth"
)

// fakeVerifier returns a fixed GoogleClaims (or a fixed error) instead of
// verifying anything -- internal/oauth.Service's own logic is what's under
// test here, not GoogleVerifier's JWT/JWKS handling (see
// google_verifier_test.go for that).
type fakeVerifier struct {
	claims oauth.GoogleClaims
	err    error
}

func (f fakeVerifier) Verify(context.Context, string) (oauth.GoogleClaims, error) {
	return f.claims, f.err
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("JBM_DATABASE_URL")
	if url == "" {
		t.Skip("JBM_DATABASE_URL not set; skipping PostgreSQL-backed test")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("database not reachable: %v", err)
	}
	return pool
}

func uniqueEmail() string {
	return uuid.New().String() + "@example.com"
}

func cleanupIdentity(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			return
		}
		defer conn.Release()
		_, _ = conn.Exec(context.Background(), "DELETE FROM sessions WHERE user_id = $1", userID)
		_, _ = conn.Exec(context.Background(), "DELETE FROM user_identities WHERE user_id = $1", userID)
		_, _ = conn.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})
}

func TestSignInWithGoogleCreatesNewAccount(t *testing.T) {
	pool := testPool(t)
	identitySvc := identity.New(pool, config.Argon2{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1})
	addr := uniqueEmail()
	verifier := fakeVerifier{claims: oauth.GoogleClaims{
		Sub: "google-sub-" + uuid.NewString(), Email: addr, EmailVerified: true, Name: "Ada Obi",
	}}
	svc := oauth.New(pool, identitySvc, verifier)

	user, rawToken, rawCSRF, sess, err := svc.SignInWithGoogle(context.Background(), "irrelevant-raw-token", "test-agent", time.Hour)
	if err != nil {
		t.Fatalf("SignInWithGoogle: %v", err)
	}
	cleanupIdentity(t, pool, user.ID)

	if user.Email != addr || user.DisplayName != "Ada Obi" || user.Status != "active" {
		t.Fatalf("unexpected created user: %+v", user)
	}
	if rawToken == "" || rawCSRF == "" || sess.UserID != user.ID {
		t.Fatalf("expected a real session, got token=%q csrf=%q sess=%+v", rawToken, rawCSRF, sess)
	}
}

func TestSignInWithGoogleReturningAccountUsesTheExistingLink(t *testing.T) {
	pool := testPool(t)
	identitySvc := identity.New(pool, config.Argon2{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1})
	addr := uniqueEmail()
	verifier := fakeVerifier{claims: oauth.GoogleClaims{
		Sub: "google-sub-" + uuid.NewString(), Email: addr, EmailVerified: true, Name: "Ada Obi",
	}}
	svc := oauth.New(pool, identitySvc, verifier)
	ctx := context.Background()

	first, _, _, _, err := svc.SignInWithGoogle(ctx, "irrelevant-raw-token", "test-agent", time.Hour)
	if err != nil {
		t.Fatalf("first SignInWithGoogle: %v", err)
	}
	cleanupIdentity(t, pool, first.ID)

	second, _, _, _, err := svc.SignInWithGoogle(ctx, "irrelevant-raw-token", "test-agent", time.Hour)
	if err != nil {
		t.Fatalf("second SignInWithGoogle: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected the second sign-in to reuse the same account, got a different user: %+v vs %+v", first, second)
	}
}

func TestSignInWithGoogleAutoLinksExistingPasswordAccount(t *testing.T) {
	pool := testPool(t)
	identitySvc := identity.New(pool, config.Argon2{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1})
	addr := uniqueEmail()

	existing, err := identitySvc.CreateUser(context.Background(), addr, "", "Existing User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupIdentity(t, pool, existing.ID)

	verifier := fakeVerifier{claims: oauth.GoogleClaims{
		Sub: "google-sub-" + uuid.NewString(), Email: addr, EmailVerified: true, Name: "Someone Else",
	}}
	svc := oauth.New(pool, identitySvc, verifier)

	linked, _, _, _, err := svc.SignInWithGoogle(context.Background(), "irrelevant-raw-token", "test-agent", time.Hour)
	if err != nil {
		t.Fatalf("SignInWithGoogle: %v", err)
	}
	if linked.ID != existing.ID {
		t.Fatalf("expected the Google sign-in to link to the existing password account %s, got a different user %s", existing.ID, linked.ID)
	}
	if linked.DisplayName != "Existing User" {
		t.Fatalf("expected linking to preserve the existing account's own name, got %q", linked.DisplayName)
	}
}

func TestSignInWithGoogleRejectsUnverifiedEmail(t *testing.T) {
	pool := testPool(t)
	identitySvc := identity.New(pool, config.Argon2{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1})
	verifier := fakeVerifier{claims: oauth.GoogleClaims{
		Sub: "google-sub-" + uuid.NewString(), Email: uniqueEmail(), EmailVerified: false,
	}}
	svc := oauth.New(pool, identitySvc, verifier)

	if _, _, _, _, err := svc.SignInWithGoogle(context.Background(), "irrelevant-raw-token", "test-agent", time.Hour); err != oauth.ErrGoogleSignInFailed {
		t.Fatalf("expected ErrGoogleSignInFailed for an unverified email, got %v", err)
	}
}

func TestSignInWithGoogleRejectsAnUnverifiableToken(t *testing.T) {
	pool := testPool(t)
	identitySvc := identity.New(pool, config.Argon2{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1})
	verifier := fakeVerifier{err: errors.New("bad token")}
	svc := oauth.New(pool, identitySvc, verifier)

	if _, _, _, _, err := svc.SignInWithGoogle(context.Background(), "garbage", "test-agent", time.Hour); err != oauth.ErrGoogleSignInFailed {
		t.Fatalf("expected ErrGoogleSignInFailed for an unverifiable token, got %v", err)
	}
}
