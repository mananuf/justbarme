package platformadmin_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/platformadmin"
)

// testServices returns a platformadmin.Service and an identity.Service
// (used only to create a real business to point platform-oversight
// operations at) sharing one real PostgreSQL pool, or skips the test if
// JBM_DATABASE_URL is not set.
func testServices(t *testing.T) (*platformadmin.Service, *identity.Service, *pgxpool.Pool) {
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
	return platformadmin.New(pool, argon2), identity.New(pool, argon2), pool
}

func uniqueEmail() string {
	return uuid.New().String() + "@example.com"
}

func cleanupStaff(t *testing.T, pool *pgxpool.Pool, staffID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			return
		}
		defer conn.Release()
		_, _ = conn.Exec(context.Background(), "DELETE FROM platform_audit_log WHERE platform_staff_id = $1 OR target_staff_id = $1", staffID)
		_, _ = conn.Exec(context.Background(), "DELETE FROM platform_sessions WHERE staff_id = $1", staffID)
		_, _ = conn.Exec(context.Background(), "DELETE FROM platform_staff WHERE id = $1", staffID)
	})
}

func cleanupBusiness(t *testing.T, pool *pgxpool.Pool, ownerID, businessID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return
		}
		defer conn.Release()

		_, _ = conn.Exec(ctx, "UPDATE platform_audit_log SET target_business_id = NULL WHERE target_business_id = $1", businessID)

		// devices/locations/business_memberships are RLS-scoped (FORCE ROW
		// LEVEL SECURITY): a raw DELETE on a bare connection only actually
		// removes rows if the connecting role happens to be an outright
		// Postgres superuser -- true for a local dev role like the table
		// owner, but not for an ordinary app-facing role such as
		// JBM_DATABASE_URL might name (e.g. after switching to a
		// jbm_app-member role for a more production-like local setup).
		// Under RLS the DELETE would otherwise silently affect zero rows
		// and leave the business/membership/user behind, later blocking
		// (or masking) other tests via the businesses foreign key. Setting
		// the same role and app.business_id a real request would set
		// (store.WithTenant's own mechanics, inlined here since sqlc.Queries
		// exposes no raw-SQL escape hatch for a table it has no query
		// against) makes this cleanup correct regardless of which kind of
		// role the test pool connects as.
		tx, err := conn.Begin(ctx)
		if err == nil {
			_, _ = tx.Exec(ctx, "SET LOCAL ROLE jbm_app")
			_, _ = tx.Exec(ctx, "SELECT set_config('app.business_id', $1, true)", businessID.String())
			_, _ = tx.Exec(ctx, "DELETE FROM devices WHERE business_id = $1", businessID)
			_, _ = tx.Exec(ctx, "DELETE FROM locations WHERE business_id = $1", businessID)
			_, _ = tx.Exec(ctx, "DELETE FROM business_memberships WHERE business_id = $1", businessID)
			_ = tx.Commit(ctx)
		}

		// businesses/users carry no RLS at all (see their own migrations),
		// so a plain DELETE is correct on any role with table privileges.
		_, _ = conn.Exec(ctx, "DELETE FROM businesses WHERE id = $1", businessID)
		_, _ = conn.Exec(ctx, "DELETE FROM users WHERE id = $1", ownerID)
	})
}

func TestCreateStaffAndAuthenticate(t *testing.T) {
	svc, _, pool := testServices(t)
	ctx := context.Background()
	email := uniqueEmail()

	created, err := svc.CreateStaff(ctx, email, "Ada Support", "password123", platformadmin.RoleSupport)
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}
	cleanupStaff(t, pool, created.ID)

	authenticated, err := svc.Authenticate(ctx, email, "password123")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if authenticated.ID != created.ID {
		t.Fatalf("expected authenticated staff %s, got %s", created.ID, authenticated.ID)
	}

	if _, err := svc.Authenticate(ctx, email, "wrong password"); err != platformadmin.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials for wrong password, got %v", err)
	}
	if _, err := svc.Authenticate(ctx, uniqueEmail(), "anything"); err != platformadmin.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials for unknown email, got %v", err)
	}
}

