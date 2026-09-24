package platformadmin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/auth"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/store"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

type Service struct {
	pool   *pgxpool.Pool
	argon2 config.Argon2
}

func New(pool *pgxpool.Pool, argon2 config.Argon2) *Service {
	return &Service{pool: pool, argon2: argon2}
}

func newID() (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("generate id: %w", err)
	}
	return id, nil
}

// CreateStaff hashes password and inserts a new platform staff account.
// There is no self-service signup for this tier -- see cmd/seed-platform-staff,
// the only caller. Returns ErrEmailTaken if the email (case-insensitive) is
// already registered.
func (s *Service) CreateStaff(ctx context.Context, email, displayName, password, role string) (Staff, error) {
	id, err := newID()
	if err != nil {
		return Staff{}, err
	}
	hash, err := auth.HashPassword(password, auth.Argon2Params{
		MemoryKiB: s.argon2.MemoryKiB, Iterations: s.argon2.Iterations, Parallelism: s.argon2.Parallelism,
	})
	if err != nil {
		return Staff{}, fmt.Errorf("hash password: %w", err)
	}

	var created sqlc.PlatformStaff
	err = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.CreatePlatformStaff(ctx, sqlc.CreatePlatformStaffParams{
			ID: id, Email: email, DisplayName: displayName, PasswordHash: hash, Role: role,
		})
		if err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		if pgErrorCode(err) == pgUniqueViolation {
			return Staff{}, ErrEmailTaken
		}
		return Staff{}, fmt.Errorf("create platform staff: %w", err)
	}
	return toStaff(created), nil
}

// Authenticate verifies email and password, returning ErrInvalidCredentials
// for either an unknown email, a wrong password, or a revoked account --
// deliberately the same error in every case, matching
// internal/identity.Service.Authenticate's reasoning exactly.
func (s *Service) Authenticate(ctx context.Context, email, password string) (Staff, error) {
	var found sqlc.PlatformStaff
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetPlatformStaffByEmail(ctx, email)
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Staff{}, ErrInvalidCredentials
		}
		return Staff{}, fmt.Errorf("look up platform staff: %w", err)
	}

	ok, verifyErr := auth.VerifyPassword(found.PasswordHash, password)
	if verifyErr != nil || !ok || found.Status != "active" {
		return Staff{}, ErrInvalidCredentials
	}
	return toStaff(found), nil
}

func (s *Service) GetStaffByID(ctx context.Context, staffID uuid.UUID) (Staff, error) {
	var found sqlc.PlatformStaff
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetPlatformStaffByID(ctx, staffID)
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Staff{}, ErrStaffNotFound
		}
		return Staff{}, fmt.Errorf("look up platform staff: %w", err)
	}
	return toStaff(found), nil
}

// CreateSession issues a new online session for staffID -- mirrors
// internal/identity.Service.CreateSession exactly, in a wholly separate
// table so a platform session token can never be presented as (or
// confused with) a business user's session token.
func (s *Service) CreateSession(ctx context.Context, staffID uuid.UUID, userAgent string, ttl time.Duration) (rawSessionToken, rawCSRFToken string, sess Session, err error) {
	id, err := newID()
	if err != nil {
		return "", "", Session{}, err
	}
	rawSessionToken, err = auth.GenerateToken()
	if err != nil {
		return "", "", Session{}, fmt.Errorf("generate session token: %w", err)
	}
	rawCSRFToken, err = auth.GenerateToken()
	if err != nil {
		return "", "", Session{}, fmt.Errorf("generate csrf token: %w", err)
	}

	var created sqlc.PlatformSession
	err = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.CreatePlatformSession(ctx, sqlc.CreatePlatformSessionParams{
			ID:            id,
			StaffID:       staffID,
			TokenHash:     auth.HashToken(rawSessionToken),
			CsrfTokenHash: auth.HashToken(rawCSRFToken),
			UserAgent:     pgText(userAgent),
			ExpiresAt:     pgTimestamptz(time.Now().Add(ttl)),
		})
		if err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		return "", "", Session{}, fmt.Errorf("create session: %w", err)
	}
	return rawSessionToken, rawCSRFToken, toSession(created), nil
}

