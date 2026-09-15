package identity_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/identity"
)

// testService returns an identity.Service backed by a real PostgreSQL
// database, or skips the test if JBM_DATABASE_URL is not set. Argon2 costs
// are trivially small so hashing stays fast across many tests.
func testService(t *testing.T) (*identity.Service, *pgxpool.Pool) {
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
	return identity.New(pool, config.Argon2{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1}), pool
}

func uniqueEmail() string {
	return uuid.New().String() + "@example.com"
}

func cleanupUser(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			return
		}
		defer conn.Release()
		// Capture owned businesses before removing the membership rows that
		// link them to userID — the reverse order would leave the
		// subqueries below with nothing to find.
		var businessIDs []uuid.UUID
		rows, err := conn.Query(context.Background(), "SELECT business_id FROM business_memberships WHERE user_id = $1", userID)
		if err == nil {
			for rows.Next() {
				var id uuid.UUID
				if rows.Scan(&id) == nil {
					businessIDs = append(businessIDs, id)
				}
			}
			rows.Close()
		}

		_, _ = conn.Exec(context.Background(), "DELETE FROM sessions WHERE user_id = $1", userID)
		_, _ = conn.Exec(context.Background(), "DELETE FROM devices WHERE user_id = $1", userID)
		_, _ = conn.Exec(context.Background(), "DELETE FROM business_memberships WHERE user_id = $1", userID)
		for _, businessID := range businessIDs {
			_, _ = conn.Exec(context.Background(), "DELETE FROM devices WHERE business_id = $1", businessID)
			_, _ = conn.Exec(context.Background(), "DELETE FROM locations WHERE business_id = $1", businessID)
			_, _ = conn.Exec(context.Background(), "DELETE FROM businesses WHERE id = $1", businessID)
		}
		_, _ = conn.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})
}

func TestCreateUserAndAuthenticate(t *testing.T) {
	svc, pool := testService(t)
	ctx := context.Background()
	email := uniqueEmail()

	created, err := svc.CreateUser(ctx, email, "", "Ada Obi", "correct horse battery staple")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupUser(t, pool, created.ID)

	authenticated, err := svc.Authenticate(ctx, email, "correct horse battery staple")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if authenticated.ID != created.ID {
		t.Fatalf("expected authenticated user %s, got %s", created.ID, authenticated.ID)
	}

	if _, err := svc.Authenticate(ctx, email, "wrong password"); err != identity.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials for wrong password, got %v", err)
	}
	if _, err := svc.Authenticate(ctx, uniqueEmail(), "anything"); err != identity.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials for unknown email, got %v", err)
	}
}

