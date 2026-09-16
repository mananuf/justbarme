package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/store"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

// These tests exercise the RLS policies directly against a real PostgreSQL
// database (docs/ARCHITECTURE.md §7.3 requires this — RLS, constraints, and
// locking cannot be exercised against a substitute). They run only when
// JBM_DATABASE_URL points at a reachable database with migrations applied.
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

// tenant is a fully set up business (owner, business, default location, one
// enrolled device) used as one side of a two-business isolation test.
type tenant struct {
	ownerID    uuid.UUID
	businessID uuid.UUID
	locationID uuid.UUID
	deviceID   uuid.UUID
}

func createTenant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, label string) tenant {
	t.Helper()

	ownerID := uuid.New()
	businessID := uuid.New()
	locationID := uuid.New()
	deviceID := uuid.New()

	// Every unique value is derived from freshly generated UUIDs, not the
	// label alone, so repeated test runs never collide on email/public-key
	// uniqueness constraints left over from a previous run.
	err := store.WithTenant(ctx, pool, ownerID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		if _, err := q.CreateUser(ctx, sqlc.CreateUserParams{
			ID: ownerID, Email: ownerID.String() + "@example.com", DisplayName: label + " Owner", PasswordHash: pgtype.Text{String: "x", Valid: true},
		}); err != nil {
			return err
		}
		if _, err := q.CreateBusiness(ctx, sqlc.CreateBusinessParams{ID: businessID, Name: label}); err != nil {
			return err
		}
		if _, err := q.CreateMembership(ctx, sqlc.CreateMembershipParams{BusinessID: businessID, UserID: ownerID, Role: "owner"}); err != nil {
			return err
		}
		if _, err := q.CreateDefaultLocation(ctx, sqlc.CreateDefaultLocationParams{ID: locationID, BusinessID: businessID}); err != nil {
			return err
		}
		if _, err := q.CreateDevice(ctx, sqlc.CreateDeviceParams{
			ID: deviceID, BusinessID: businessID, UserID: ownerID, LocationID: locationID,
			PublicKey: deviceID.String(), DisplayName: label + " Phone",
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("set up tenant %q: %v", label, err)
	}

	t.Cleanup(func() {
		// A superuser-level connection (no SET ROLE) so cleanup succeeds
		// even for rows a cross-tenant test deliberately left unreachable
		// to jbm_app under one specific tenant scope.
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			t.Logf("cleanup: acquire connection: %v", err)
			return
		}
		defer conn.Release()
		for _, stmt := range []string{
			"DELETE FROM devices WHERE business_id = $1",
			"DELETE FROM business_memberships WHERE business_id = $1",
			"DELETE FROM locations WHERE business_id = $1",
			"DELETE FROM businesses WHERE id = $1",
		} {
			if _, err := conn.Exec(context.Background(), stmt, businessID); err != nil {
				t.Logf("cleanup %q: %v", stmt, err)
			}
		}
		if _, err := conn.Exec(context.Background(), "DELETE FROM users WHERE id = $1", ownerID); err != nil {
			t.Logf("cleanup users: %v", err)
		}
	})

	// CreateUser above ran inside a WithTenant transaction (for atomicity
	// with everything else), which is fine: users carries no RLS, so the
	// tenant-scoped role/GUCs it also applied are simply unused for that
	// one statement.
	return tenant{ownerID: ownerID, businessID: businessID, locationID: locationID, deviceID: deviceID}
}

func TestTenantIsolation_ReadsAreBlockedAcrossBusinesses(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	a := createTenant(t, ctx, pool, "Isolation-Read-A")
	b := createTenant(t, ctx, pool, "Isolation-Read-B")

	// Scoped to business A, ask directly (and correctly, by ID) for
	// business B's device. RLS — not the query's WHERE clause — must be
	// what blocks this.
	err := store.WithTenant(ctx, pool, a.ownerID, a.businessID, func(ctx context.Context, q *sqlc.Queries) error {
		_, err := q.GetDeviceByID(ctx, sqlc.GetDeviceByIDParams{BusinessID: b.businessID, ID: b.deviceID})
		return err
	})
	if err == nil {
		t.Fatal("expected business A's session to be unable to read business B's device")
	}
	if err != pgx.ErrNoRows {
		t.Fatalf("expected pgx.ErrNoRows (row invisible under RLS), got: %v", err)
	}
}

func TestTenantIsolation_WritesAreBlockedAcrossBusinesses(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	a := createTenant(t, ctx, pool, "Isolation-Write-A")
	b := createTenant(t, ctx, pool, "Isolation-Write-B")

	// Scoped to business A, attempt to revoke business B's device by its
	// real ID. The row must remain untouched.
	err := store.WithTenant(ctx, pool, a.ownerID, a.businessID, func(ctx context.Context, q *sqlc.Queries) error {
		_, err := q.RevokeDevice(ctx, sqlc.RevokeDeviceParams{BusinessID: b.businessID, ID: b.deviceID})
		return err
	})
	if err == nil {
		t.Fatal("expected business A's session to be unable to revoke business B's device")
	}

	// Confirm B's device is untouched, scoped correctly this time.
	err = store.WithTenant(ctx, pool, b.ownerID, b.businessID, func(ctx context.Context, q *sqlc.Queries) error {
		device, err := q.GetDeviceByID(ctx, sqlc.GetDeviceByIDParams{BusinessID: b.businessID, ID: b.deviceID})
		if err != nil {
			return err
		}
		if device.Status != "active" {
			t.Fatalf("expected business B's device to remain active, got status %q", device.Status)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify device B untouched: %v", err)
	}
}

func TestTenantIsolation_CompositeForeignKeyRejectsCrossTenantReference(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	a := createTenant(t, ctx, pool, "Isolation-FK-A")
	b := createTenant(t, ctx, pool, "Isolation-FK-B")

	// A device row claiming business_id = A but location_id = B's location.
	// No (business_id, id) row satisfies that pair in locations, so the
	// composite foreign key must reject it outright.
	err := store.WithTenant(ctx, pool, a.ownerID, a.businessID, func(ctx context.Context, q *sqlc.Queries) error {
		_, err := q.CreateDevice(ctx, sqlc.CreateDeviceParams{
			ID: uuid.New(), BusinessID: a.businessID, UserID: a.ownerID, LocationID: b.locationID,
			PublicKey: "cross-tenant-key", DisplayName: "Should never exist",
		})
		return err
	})
	if err == nil {
		t.Fatal("expected the composite foreign key to reject a cross-tenant location reference")
	}
}

func TestTenantIsolation_MissingBusinessContextSeesNothing(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	a := createTenant(t, ctx, pool, "Isolation-NoContext-A")

	// A transaction that never sets app.business_id at all (uuid.Nil) must
	// fail closed, not fall back to seeing every business.
	err := store.WithTenant(ctx, pool, a.ownerID, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		_, err := q.GetDeviceByID(ctx, sqlc.GetDeviceByIDParams{BusinessID: a.businessID, ID: a.deviceID})
		return err
	})
	if err != pgx.ErrNoRows {
		t.Fatalf("expected a missing/nil business context to see no rows, got: %v", err)
	}
}