func TestCreateStaffDuplicateEmailRejected(t *testing.T) {
	svc, _, pool := testServices(t)
	ctx := context.Background()
	email := uniqueEmail()

	first, err := svc.CreateStaff(ctx, email, "First", "password123", platformadmin.RoleSupport)
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}
	cleanupStaff(t, pool, first.ID)

	if _, err := svc.CreateStaff(ctx, email, "Second", "password123", platformadmin.RoleSuperadmin); err != platformadmin.ErrEmailTaken {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestPlatformSessionLifecycle(t *testing.T) {
	svc, _, pool := testServices(t)
	ctx := context.Background()

	staff, err := svc.CreateStaff(ctx, uniqueEmail(), "Session Staff", "password123", platformadmin.RoleSupport)
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}
	cleanupStaff(t, pool, staff.ID)

	rawToken, rawCSRF, sess, err := svc.CreateSession(ctx, staff.ID, "test-agent", time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if rawToken == "" || rawCSRF == "" {
		t.Fatal("expected non-empty raw session and CSRF tokens")
	}
	if sess.StaffID != staff.ID {
		t.Fatalf("expected session for staff %s, got %s", staff.ID, sess.StaffID)
	}

	looked, err := svc.GetActiveSessionByToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("GetActiveSessionByToken: %v", err)
	}
	if looked.ID != sess.ID {
		t.Fatalf("expected to resolve session %s, got %s", sess.ID, looked.ID)
	}

	if _, err := svc.GetActiveSessionByToken(ctx, "not-a-real-token"); err != platformadmin.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound for a bogus token, got %v", err)
	}

	rotatedCSRF, err := svc.RotateCSRFToken(ctx, sess.ID)
	if err != nil {
		t.Fatalf("RotateCSRFToken: %v", err)
	}
	if rotatedCSRF == "" || rotatedCSRF == rawCSRF {
		t.Fatalf("expected a new, non-empty CSRF token, got %q (original was %q)", rotatedCSRF, rawCSRF)
	}

	if err := svc.RevokeSession(ctx, staff.ID, sess.ID, "logout"); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if _, err := svc.GetActiveSessionByToken(ctx, rawToken); err != platformadmin.ErrSessionNotFound {
		t.Fatalf("expected revoked session to be unresolvable, got %v", err)
	}
	if err := svc.RevokeSession(ctx, staff.ID, sess.ID, "logout again"); err != platformadmin.ErrSessionNotFound {
		t.Fatalf("expected re-revoking to report ErrSessionNotFound, got %v", err)
	}

	otherStaff, err := svc.CreateStaff(ctx, uniqueEmail(), "Other Staff", "password123", platformadmin.RoleSupport)
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}
	cleanupStaff(t, pool, otherStaff.ID)
	_, _, otherSess, err := svc.CreateSession(ctx, otherStaff.ID, "test-agent", time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := svc.RevokeSession(ctx, staff.ID, otherSess.ID, "attempted takeover"); err != platformadmin.ErrSessionNotFound {
		t.Fatalf("expected revoking another staff member's session to fail, got %v", err)
	}
}

func TestCapabilitiesBySupportAndSuperadminRole(t *testing.T) {
	support := platformadmin.ForRole(platformadmin.RoleSupport)
	if !support.Has(platformadmin.CapabilityBusinessesRead) || !support.Has(platformadmin.CapabilityAuditRead) {
		t.Fatal("expected support to have read and audit-read capabilities")
	}
	if support.Has(platformadmin.CapabilityBusinessesSuspend) {
		t.Fatal("expected support NOT to have businesses:suspend -- oversight, never mutation")
	}

	superadmin := platformadmin.ForRole(platformadmin.RoleSuperadmin)
	if !superadmin.Has(platformadmin.CapabilityBusinessesRead) ||
		!superadmin.Has(platformadmin.CapabilityBusinessesSuspend) ||
		!superadmin.Has(platformadmin.CapabilityAuditRead) {
		t.Fatal("expected superadmin to have every platform capability")
	}

	if platformadmin.ForRole("root") != nil {
		t.Fatal("expected an unrecognized role to map to no capabilities")
	}
}

