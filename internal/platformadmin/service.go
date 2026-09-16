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
