package invitations_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/email"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/invitations"
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

func (f *fakeEmailProvider) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
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

func (f *fakeWhatsAppProvider) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
}

func testServices(t *testing.T) (*invitations.Service, *identity.Service, *fakeEmailProvider, *fakeWhatsAppProvider, *pgxpool.Pool) {
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
	invSvc := invitations.New(pool, identitySvc, fakeEmail, fakeWA, "https://justbarme.test")
	return invSvc, identitySvc, fakeEmail, fakeWA, pool
}

func uniqueEmail() string { return uuid.New().String() + "@example.com" }
func uniquePhone() string { return "+234" + uuid.New().String()[:10] }

func cleanupInvitation(t *testing.T, pool *pgxpool.Pool, businessID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			return
		}
		defer conn.Release()
		_, _ = conn.Exec(context.Background(), "DELETE FROM invitations WHERE business_id = $1", businessID)
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
		_, _ = conn.Exec(context.Background(), "DELETE FROM business_memberships WHERE user_id = $1", userID)
		_, _ = conn.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})
}

func cleanupBusiness(t *testing.T, pool *pgxpool.Pool, ownerID, businessID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			return
		}
		defer conn.Release()
		_, _ = conn.Exec(context.Background(), "DELETE FROM locations WHERE business_id = $1", businessID)
		_, _ = conn.Exec(context.Background(), "DELETE FROM business_memberships WHERE business_id = $1", businessID)
		_, _ = conn.Exec(context.Background(), "DELETE FROM businesses WHERE id = $1", businessID)
	})
}