func TestListBusinessesAndSuspendReactivateIsAtomicAndAudited(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()

	owner, err := identitySvc.CreateUser(ctx, uniqueEmail(), "", "Business Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	business, _, _, err := identitySvc.CreateBusinessWithOwner(ctx, owner.ID, "Platform Oversight Test Bar "+uuid.New().String())
	if err != nil {
		t.Fatalf("CreateBusinessWithOwner: %v", err)
	}
	cleanupBusiness(t, pool, owner.ID, business.ID)

	staff, err := svc.CreateStaff(ctx, uniqueEmail(), "Oversight Staff", "password123", platformadmin.RoleSuperadmin)
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}
	cleanupStaff(t, pool, staff.ID)

	businesses, err := svc.ListBusinesses(ctx)
	if err != nil {
		t.Fatalf("ListBusinesses: %v", err)
	}
	found := false
	for _, b := range businesses {
		if b.ID == business.ID {
			found = true
			if b.Status != "active" {
				t.Fatalf("expected newly created business to be active, got %q", b.Status)
			}
		}
	}
	if !found {
		t.Fatal("expected the newly created business to appear in ListBusinesses")
	}

	suspended, err := svc.SuspendBusiness(ctx, staff.ID, business.ID, "non-payment", "req-1")
	if err != nil {
		t.Fatalf("SuspendBusiness: %v", err)
	}
	if suspended.Status != "suspended" {
		t.Fatalf("expected suspended status, got %q", suspended.Status)
	}

	reactivated, err := svc.ReactivateBusiness(ctx, staff.ID, business.ID, "payment received", "req-2")
	if err != nil {
		t.Fatalf("ReactivateBusiness: %v", err)
	}
	if reactivated.Status != "active" {
		t.Fatalf("expected active status after reactivation, got %q", reactivated.Status)
	}

	if _, err := svc.SuspendBusiness(ctx, staff.ID, uuid.New(), "reason", "req-3"); err != platformadmin.ErrBusinessNotFound {
		t.Fatalf("expected ErrBusinessNotFound for an unknown business, got %v", err)
	}

	entries, err := svc.ListAuditLog(ctx, 100)
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	var sawSuspend, sawReactivate bool
	for _, e := range entries {
		if e.StaffID != staff.ID || e.TargetBusinessID != business.ID {
			continue
		}
		switch e.Action {
		case platformadmin.ActionBusinessSuspended:
			sawSuspend = true
			if e.Reason != "non-payment" {
				t.Fatalf("expected suspend reason %q, got %q", "non-payment", e.Reason)
			}
		case platformadmin.ActionBusinessReactivated:
			sawReactivate = true
		}
	}
	if !sawSuspend || !sawReactivate {
		t.Fatalf("expected both a suspend and a reactivate audit entry for this business, sawSuspend=%v sawReactivate=%v", sawSuspend, sawReactivate)
	}
}

func TestCreateStaffAuditedRecordsAuditEntry(t *testing.T) {
	svc, _, pool := testServices(t)
	ctx := context.Background()

	actor, err := svc.CreateStaff(ctx, uniqueEmail(), "Actor", "password123", platformadmin.RoleSuperadmin)
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}
	cleanupStaff(t, pool, actor.ID)

	created, err := svc.CreateStaffAudited(ctx, actor.ID, uniqueEmail(), "New Hire", "password123", platformadmin.RoleSupport, "req-1")
	if err != nil {
		t.Fatalf("CreateStaffAudited: %v", err)
	}
	cleanupStaff(t, pool, created.ID)

	entries, err := svc.ListAuditLog(ctx, 50)
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == platformadmin.ActionStaffCreated && e.StaffID == actor.ID && e.TargetStaffID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a staff.created audit entry naming the actor and the new account")
	}

	if _, err := svc.CreateStaffAudited(ctx, actor.ID, created.Email, "Dup", "password123", platformadmin.RoleSupport, "req-2"); err != platformadmin.ErrEmailTaken {
		t.Fatalf("expected ErrEmailTaken for a duplicate email, got %v", err)
	}
}