// GetActiveSessionByToken resolves a raw bearer token to its session,
// touching last_used_at as a side effect. Returns ErrSessionNotFound for a
// token that is unknown, revoked, or expired.
func (s *Service) GetActiveSessionByToken(ctx context.Context, rawToken string) (Session, error) {
	tokenHash := auth.HashToken(rawToken)

	var found sqlc.PlatformSession
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetActivePlatformSessionByTokenHash(ctx, tokenHash)
		if err != nil {
			return err
		}
		found = row
		return q.TouchPlatformSession(ctx, row.ID)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, fmt.Errorf("look up session: %w", err)
	}
	return toSession(found), nil
}

// RotateCSRFToken issues and stores a new CSRF token for an existing
// session without disturbing the session itself, mirroring
// internal/identity.Service.RotateCSRFToken exactly -- this is what lets
// platformMe return a usable token after a page reload, since only the
// hash is ever persisted and login is the only other place a raw token is
// issued. The previous CSRF token stops working immediately.
func (s *Service) RotateCSRFToken(ctx context.Context, sessionID uuid.UUID) (string, error) {
	rawCSRF, err := auth.GenerateToken()
	if err != nil {
		return "", fmt.Errorf("generate csrf token: %w", err)
	}
	err = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		return q.UpdatePlatformSessionCSRFTokenHash(ctx, sqlc.UpdatePlatformSessionCSRFTokenHashParams{
			ID: sessionID, CsrfTokenHash: auth.HashToken(rawCSRF),
		})
	})
	if err != nil {
		return "", fmt.Errorf("rotate csrf token: %w", err)
	}
	return rawCSRF, nil
}

// RevokeSession revokes sessionID if and only if it belongs to staffID.
func (s *Service) RevokeSession(ctx context.Context, staffID, sessionID uuid.UUID, reason string) error {
	var affected int64
	err := store.WithApp(ctx, s.pool, staffID, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		affected, err = q.RevokePlatformSession(ctx, sqlc.RevokePlatformSessionParams{
			ID: sessionID, RevokedReason: pgText(reason), StaffID: staffID,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if affected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// ListBusinesses returns platform-oversight summaries -- name, status,
// timezone, currency, creation date -- for every business on the service.
// Deliberately not audited: it exposes no tenant-owned data (no sales,
// members, or catalogue), the same non-sensitive fields visible in any
// business's own onboarding confirmation. Reading a business's actual
// tenant data requires a still-unbuilt, separately audited impersonation
// flow.
func (s *Service) ListBusinesses(ctx context.Context) ([]Business, error) {
	var rows []sqlc.Business
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		rows, err = q.ListAllBusinesses(ctx)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list businesses: %w", err)
	}
	out := make([]Business, 0, len(rows))
	for _, r := range rows {
		out = append(out, toBusiness(r))
	}
	return out, nil
}

// setBusinessStatus updates businessID's status and records the audit
// entry in the same transaction -- the action and its audit trail either
// both land or neither does, never one without the other.
func (s *Service) setBusinessStatus(ctx context.Context, staffID, businessID uuid.UUID, status, action, reason, requestID string) (Business, error) {
	auditID, err := newID()
	if err != nil {
		return Business{}, err
	}

	var updated sqlc.Business
	err = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.SetBusinessStatus(ctx, sqlc.SetBusinessStatusParams{ID: businessID, Status: status})
		if err != nil {
			return err
		}
		updated = row

		_, err = q.CreatePlatformAuditEntry(ctx, sqlc.CreatePlatformAuditEntryParams{
			ID: auditID, PlatformStaffID: staffID, Action: action,
			TargetBusinessID: pgUUID(businessID), Reason: pgText(reason), RequestID: pgText(requestID),
		})
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Business{}, ErrBusinessNotFound
		}
		return Business{}, fmt.Errorf("%s: %w", action, err)
	}
	return toBusiness(updated), nil
}

// SuspendBusiness flips businessID to suspended and records why, atomically.
func (s *Service) SuspendBusiness(ctx context.Context, staffID, businessID uuid.UUID, reason, requestID string) (Business, error) {
	return s.setBusinessStatus(ctx, staffID, businessID, "suspended", ActionBusinessSuspended, reason, requestID)
}

// ReactivateBusiness flips businessID back to active and records why,
// atomically.
func (s *Service) ReactivateBusiness(ctx context.Context, staffID, businessID uuid.UUID, reason, requestID string) (Business, error) {
	return s.setBusinessStatus(ctx, staffID, businessID, "active", ActionBusinessReactivated, reason, requestID)
}

// RecordLogin writes a login audit entry. Best-effort by design: a failure
// here should never block the login it is recording, so callers log and
// continue rather than fail the request.
func (s *Service) RecordLogin(ctx context.Context, staffID uuid.UUID, requestID string) error {
	id, err := newID()
	if err != nil {
		return err
	}
	return store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		_, err := q.CreatePlatformAuditEntry(ctx, sqlc.CreatePlatformAuditEntryParams{
			ID: id, PlatformStaffID: staffID, Action: ActionLogin, RequestID: pgText(requestID),
		})
		return err
	})
}

