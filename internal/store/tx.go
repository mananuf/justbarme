package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

// appRole is the reduced-privilege role every application transaction runs
// as: NOBYPASSRLS, no schema-ownership rights. See migrations/000001_app_role.
const appRole = "jbm_app"

// WithApp runs fn inside a transaction scoped to userID but not to any
// business. Use it for global operations — resolving a session, looking up
// a user by email — that must not be blocked by a tenant-owned table's RLS
// policy but still want the caller's identity available to any policy that
// checks it (see business_memberships_self_access).
//
// userID may be uuid.Nil for requests with no authenticated principal yet
// (for example, verifying a login attempt): current_setting('app.user_id')
// then simply never matches a real row.
func WithApp(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, fn func(ctx context.Context, q *sqlc.Queries) error) error {
	return withTx(ctx, pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+appRole); err != nil {
			return fmt.Errorf("set application role: %w", err)
		}
		if _, err := tx.Exec(ctx, "SELECT set_config('app.user_id', $1, true)", userID.String()); err != nil {
			return fmt.Errorf("set tenant context: %w", err)
		}
		return fn(ctx, sqlc.New(tx))
	})
}

// WithTenant runs fn inside a transaction scoped to both userID and
// businessID, so every RLS policy on a tenant-owned table applies. Callers
// must have already verified userID holds an active membership in
// businessID — WithTenant enforces isolation at the database layer, it does
// not check membership itself.
func WithTenant(ctx context.Context, pool *pgxpool.Pool, userID, businessID uuid.UUID, fn func(ctx context.Context, q *sqlc.Queries) error) error {
	return withTx(ctx, pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+appRole); err != nil {
			return fmt.Errorf("set application role: %w", err)
		}
		if _, err := tx.Exec(ctx, "SELECT set_config('app.user_id', $1, true)", userID.String()); err != nil {
			return fmt.Errorf("set tenant context: %w", err)
		}
		if _, err := tx.Exec(ctx, "SELECT set_config('app.business_id', $1, true)", businessID.String()); err != nil {
			return fmt.Errorf("set tenant context: %w", err)
		}
		return fn(ctx, sqlc.New(tx))
	})
}

func withTx(ctx context.Context, pool *pgxpool.Pool, fn func(ctx context.Context, tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