func TestRevokeStaffEndsSessionsAndBlocksLogin(t *testing.T) {
	svc, _, pool := testServices(t)
	ctx := context.Background()

	actor, err := svc.CreateStaff(ctx, uniqueEmail(), "Actor", "password123", platformadmin.RoleSuperadmin)
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}
	cleanupStaff(t, pool, actor.ID)
	email := uniqueEmail()
	target, err := svc.CreateStaff(ctx, email, "Target", "password123", platformadmin.RoleSupport)
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}
	cleanupStaff(t, pool, target.ID)

	rawToken, _, _, err := svc.CreateSession(ctx, target.ID, "ua", time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	revoked, err := svc.RevokeStaff(ctx, actor.ID, target.ID, "left the team", "req-3")
	if err != nil {
		t.Fatalf("RevokeStaff: %v", err)
	}
	if revoked.Status != "revoked" {
		t.Fatalf("expected status revoked, got %q", revoked.Status)
	}
	if _, err := svc.GetActiveSessionByToken(ctx, rawToken); err != platformadmin.ErrSessionNotFound {
		t.Fatalf("expected the revoked staff's existing session to stop working immediately, got %v", err)
	}
	if _, err := svc.Authenticate(ctx, email, "password123"); err != platformadmin.ErrInvalidCredentials {
		t.Fatalf("expected a revoked account to be unable to log in, got %v", err)
	}

	if _, err := svc.RevokeStaff(ctx, actor.ID, actor.ID, "oops", "req-4"); err != platformadmin.ErrCannotRevokeSelf {
		t.Fatalf("expected ErrCannotRevokeSelf, got %v", err)
	}
	if _, err := svc.RevokeStaff(ctx, actor.ID, uuid.New(), "nobody", "req-5"); err != platformadmin.ErrStaffNotFound {
		t.Fatalf("expected ErrStaffNotFound for an unknown id, got %v", err)
	}
}

func TestSupportCanReadActivityButNeverManageStaff(t *testing.T) {
	support := platformadmin.ForRole(platformadmin.RoleSupport)
	super := platformadmin.ForRole(platformadmin.RoleSuperadmin)
	for _, c := range []platformadmin.Capability{
		platformadmin.CapabilityBusinessesReadActivity, platformadmin.CapabilityStaffRead,
	} {
		if !support.Has(c) || !super.Has(c) {
			t.Fatalf("both roles should hold read capability %s", c)
		}
	}
	for _, c := range []platformadmin.Capability{
		platformadmin.CapabilityStaffManage, platformadmin.CapabilityBusinessesSuspend,
	} {
		if support.Has(c) {
			t.Fatalf("support must never hold write capability %s", c)
		}
		if !super.Has(c) {
			t.Fatalf("superadmin should hold write capability %s", c)
		}
	}
}

func TestRecordBusinessActivityViewedAndStats(t *testing.T) {
	svc, idsvc, pool := testServices(t)
	ctx := context.Background()

	staff, err := svc.CreateStaff(ctx, uniqueEmail(), "Viewer", "password123", platformadmin.RoleSupport)
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}
	cleanupStaff(t, pool, staff.ID)
	owner, err := idsvc.CreateUser(ctx, uniqueEmail(), "", "Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	biz, _, _, err := idsvc.CreateBusinessWithOwner(ctx, owner.ID, "Oversight Test Bar")
	if err != nil {
		t.Fatalf("CreateBusinessWithOwner: %v", err)
	}
	cleanupBusiness(t, pool, owner.ID, biz.ID)

	if err := svc.RecordBusinessActivityViewed(ctx, staff.ID, biz.ID, "req-6"); err != nil {
		t.Fatalf("RecordBusinessActivityViewed: %v", err)
	}
	entries, err := svc.ListAuditLog(ctx, 50)
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == platformadmin.ActionBusinessActivityViewed && e.StaffID == staff.ID && e.TargetBusinessID == biz.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a business.activity_viewed audit entry")
	}

	members, err := idsvc.ListMembersForBusiness(ctx, uuid.Nil, biz.ID)
	if err != nil {
		t.Fatalf("ListMembersForBusiness: %v", err)
	}
	if len(members) != 1 || members[0].Role != "owner" {
		t.Fatalf("expected exactly the owner, got %+v", members)
	}

	stats, err := svc.Stats(ctx, time.Now())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalBusinesses < 1 || stats.ActiveBusinesses < 1 || stats.NewBusinessesLast7d < 1 || stats.TotalUsers < 1 {
		t.Fatalf("expected the just-created business/user to be counted, got %+v", stats)
	}
}