// ListAuditLog returns the most recent platform audit entries, newest
// first, capped at limit.
func (s *Service) ListAuditLog(ctx context.Context, limit int32) ([]AuditEntry, error) {
	var rows []sqlc.PlatformAuditLog
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		rows, err = q.ListPlatformAuditLog(ctx, limit)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list audit log: %w", err)
	}
	out := make([]AuditEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAuditEntry(r))
	}
	return out, nil
}

// RecordBusinessActivityViewed logs that staffID opened businessID's
// aggregate activity summary. Unlike RecordLogin this is NOT best-effort:
// callers must write it before returning any tenant data, so an
// unrecordable read is a refused read, never an undetectable one.
func (s *Service) RecordBusinessActivityViewed(ctx context.Context, staffID, businessID uuid.UUID, requestID string) error {
	id, err := newID()
	if err != nil {
		return err
	}
	err = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		_, err := q.CreatePlatformAuditEntry(ctx, sqlc.CreatePlatformAuditEntryParams{
			ID: id, PlatformStaffID: staffID, Action: ActionBusinessActivityViewed,
			TargetBusinessID: pgUUID(businessID), RequestID: pgText(requestID),
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("record business activity view: %w", err)
	}
	return nil
}

// GetBusiness returns one business's oversight summary, or
// ErrBusinessNotFound.
func (s *Service) GetBusiness(ctx context.Context, businessID uuid.UUID) (Business, error) {
	var row sqlc.Business
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		row, err = q.GetBusinessByID(ctx, businessID)
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Business{}, ErrBusinessNotFound
		}
		return Business{}, fmt.Errorf("get business: %w", err)
	}
	return toBusiness(row), nil
}

// Stats summarizes the whole platform from non-tenant tables only
// (businesses, users) -- no RLS-scoped data is read.
func (s *Service) Stats(ctx context.Context, now time.Time) (Stats, error) {
	businesses, err := s.ListBusinesses(ctx)
	if err != nil {
		return Stats{}, err
	}
	out := Stats{TotalBusinesses: len(businesses)}
	weekAgo := now.Add(-7 * 24 * time.Hour)
	for _, b := range businesses {
		if b.Status == "suspended" {
			out.SuspendedBusinesses++
		} else {
			out.ActiveBusinesses++
		}
		if b.CreatedAt.After(weekAgo) {
			out.NewBusinessesLast7d++
		}
	}
	err = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		out.TotalUsers, err = q.CountUsers(ctx)
		return err
	})
	if err != nil {
		return Stats{}, fmt.Errorf("count users: %w", err)
	}
	return out, nil
}

