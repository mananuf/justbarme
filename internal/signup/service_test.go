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
	"github.com/mananuf/justbarme/internal/whatsapp"
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

// fakeWhatsAppProvider mirrors fakeEmailProvider for the WhatsApp channel.
type fakeWhatsAppProvider struct {
	mu       sync.Mutex
	messages []whatsapp.Message
}

func (f *fakeWhatsAppProvider) Send(_ context.Context, msg whatsapp.Message) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, msg)
	return "fake-message-id", nil
}

func (f *fakeWhatsAppProvider) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
}

func (f *fakeWhatsAppProvider) lastCode(t *testing.T) string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.messages) == 0 {
		t.Fatal("no whatsapp message was sent")
	}
	code := otpPattern.FindString(f.messages[len(f.messages)-1].Text)
	if code == "" {
		t.Fatalf("could not find a 6-digit code in whatsapp message: %q", f.messages[len(f.messages)-1].Text)
	}
	return code
}

// testServices returns a signup.Service (with fake email/whatsapp
// providers so tests can read the OTP back out) and the identity.Service
// it wraps, sharing one real PostgreSQL pool, or skips if
// JBM_DATABASE_URL is unset.
func testServices(t *testing.T, otpTTL time.Duration) (*signup.Service, *identity.Service, *fakeEmailProvider, *fakeWhatsAppProvider, *pgxpool.Pool) {
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
	fakeWA := &fakeWhatsAppProvider{}
	signupSvc := signup.New(pool, argon2, identitySvc, fakeEmail, fakeWA, otpTTL)
	return signupSvc, identitySvc, fakeEmail, fakeWA, pool
}

func uniqueEmail() string {
	return uuid.New().String() + "@example.com"
}

func uniquePhone() string {
	// +234 (Nigeria) followed by digits derived from a random UUID, kept
	// short enough to look like a real E.164 number.
	return "+234" + uuid.New().String()[:10]
}