func newOwnerWithBusiness(t *testing.T, ctx context.Context, identitySvc *identity.Service, pool *pgxpool.Pool) (identity.User, identity.Business) {
	t.Helper()
	owner, err := identitySvc.CreateUser(ctx, uniqueEmail(), "", "Test Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupUser(t, pool, owner.ID)
	business, _, _, err := identitySvc.CreateBusinessWithOwner(ctx, owner.ID, "Test Bar "+uuid.New().String())
	if err != nil {
		t.Fatalf("CreateBusinessWithOwner: %v", err)
	}
	cleanupBusiness(t, pool, owner.ID, business.ID)
	cleanupInvitation(t, pool, business.ID)
	return owner, business
}

// extractToken pulls the raw invitation token back out of a delivered
// message -- the only place it ever appears in plaintext, since only its
// hash is stored server-side (same pattern as sessions/signup codes).
func extractToken(t *testing.T, message, baseURL string) string {
	t.Helper()
	marker := baseURL + "/invite/"
	idx := indexOf(message, marker)
	if idx < 0 {
		t.Fatalf("could not find invite link in message: %q", message)
	}
	return message[idx+len(marker):]
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestCreateAndAcceptAsNewUserViaWhatsApp(t *testing.T) {
	svc, identitySvc, _, fakeWA, pool := testServices(t)
	ctx := context.Background()
	owner, business := newOwnerWithBusiness(t, ctx, identitySvc, pool)
	phone := uniquePhone()

	inv, err := svc.Create(ctx, business.ID, owner.ID, phone, "", invitations.RoleStaff)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inv.Status != invitations.StatusPending || inv.Phone != phone {
		t.Fatalf("unexpected invitation: %+v", inv)
	}
	if fakeWA.count() != 1 {
		t.Fatalf("expected exactly one whatsapp message, got %d", fakeWA.count())
	}

	fakeWA.mu.Lock()
	token := extractToken(t, fakeWA.messages[0].Text, "https://justbarme.test")
	fakeWA.mu.Unlock()

	fetched, err := svc.GetByToken(ctx, token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if fetched.ID != inv.ID {
		t.Fatalf("expected to fetch the same invitation, got %+v", fetched)
	}

	user, accepted, rawSession, rawCSRF, sess, err := svc.AcceptAsNewUser(ctx, token, "New Staff", "password123", "test-agent", time.Hour)
	if err != nil {
		t.Fatalf("AcceptAsNewUser: %v", err)
	}
	cleanupUser(t, pool, user.ID)
	if user.Phone != phone || user.DisplayName != "New Staff" {
		t.Fatalf("unexpected created user: %+v", user)
	}
	if accepted.Status != invitations.StatusAccepted || accepted.AcceptedBy != user.ID {
		t.Fatalf("expected invitation marked accepted by the new user, got %+v", accepted)
	}
	if rawSession == "" || rawCSRF == "" || sess.UserID != user.ID {
		t.Fatalf("expected a real session, got session=%q csrf=%q sess=%+v", rawSession, rawCSRF, sess)
	}

	membership, err := identitySvc.GetMembership(ctx, user.ID, business.ID)
	if err != nil {
		t.Fatalf("GetMembership: %v", err)
	}
	if membership.Role != invitations.RoleStaff {
		t.Fatalf("expected staff role, got %q", membership.Role)
	}

	// Accepting again must fail -- it's no longer pending.
	if _, _, _, _, _, err := svc.AcceptAsNewUser(ctx, token, "Someone Else", "password123", "test-agent", time.Hour); err != invitations.ErrInvitationNotOpen {
		t.Fatalf("expected ErrInvitationNotOpen for a re-accept, got %v", err)
	}
}

func TestCreateViaEmailWhenNoPhoneGiven(t *testing.T) {
	svc, identitySvc, fakeEmail, fakeWA, pool := testServices(t)
	ctx := context.Background()
	owner, business := newOwnerWithBusiness(t, ctx, identitySvc, pool)
	addr := uniqueEmail()

	if _, err := svc.Create(ctx, business.ID, owner.ID, "", addr, invitations.RoleStaff); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if fakeEmail.count() != 1 {
		t.Fatalf("expected exactly one email sent, got %d", fakeEmail.count())
	}
	if fakeWA.count() != 0 {
		t.Fatalf("expected no whatsapp message when only an email was given, got %d", fakeWA.count())
	}
}

func TestCreateRejectsDuplicatePendingInvite(t *testing.T) {
	svc, identitySvc, _, _, pool := testServices(t)
	ctx := context.Background()
	owner, business := newOwnerWithBusiness(t, ctx, identitySvc, pool)
	phone := uniquePhone()

	if _, err := svc.Create(ctx, business.ID, owner.ID, phone, "", invitations.RoleStaff); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := svc.Create(ctx, business.ID, owner.ID, phone, "", invitations.RoleStaff); err != invitations.ErrAlreadyPending {
		t.Fatalf("expected ErrAlreadyPending, got %v", err)
	}
}

func TestRevokeClosesAPendingInvitation(t *testing.T) {
	svc, identitySvc, _, _, pool := testServices(t)
	ctx := context.Background()
	owner, business := newOwnerWithBusiness(t, ctx, identitySvc, pool)

	inv, err := svc.Create(ctx, business.ID, owner.ID, uniquePhone(), "", invitations.RoleStaff)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	revoked, err := svc.Revoke(ctx, owner.ID, business.ID, inv.ID)
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if revoked.Status != invitations.StatusRevoked {
		t.Fatalf("expected revoked status, got %q", revoked.Status)
	}

	if _, err := svc.Revoke(ctx, owner.ID, business.ID, inv.ID); err != invitations.ErrInvitationNotOpen {
		t.Fatalf("expected ErrInvitationNotOpen for a second revoke, got %v", err)
	}
}

func TestAcceptAsExistingUserAttachesMembershipWithoutCreatingAccount(t *testing.T) {
	svc, identitySvc, _, fakeWA, pool := testServices(t)
	ctx := context.Background()
	owner, business := newOwnerWithBusiness(t, ctx, identitySvc, pool)

	existingUser, err := identitySvc.CreateUser(ctx, uniqueEmail(), "", "Existing Person", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupUser(t, pool, existingUser.ID)

	if _, err := svc.Create(ctx, business.ID, owner.ID, uniquePhone(), "", invitations.RoleStaff); err != nil {
		t.Fatalf("Create: %v", err)
	}

	fakeWA.mu.Lock()
	token := extractToken(t, fakeWA.messages[len(fakeWA.messages)-1].Text, "https://justbarme.test")
	fakeWA.mu.Unlock()

	// Simulate the HTTP layer already having verified the invitee's
	// identifier belongs to existingUser (internal/verification's job) --
	// AcceptAsExistingUser only finalizes the grant, never creating a
	// second account.
	accepted, err := svc.AcceptAsExistingUser(ctx, token, existingUser.ID)
	if err != nil {
		t.Fatalf("AcceptAsExistingUser: %v", err)
	}
	if accepted.Status != invitations.StatusAccepted || accepted.AcceptedBy != existingUser.ID {
		t.Fatalf("expected invitation accepted by the existing user, got %+v", accepted)
	}

	membership, err := identitySvc.GetMembership(ctx, existingUser.ID, business.ID)
	if err != nil {
		t.Fatalf("GetMembership: %v", err)
	}
	if membership.Role != invitations.RoleStaff {
		t.Fatalf("expected staff role, got %q", membership.Role)
	}

	list, err := svc.List(ctx, owner.ID, business.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Status != invitations.StatusAccepted {
		t.Fatalf("expected exactly one accepted invitation, got %+v", list)
	}
}

func TestAcceptAsExistingUserTreatsAlreadyMemberAsFulfilled(t *testing.T) {
	svc, identitySvc, _, fakeWA, pool := testServices(t)
	ctx := context.Background()
	owner, business := newOwnerWithBusiness(t, ctx, identitySvc, pool)

	existingUser, err := identitySvc.CreateUser(ctx, uniqueEmail(), "", "Already Staff", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupUser(t, pool, existingUser.ID)
	if _, err := identitySvc.AddMembership(ctx, business.ID, existingUser.ID, invitations.RoleStaff); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	if _, err := svc.Create(ctx, business.ID, owner.ID, uniquePhone(), "", invitations.RoleStaff); err != nil {
		t.Fatalf("Create: %v", err)
	}
	fakeWA.mu.Lock()
	token := extractToken(t, fakeWA.messages[len(fakeWA.messages)-1].Text, "https://justbarme.test")
	fakeWA.mu.Unlock()

	// existingUser is already staff at this business -- accepting must
	// still succeed (the invitation is fulfilled, not rejected) rather
	// than erroring on the redundant membership.
	accepted, err := svc.AcceptAsExistingUser(ctx, token, existingUser.ID)
	if err != nil {
		t.Fatalf("AcceptAsExistingUser should treat an already-member as fulfilled, got error: %v", err)
	}
	if accepted.Status != invitations.StatusAccepted {
		t.Fatalf("expected the invitation marked accepted, got %+v", accepted)
	}
}
