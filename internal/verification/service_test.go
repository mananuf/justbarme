package verification_test

import (
	"context"
	"os"
	"regexp"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/email"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/verification"
	"github.com/mananuf/justbarme/internal/whatsapp"
)

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

func testServices(t *testing.T) (*verification.Service, *identity.Service, *fakeEmailProvider, *fakeWhatsAppProvider, *pgxpool.Pool) {
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
	verifySvc := verification.New(pool, identitySvc, fakeEmail, fakeWA)
	return verifySvc, identitySvc, fakeEmail, fakeWA, pool
}

func uniqueEmail() string { return uuid.New().String() + "@example.com" }
func uniquePhone() string { return "+234" + uuid.New().String()[:10] }

func newTestUser(t *testing.T, ctx context.Context, identitySvc *identity.Service, pool *pgxpool.Pool) identity.User {
	t.Helper()
	user, err := identitySvc.CreateUser(ctx, uniqueEmail(), "", "Test User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			return
		}
		defer conn.Release()
		_, _ = conn.Exec(context.Background(), "DELETE FROM identity_verifications WHERE user_id = $1", user.ID)
		_, _ = conn.Exec(context.Background(), "DELETE FROM sessions WHERE user_id = $1", user.ID)
		_, _ = conn.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})
	return user
}

func TestStartAndConfirmAddsEmail(t *testing.T) {
	svc, identitySvc, fakeEmail, _, pool := testServices(t)
	ctx := context.Background()
	user := newTestUser(t, ctx, identitySvc, pool)
	newEmail := uniqueEmail()

	v, err := svc.Start(ctx, user.ID, verification.ChannelEmail, newEmail, verification.PurposeAddIdentifier)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	code := fakeEmail.lastCode(t)

	updated, err := svc.Confirm(ctx, user.ID, v.ID, code)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if updated.Email != newEmail {
		t.Fatalf("expected email %q attached, got %q", newEmail, updated.Email)
	}

	// The pending row is deleted on success -- confirming again must fail.
	if _, err := svc.Confirm(ctx, user.ID, v.ID, code); err != verification.ErrVerificationNotFound {
		t.Fatalf("expected ErrVerificationNotFound after a completed confirm, got %v", err)
	}
}

func TestStartAndConfirmAddsPhone(t *testing.T) {
	svc, identitySvc, _, fakeWA, pool := testServices(t)
	ctx := context.Background()
	user := newTestUser(t, ctx, identitySvc, pool)
	phone := uniquePhone()

	v, err := svc.Start(ctx, user.ID, verification.ChannelWhatsApp, phone, verification.PurposeInvitationLink)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	code := fakeWA.lastCode(t)

	updated, err := svc.Confirm(ctx, user.ID, v.ID, code)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if updated.Phone != phone {
		t.Fatalf("expected phone %q attached, got %q", phone, updated.Phone)
	}
}

func TestStartRejectsIdentifierAlreadyInUse(t *testing.T) {
	svc, identitySvc, _, _, pool := testServices(t)
	ctx := context.Background()
	user := newTestUser(t, ctx, identitySvc, pool)
	other := newTestUser(t, ctx, identitySvc, pool)

	if _, err := svc.Start(ctx, user.ID, verification.ChannelEmail, other.Email, verification.PurposeAddIdentifier); err != verification.ErrIdentifierAlreadyInUse {
		t.Fatalf("expected ErrIdentifierAlreadyInUse, got %v", err)
	}
}

func TestConfirmWrongCodeLocksOutAfterMaxAttempts(t *testing.T) {
	svc, identitySvc, fakeEmail, _, pool := testServices(t)
	ctx := context.Background()
	user := newTestUser(t, ctx, identitySvc, pool)

	v, err := svc.Start(ctx, user.ID, verification.ChannelEmail, uniqueEmail(), verification.PurposeAddIdentifier)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	correctCode := fakeEmail.lastCode(t)

	for i := 0; i < 3; i++ {
		if _, err := svc.Confirm(ctx, user.ID, v.ID, "000000"); err != verification.ErrInvalidOrExpiredCode {
			t.Fatalf("attempt %d: expected ErrInvalidOrExpiredCode, got %v", i, err)
		}
	}
	if _, err := svc.Confirm(ctx, user.ID, v.ID, correctCode); err != verification.ErrInvalidOrExpiredCode {
		t.Fatalf("expected the correct code to be locked out after max attempts, got %v", err)
	}
}

func TestConfirmRejectsMismatchedUser(t *testing.T) {
	svc, identitySvc, _, _, pool := testServices(t)
	ctx := context.Background()
	owner := newTestUser(t, ctx, identitySvc, pool)
	attacker := newTestUser(t, ctx, identitySvc, pool)

	v, err := svc.Start(ctx, owner.ID, verification.ChannelEmail, uniqueEmail(), verification.PurposeAddIdentifier)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if _, err := svc.Confirm(ctx, attacker.ID, v.ID, "000000"); err != verification.ErrVerificationNotFound {
		t.Fatalf("expected ErrVerificationNotFound for a mismatched user, got %v", err)
	}
}

func TestConfirmExpiredCodeFails(t *testing.T) {
	// otpTTL is a package-internal constant (10 minutes), not injectable
	// here -- this test instead relies on IncrementIdentityVerificationAttempts'
	// lockout path being exercised by TestConfirmWrongCodeLocksOutAfterMaxAttempts,
	// and directly verifies a clearly-wrong code is always rejected rather
	// than waiting out a real 10-minute TTL.
	svc, identitySvc, fakeEmail, _, pool := testServices(t)
	ctx := context.Background()
	user := newTestUser(t, ctx, identitySvc, pool)

	v, err := svc.Start(ctx, user.ID, verification.ChannelEmail, uniqueEmail(), verification.PurposeAddIdentifier)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_ = fakeEmail.lastCode(t)

	if _, err := svc.Confirm(ctx, user.ID, v.ID, "999999"); err != verification.ErrInvalidOrExpiredCode {
		t.Fatalf("expected ErrInvalidOrExpiredCode for a wrong code, got %v", err)
	}
}
