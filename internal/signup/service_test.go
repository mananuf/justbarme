package signup_test

import (
	"context"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/email"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/signup"
)

// fakeEmailProvider captures every message instead of sending it, so tests
// can pull the OTP code back out (it is never returned by the service
// itself -- only the real email carries it, matching production).
type fakeEmailProvider struct {
	mu       sync.Mutex
	messages []email.Message
}

func (f *fakeEmailProvider) Send(_ context.Context, msg email.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, msg)
	return nil
}

func (f *fakeEmailProvider) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
}

var otpPattern = regexp.MustCompile(`\d{6}`)

func (f *fakeEmailProvider) lastCode(t *testing.T) string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.messages) == 0 {
		t.Fatal("no email was sent")
	}
	code := otpPattern.FindString(f.messages[len(f.messages)-1].Text)
	if code == "" {
		t.Fatalf("could not find a 6-digit code in email body: %q", f.messages[len(f.messages)-1].Text)
	}
	return code
}

// testServices returns a signup.Service (with a fake email provider so
// tests can read the OTP back out) and the identity.Service it wraps,
// sharing one real PostgreSQL pool, or skips if JBM_DATABASE_URL is unset.
func testServices(t *testing.T, otpTTL time.Duration) (*signup.Service, *identity.Service, *fakeEmailProvider, *pgxpool.Pool) {
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

	argon2 := config.Argon2{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1}
	identitySvc := identity.New(pool, argon2)
	fakeEmail := &fakeEmailProvider{}
	signupSvc := signup.New(pool, argon2, identitySvc, fakeEmail, otpTTL)
	return signupSvc, identitySvc, fakeEmail, pool
}

func uniqueEmail() string {
	return uuid.New().String() + "@example.com"
}

func cleanupSignup(t *testing.T, pool *pgxpool.Pool, email string) {
	t.Helper()
	t.Cleanup(func() {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			return
		}
		defer conn.Release()
		_, _ = conn.Exec(context.Background(), "DELETE FROM signup_verifications WHERE lower(email) = lower($1)", email)
	})
}

func cleanupUser(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			return
		}
		defer conn.Release()
		_, _ = conn.Exec(context.Background(), "DELETE FROM sessions WHERE user_id = $1", userID)
		_, _ = conn.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})
}

func TestStartAndVerifySignup(t *testing.T) {
	svc, _, fakeEmail, pool := testServices(t, time.Hour)
	ctx := context.Background()
	addr := uniqueEmail()
	cleanupSignup(t, pool, addr)

	if err := svc.Start(ctx, addr, "Ada Obi", "password123"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if fakeEmail.count() != 1 {
		t.Fatalf("expected exactly one email sent, got %d", fakeEmail.count())
	}
	code := fakeEmail.lastCode(t)

	user, rawToken, rawCSRF, sess, err := svc.Verify(ctx, addr, code, "test-agent", time.Hour)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	cleanupUser(t, pool, user.ID)

	if user.Email != addr || user.DisplayName != "Ada Obi" || user.Status != "active" {
		t.Fatalf("unexpected created user: %+v", user)
	}
	if rawToken == "" || rawCSRF == "" || sess.UserID != user.ID {
		t.Fatalf("expected a real session for the new user, got token=%q csrf=%q sess=%+v", rawToken, rawCSRF, sess)
	}

	// The pending row is deleted on success -- verifying again (even with
	// the same, still "valid-looking" code) must fail, not silently
	// succeed or create a second account.
	if _, _, _, _, err := svc.Verify(ctx, addr, code, "test-agent", time.Hour); err != signup.ErrNoPendingSignup {
		t.Fatalf("expected ErrNoPendingSignup after a completed signup, got %v", err)
	}
}

func TestStartRejectsAlreadyRegisteredEmail(t *testing.T) {
	svc, identitySvc, fakeEmail, pool := testServices(t, time.Hour)
	ctx := context.Background()
	addr := uniqueEmail()

	existing, err := identitySvc.CreateUser(ctx, addr, "", "Existing User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupUser(t, pool, existing.ID)

	if err := svc.Start(ctx, addr, "Someone Else", "password123"); err != signup.ErrEmailAlreadyRegistered {
		t.Fatalf("expected ErrEmailAlreadyRegistered, got %v", err)
	}
	if fakeEmail.count() != 0 {
		t.Fatal("expected no email to be sent for an already-registered address")
	}
}

func TestVerifyNoPendingSignupForUnknownEmail(t *testing.T) {
	svc, _, _, _ := testServices(t, time.Hour)
	if _, _, _, _, err := svc.Verify(context.Background(), uniqueEmail(), "123456", "test-agent", time.Hour); err != signup.ErrNoPendingSignup {
		t.Fatalf("expected ErrNoPendingSignup, got %v", err)
	}
}

func TestVerifyWrongCodeLocksOutAfterMaxAttempts(t *testing.T) {
	svc, _, fakeEmail, pool := testServices(t, time.Hour)
	ctx := context.Background()
	addr := uniqueEmail()
	cleanupSignup(t, pool, addr)

	if err := svc.Start(ctx, addr, "Ada Obi", "password123"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	correctCode := fakeEmail.lastCode(t)

	// Five wrong attempts, matching the service's own maxOTPAttempts.
	for i := 0; i < 5; i++ {
		if _, _, _, _, err := svc.Verify(ctx, addr, "000000", "test-agent", time.Hour); err != signup.ErrInvalidOrExpiredCode {
			t.Fatalf("attempt %d: expected ErrInvalidOrExpiredCode, got %v", i, err)
		}
	}

	// The correct code must now also fail -- attempts exhausted, not just
	// individual wrong guesses.
	if _, _, _, _, err := svc.Verify(ctx, addr, correctCode, "test-agent", time.Hour); err != signup.ErrInvalidOrExpiredCode {
		t.Fatalf("expected the correct code to be locked out after max attempts, got %v", err)
	}
}

func TestStartAgainReplacesThePreviousCode(t *testing.T) {
	svc, _, fakeEmail, pool := testServices(t, time.Hour)
	ctx := context.Background()
	addr := uniqueEmail()
	cleanupSignup(t, pool, addr)

	if err := svc.Start(ctx, addr, "Ada Obi", "password123"); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	firstCode := fakeEmail.lastCode(t)

	if err := svc.Start(ctx, addr, "Ada Obi", "password123"); err != nil {
		t.Fatalf("second Start (resend): %v", err)
	}
	secondCode := fakeEmail.lastCode(t)

	if _, _, _, _, err := svc.Verify(ctx, addr, firstCode, "test-agent", time.Hour); err != signup.ErrInvalidOrExpiredCode {
		t.Fatalf("expected the superseded first code to be rejected, got %v", err)
	}

	user, _, _, _, err := svc.Verify(ctx, addr, secondCode, "test-agent", time.Hour)
	if err != nil {
		t.Fatalf("expected the latest code to verify, got %v", err)
	}
	cleanupUser(t, pool, user.ID)
}

func TestVerifyExpiredCodeFails(t *testing.T) {
	svc, _, fakeEmail, pool := testServices(t, 10*time.Millisecond)
	ctx := context.Background()
	addr := uniqueEmail()
	cleanupSignup(t, pool, addr)

	if err := svc.Start(ctx, addr, "Ada Obi", "password123"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	code := fakeEmail.lastCode(t)

	time.Sleep(50 * time.Millisecond)

	if _, _, _, _, err := svc.Verify(ctx, addr, code, "test-agent", time.Hour); err != signup.ErrInvalidOrExpiredCode {
		t.Fatalf("expected ErrInvalidOrExpiredCode for an expired code, got %v", err)
	}
}