func TestCreateUserDuplicateEmailRejected(t *testing.T) {
	svc, pool := testService(t)
	ctx := context.Background()
	email := uniqueEmail()

	first, err := svc.CreateUser(ctx, email, "", "First", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupUser(t, pool, first.ID)

	if _, err := svc.CreateUser(ctx, email, "", "Second", "password123"); err != identity.ErrEmailTaken {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}

	// Case-insensitive: the same email in a different case must also be
	// rejected, not treated as a distinct address.
	upper := strings.ToUpper(email)
	if _, err := svc.CreateUser(ctx, upper, "", "Third", "password123"); err != identity.ErrEmailTaken {
		t.Fatalf("expected ErrEmailTaken for a case-variant duplicate, got %v", err)
	}
}

func TestCreateBusinessWithOwnerIsAtomic(t *testing.T) {
	svc, pool := testService(t)
	ctx := context.Background()

	owner, err := svc.CreateUser(ctx, uniqueEmail(), "", "Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupUser(t, pool, owner.ID)

	business, membership, location, err := svc.CreateBusinessWithOwner(ctx, owner.ID, "The Place")
	if err != nil {
		t.Fatalf("CreateBusinessWithOwner: %v", err)
	}
	if business.Name != "The Place" {
		t.Errorf("expected business name %q, got %q", "The Place", business.Name)
	}
	if membership.Role != "owner" || membership.Status != "active" {
		t.Errorf("expected an active owner membership, got %+v", membership)
	}
	if !location.IsDefault || location.BusinessID != business.ID {
		t.Errorf("expected a default location for the new business, got %+v", location)
	}

	got, err := svc.GetMembership(ctx, owner.ID, business.ID)
	if err != nil {
		t.Fatalf("GetMembership: %v", err)
	}
	if got.Role != "owner" {
		t.Errorf("expected role owner, got %q", got.Role)
	}

	memberships, err := svc.ListMembershipsForUser(ctx, owner.ID)
	if err != nil {
		t.Fatalf("ListMembershipsForUser: %v", err)
	}
	if len(memberships) != 1 || memberships[0].BusinessName != "The Place" {
		t.Fatalf("expected exactly one membership naming the new business, got %+v", memberships)
	}
}

func TestGetMembershipForUnrelatedBusinessFails(t *testing.T) {
	svc, pool := testService(t)
	ctx := context.Background()

	ownerA, err := svc.CreateUser(ctx, uniqueEmail(), "", "Owner A", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupUser(t, pool, ownerA.ID)
	ownerB, err := svc.CreateUser(ctx, uniqueEmail(), "", "Owner B", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupUser(t, pool, ownerB.ID)

	_, _, _, err = svc.CreateBusinessWithOwner(ctx, ownerA.ID, "Business A")
	if err != nil {
		t.Fatalf("CreateBusinessWithOwner A: %v", err)
	}
	businessB, _, _, err := svc.CreateBusinessWithOwner(ctx, ownerB.ID, "Business B")
	if err != nil {
		t.Fatalf("CreateBusinessWithOwner B: %v", err)
	}

	if _, err := svc.GetMembership(ctx, ownerA.ID, businessB.ID); err != identity.ErrMembershipNotFound {
		t.Fatalf("expected ErrMembershipNotFound for owner A in business B, got %v", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	svc, pool := testService(t)
	ctx := context.Background()

	user, err := svc.CreateUser(ctx, uniqueEmail(), "", "Session User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupUser(t, pool, user.ID)

	rawToken, rawCSRF, sess, err := svc.CreateSession(ctx, user.ID, "test-agent", time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if rawToken == "" || rawCSRF == "" {
		t.Fatal("expected non-empty raw session and CSRF tokens")
	}
	if sess.UserID != user.ID {
		t.Fatalf("expected session for user %s, got %s", user.ID, sess.UserID)
	}

	looked, err := svc.GetActiveSessionByToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("GetActiveSessionByToken: %v", err)
	}
	if looked.ID != sess.ID {
		t.Fatalf("expected to resolve session %s, got %s", sess.ID, looked.ID)
	}

	if _, err := svc.GetActiveSessionByToken(ctx, "not-a-real-token"); err != identity.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound for a bogus token, got %v", err)
	}

	if err := svc.RevokeSession(ctx, user.ID, sess.ID, "logout"); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if _, err := svc.GetActiveSessionByToken(ctx, rawToken); err != identity.ErrSessionNotFound {
		t.Fatalf("expected revoked session to be unresolvable, got %v", err)
	}

	// Revoking again, or revoking someone else's session ID, must both fail
	// rather than silently succeed.
	if err := svc.RevokeSession(ctx, user.ID, sess.ID, "logout again"); err != identity.ErrSessionNotFound {
		t.Fatalf("expected re-revoking to report ErrSessionNotFound, got %v", err)
	}

	otherUser, err := svc.CreateUser(ctx, uniqueEmail(), "", "Other User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupUser(t, pool, otherUser.ID)
	_, _, otherSess, err := svc.CreateSession(ctx, otherUser.ID, "test-agent", time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := svc.RevokeSession(ctx, user.ID, otherSess.ID, "attempted takeover"); err != identity.ErrSessionNotFound {
		t.Fatalf("expected revoking another user's session to fail, got %v", err)
	}
}

func TestDeviceLifecycle(t *testing.T) {
	svc, pool := testService(t)
	ctx := context.Background()

	owner, err := svc.CreateUser(ctx, uniqueEmail(), "", "Device Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupUser(t, pool, owner.ID)

	business, _, location, err := svc.CreateBusinessWithOwner(ctx, owner.ID, "Device Test Bar")
	if err != nil {
		t.Fatalf("CreateBusinessWithOwner: %v", err)
	}

	publicKey := uuid.New().String()
	device, err := svc.CreateDevice(ctx, owner.ID, business.ID, location.ID, publicKey, "Owner's Phone")
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	if device.Status != "active" {
		t.Fatalf("expected newly enrolled device to be active, got %q", device.Status)
	}

	if _, err := svc.CreateDevice(ctx, owner.ID, business.ID, location.ID, publicKey, "Duplicate"); err != identity.ErrDevicePublicKeyTaken {
		t.Fatalf("expected ErrDevicePublicKeyTaken for a duplicate public key, got %v", err)
	}

	devices, err := svc.ListDevices(ctx, owner.ID, business.ID)
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected exactly one device, got %d", len(devices))
	}

	revoked, err := svc.RevokeDevice(ctx, owner.ID, business.ID, device.ID)
	if err != nil {
		t.Fatalf("RevokeDevice: %v", err)
	}
	if revoked.Status != "revoked" {
		t.Fatalf("expected revoked status, got %q", revoked.Status)
	}

	if _, err := svc.RevokeDevice(ctx, owner.ID, business.ID, device.ID); err != identity.ErrDeviceNotFound {
		t.Fatalf("expected re-revoking an already-revoked device to report ErrDeviceNotFound, got %v", err)
	}
}
