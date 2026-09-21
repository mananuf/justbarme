package sales

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mananuf/justbarme/internal/auth"
	"github.com/mananuf/justbarme/internal/store"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

func optionalTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	out := t.Time
	return &out
}

func pgOptionalTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func toBillShareLink(l sqlc.BillShareLink) BillShareLink {
	return BillShareLink{
		ID: l.ID, BillID: l.BillID, CreatedBy: l.CreatedBy, CreatedAt: l.CreatedAt.Time,
		ExpiresAt: optionalTime(l.ExpiresAt), RevokedAt: optionalTime(l.RevokedAt),
	}
}

// CreateOrRotateShareLink creates a public bill-share link, or replaces the
// existing one if the bill already has one (bill_id is UNIQUE on
// bill_share_links -- "creates or rotates" per docs/PHASE_PILOT_RELEASE.md
// §4) -- a bill only ever has at most one active link, never several
// accumulating from repeated re-shares. The raw token (and its full URL)
// is returned exactly once, here, at creation -- only the token's hash is
// ever persisted, same one-time-reveal convention GET /me's CSRF rotation
// already uses. expiresAt is nil for a link that never expires until
// explicitly revoked or rotated.
func (s *Service) CreateOrRotateShareLink(ctx context.Context, userID, businessID, billID uuid.UUID, expiresAt *time.Time) (rawToken, url string, link BillShareLink, err error) {
	rawToken, err = auth.GenerateToken()
	if err != nil {
		return "", "", BillShareLink{}, fmt.Errorf("generate share link token: %w", err)
	}
	linkID, err := newID()
	if err != nil {
		return "", "", BillShareLink{}, err
	}

	var created sqlc.BillShareLink
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		if _, err := q.GetBillByID(ctx, sqlc.GetBillByIDParams{BusinessID: businessID, ID: billID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrBillNotFound
			}
			return fmt.Errorf("get bill: %w", err)
		}
		row, err := q.UpsertBillShareLink(ctx, sqlc.UpsertBillShareLinkParams{
			ID: linkID, BusinessID: businessID, BillID: billID,
			TokenHash: auth.HashToken(rawToken), CreatedBy: userID, ExpiresAt: pgOptionalTimestamptz(expiresAt),
		})
		if err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrBillNotFound) {
			return "", "", BillShareLink{}, err
		}
		return "", "", BillShareLink{}, fmt.Errorf("create bill share link: %w", err)
	}
	return rawToken, s.publicBaseURL + "/bill/" + rawToken, toBillShareLink(created), nil
}

// RevokeShareLink is idempotent: revoking a bill with no active link (never
// shared, already revoked, or already rotated away) is a no-op rather than
// an error, the same "deleting a key that doesn't exist is not an error"
// convention storage.Provider.Delete documents.
func (s *Service) RevokeShareLink(ctx context.Context, userID, businessID, billID uuid.UUID) error {
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		_, err := q.RevokeBillShareLinkByBillID(ctx, sqlc.RevokeBillShareLinkByBillIDParams{BusinessID: businessID, BillID: billID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("revoke bill share link: %w", err)
	}
	return nil
}

// ResolveShareLink is the public, unauthenticated lookup behind
// GET /public/bills/{token}: it hashes the raw token and looks it up with
// no tenant context at all (bill_share_links carries no RLS, same
// reasoning as invitations -- see that table's own migration comment),
// returning the business/bill IDs a caller then uses to fetch the actual
// bill detail via the ordinary GetBillDetail (bills' RLS policy only ever
// checks business_id, so a uuid.Nil userID there is safe -- see
// docs/PHASE_PILOT_RELEASE.md §4).
func (s *Service) ResolveShareLink(ctx context.Context, rawToken string) (businessID, billID uuid.UUID, err error) {
	var found sqlc.BillShareLink
	err = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetActiveBillShareLinkByTokenHash(ctx, auth.HashToken(rawToken))
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.UUID{}, uuid.UUID{}, ErrShareLinkNotFound
		}
		return uuid.UUID{}, uuid.UUID{}, fmt.Errorf("look up bill share link: %w", err)
	}
	return found.BusinessID, found.BillID, nil
}