// ListStaff returns every platform staff account, including revoked ones
// (status is part of the row).
func (s *Service) ListStaff(ctx context.Context) ([]Staff, error) {
	var rows []sqlc.PlatformStaff
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		rows, err = q.ListPlatformStaff(ctx)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list platform staff: %w", err)
	}
	out := make([]Staff, 0, len(rows))
	for _, r := range rows {
		out = append(out, toStaff(r))
	}
	return out, nil
}

// CreateStaffAudited creates a platform staff account on behalf of
// actingStaffID and records the audit entry in the same transaction.
func (s *Service) CreateStaffAudited(ctx context.Context, actingStaffID uuid.UUID, email, displayName, password, role, requestID string) (Staff, error) {
	id, err := newID()
	if err != nil {
		return Staff{}, err
	}
	auditID, err := newID()
	if err != nil {
		return Staff{}, err
	}
	hash, err := auth.HashPassword(password, auth.Argon2Params{
		MemoryKiB: s.argon2.MemoryKiB, Iterations: s.argon2.Iterations, Parallelism: s.argon2.Parallelism,
	})
	if err != nil {
		return Staff{}, fmt.Errorf("hash password: %w", err)
	}

	var created sqlc.PlatformStaff
	err = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.CreatePlatformStaff(ctx, sqlc.CreatePlatformStaffParams{
			ID: id, Email: email, DisplayName: displayName, PasswordHash: hash, Role: role,
		})
		if err != nil {
			return err
		}
		created = row
		_, err = q.CreatePlatformAuditEntry(ctx, sqlc.CreatePlatformAuditEntryParams{
			ID: auditID, PlatformStaffID: actingStaffID, Action: ActionStaffCreated,
			TargetStaffID: pgUUID(id), RequestID: pgText(requestID),
		})
		return err
	})
	if err != nil {
		if pgErrorCode(err) == pgUniqueViolation {
			return Staff{}, ErrEmailTaken
		}
		return Staff{}, fmt.Errorf("create platform staff: %w", err)
	}
	return toStaff(created), nil
}

// RevokeStaff disables targetID's account and ends every session it has,
// atomically with the audit entry -- revocation takes effect immediately,
// not merely at the next login. A staff member cannot revoke themselves, so
// at least one superadmin (the actor) always remains.
func (s *Service) RevokeStaff(ctx context.Context, actingStaffID, targetID uuid.UUID, reason, requestID string) (Staff, error) {
	if actingStaffID == targetID {
		return Staff{}, ErrCannotRevokeSelf
	}
	auditID, err := newID()
	if err != nil {
		return Staff{}, err
	}
	var updated sqlc.PlatformStaff
	err = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.SetPlatformStaffStatus(ctx, sqlc.SetPlatformStaffStatusParams{ID: targetID, Status: "revoked"})
		if err != nil {
			return err
		}
		updated = row
		if err := q.RevokeAllPlatformSessionsForStaff(ctx, sqlc.RevokeAllPlatformSessionsForStaffParams{
			StaffID: targetID, RevokedReason: pgText("account revoked"),
		}); err != nil {
			return err
		}
		_, err = q.CreatePlatformAuditEntry(ctx, sqlc.CreatePlatformAuditEntryParams{
			ID: auditID, PlatformStaffID: actingStaffID, Action: ActionStaffRevoked,
			TargetStaffID: pgUUID(targetID), Reason: pgText(reason), RequestID: pgText(requestID),
		})
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Staff{}, ErrStaffNotFound
		}
		return Staff{}, fmt.Errorf("revoke platform staff: %w", err)
	}
	return toStaff(updated), nil
}