func cleanupSignup(t *testing.T, pool *pgxpool.Pool, identifier string) {
	t.Helper()
	t.Cleanup(func() {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			return
		}
		defer conn.Release()
		_, _ = conn.Exec(context.Background(), "DELETE FROM signup_verifications WHERE lower(email) = lower($1) OR phone = $1", identifier)
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
	svc, _, fakeEmail, _, pool := testServices(t, time.Hour)
	ctx := context.Background()
	addr := uniqueEmail()
	cleanupSignup(t, pool, addr)

	if err := svc.Start(ctx, signup.ChannelEmail, addr, "Ada Obi", "password123"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if fakeEmail.count() != 1 {
		t.Fatalf("expected exactly one email sent, got %d", fakeEmail.count())
	}
	code := fakeEmail.lastCode(t)

	user, rawToken, rawCSRF, sess, err := svc.Verify(ctx, signup.ChannelEmail, addr, code, "test-agent", time.Hour)
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
	if _, _, _, _, err := svc.Verify(ctx, signup.ChannelEmail, addr, code, "test-agent", time.Hour); err != signup.ErrNoPendingSignup {
		t.Fatalf("expected ErrNoPendingSignup after a completed signup, got %v", err)
	}
}

func TestStartAndVerifySignupViaWhatsApp(t *testing.T) {
	svc, _, _, fakeWA, pool := testServices(t, time.Hour)
	ctx := context.Background()
	phone := uniquePhone()
	cleanupSignup(t, pool, phone)

	if err := svc.Start(ctx, signup.ChannelWhatsApp, phone, "Bello Musa", "password123"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if fakeWA.count() != 1 {
		t.Fatalf("expected exactly one whatsapp message sent, got %d", fakeWA.count())
	}
	code := fakeWA.lastCode(t)

	user, rawToken, rawCSRF, sess, err := svc.Verify(ctx, signup.ChannelWhatsApp, phone, code, "test-agent", time.Hour)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	cleanupUser(t, pool, user.ID)

	if user.Phone != phone || user.Email != "" || user.DisplayName != "Bello Musa" {
		t.Fatalf("unexpected created user: %+v", user)
	}
	if rawToken == "" || rawCSRF == "" || sess.UserID != user.ID {
		t.Fatalf("expected a real session for the new user, got token=%q csrf=%q sess=%+v", rawToken, rawCSRF, sess)
	}
}

func TestStartRejectsAlreadyRegisteredEmail(t *testing.T) {
	svc, identitySvc, fakeEmail, _, pool := testServices(t, time.Hour)
	ctx := context.Background()
	addr := uniqueEmail()

	existing, err := identitySvc.CreateUser(ctx, addr, "", "Existing User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupUser(t, pool, existing.ID)

	if err := svc.Start(ctx, signup.ChannelEmail, addr, "Someone Else", "password123"); err != signup.ErrAlreadyRegistered {
		t.Fatalf("expected ErrAlreadyRegistered, got %v", err)
	}
	if fakeEmail.count() != 0 {
		t.Fatal("expected no email to be sent for an already-registered address")
	}
}

func TestVerifyNoPendingSignupForUnknownEmail(t *testing.T) {
	svc, _, _, _, _ := testServices(t, time.Hour)
	if _, _, _, _, err := svc.Verify(context.Background(), signup.ChannelEmail, uniqueEmail(), "123456", "test-agent", time.Hour); err != signup.ErrNoPendingSignup {
		t.Fatalf("expected ErrNoPendingSignup, got %v", err)
	}
}

func TestVerifyWrongCodeLocksOutAfterMaxAttempts(t *testing.T) {
	svc, _, fakeEmail, _, pool := testServices(t, time.Hour)
	ctx := context.Background()
	addr := uniqueEmail()
	cleanupSignup(t, pool, addr)

	if err := svc.Start(ctx, signup.ChannelEmail, addr, "Ada Obi", "password123"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	correctCode := fakeEmail.lastCode(t)

	// Five wrong attempts, matching the service's own maxOTPAttempts.
	for i := 0; i < 5; i++ {
		if _, _, _, _, err := svc.Verify(ctx, signup.ChannelEmail, addr, "000000", "test-agent", time.Hour); err != signup.ErrInvalidOrExpiredCode {
			t.Fatalf("attempt %d: expected ErrInvalidOrExpiredCode, got %v", i, err)
		}
	}

	// The correct code must now also fail -- attempts exhausted, not just
	// individual wrong guesses.
	if _, _, _, _, err := svc.Verify(ctx, signup.ChannelEmail, addr, correctCode, "test-agent", time.Hour); err != signup.ErrInvalidOrExpiredCode {
		t.Fatalf("expected the correct code to be locked out after max attempts, got %v", err)
	}
}

func TestStartAgainReplacesThePreviousCode(t *testing.T) {
	svc, _, fakeEmail, _, pool := testServices(t, time.Hour)
	ctx := context.Background()
	addr := uniqueEmail()
	cleanupSignup(t, pool, addr)

	if err := svc.Start(ctx, signup.ChannelEmail, addr, "Ada Obi", "password123"); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	firstCode := fakeEmail.lastCode(t)

	if err := svc.Start(ctx, signup.ChannelEmail, addr, "Ada Obi", "password123"); err != nil {
		t.Fatalf("second Start (resend): %v", err)
	}
	secondCode := fakeEmail.lastCode(t)

	if _, _, _, _, err := svc.Verify(ctx, signup.ChannelEmail, addr, firstCode, "test-agent", time.Hour); err != signup.ErrInvalidOrExpiredCode {
		t.Fatalf("expected the superseded first code to be rejected, got %v", err)
	}

	user, _, _, _, err := svc.Verify(ctx, signup.ChannelEmail, addr, secondCode, "test-agent", time.Hour)
	if err != nil {
		t.Fatalf("expected the latest code to verify, got %v", err)
	}
	cleanupUser(t, pool, user.ID)
}

func TestVerifyExpiredCodeFails(t *testing.T) {
	svc, _, fakeEmail, _, pool := testServices(t, 10*time.Millisecond)
	ctx := context.Background()
	addr := uniqueEmail()
	cleanupSignup(t, pool, addr)

	if err := svc.Start(ctx, signup.ChannelEmail, addr, "Ada Obi", "password123"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	code := fakeEmail.lastCode(t)

	time.Sleep(50 * time.Millisecond)

	if _, _, _, _, err := svc.Verify(ctx, signup.ChannelEmail, addr, code, "test-agent", time.Hour); err != signup.ErrInvalidOrExpiredCode {
		t.Fatalf("expected ErrInvalidOrExpiredCode for an expired code, got %v", err)
	}
}
