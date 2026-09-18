package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/auth"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/store"
	"github.com/mananuf/justbarme/internal/store/sqlc"
	"github.com/mananuf/justbarme/internal/tenancy"
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

// CreateUser hashes password and inserts a new global identity. It returns
// ErrEmailTaken if the email (case-insensitive) is already registered.
func (s *Service) CreateUser(ctx context.Context, email, phone, displayName, password string) (User, error) {
	hash, err := auth.HashPassword(password, auth.Argon2Params{
		MemoryKiB:   s.argon2.MemoryKiB,
		Iterations:  s.argon2.Iterations,
		Parallelism: s.argon2.Parallelism,
	})
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}
	return s.CreateUserWithHashedPassword(ctx, email, phone, displayName, hash)
}

// CreateUserWithHashedPassword inserts a new global identity from a password
// already hashed by the caller, for a flow that must not hold a plaintext
// password in memory across a delay (internal/signup hashes it at
// signup-start time, before the person has even confirmed their email, and
// passes the hash through here once the OTP verifies). It is the only other
// writer of the users table, kept here rather than in internal/signup so
// that package never touches sqlc/store directly for a table it does not
// own. Returns ErrEmailTaken if the email (case-insensitive) is already
// registered.
func (s *Service) CreateUserWithHashedPassword(ctx context.Context, email, phone, displayName, passwordHash string) (User, error) {
	id, err := newID()
	if err != nil {
		return User{}, err
	}

	var created sqlc.User
	err = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.CreateUser(ctx, sqlc.CreateUserParams{
			ID:           id,
			Email:        pgText(email),
			Phone:        pgText(phone),
			DisplayName:  displayName,
			PasswordHash: pgText(passwordHash),
		})
		if err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		if pgErrorCode(err) == pgUniqueViolation {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return toUser(created), nil
}

// CreateUserWithoutPassword inserts a new global identity with no password
// at all -- for an account created via an external identity provider
// (internal/oauth) that already proved control of the email some other
// way. password_hash is nullable specifically for this case;
// auth.VerifyPassword already fails safely (ErrInvalidHashFormat) against
// an empty/NULL hash, so Authenticate needs no special case for these
// accounts -- a password-login attempt on one just looks like a wrong
// password. Returns ErrEmailTaken if the email is already registered.
func (s *Service) CreateUserWithoutPassword(ctx context.Context, email, phone, displayName string) (User, error) {
	return s.CreateUserWithHashedPassword(ctx, email, phone, displayName, "")
}

// GetUserByEmail looks up a user by email (case-insensitive), for a caller
// that already knows (e.g. via EmailIsRegistered) that a real account
// should exist -- internal/oauth uses this to auto-link a Google sign-in to
// an existing password-based account. Returns ErrUserNotFound if none does.
func (s *Service) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var found sqlc.User
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetUserByEmail(ctx, email)
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("look up user by email: %w", err)
	}
	return toUser(found), nil
}

// EmailIsRegistered reports whether email (case-insensitive) already
// belongs to a real, completed account -- the minimal check
// internal/signup needs before starting a new signup, without exposing a
// full User lookup across the package boundary.
func (s *Service) EmailIsRegistered(ctx context.Context, email string) (bool, error) {
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		_, err := q.GetUserByEmail(ctx, email)
		return err
	})
	if err == nil {
		return true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("look up user by email: %w", err)
}

// PhoneIsRegistered reports whether phone already belongs to a real,
// completed account -- the phone-channel counterpart of EmailIsRegistered.
func (s *Service) PhoneIsRegistered(ctx context.Context, phone string) (bool, error) {
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		_, err := q.GetUserByPhone(ctx, pgText(phone))
		return err
	})
	if err == nil {
		return true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("look up user by phone: %w", err)
}

// GetUserByPhone looks up a user by phone, for a caller that already knows
// (e.g. via PhoneIsRegistered, or an invitation match) that a real account
// should exist. Returns ErrUserNotFound if none does.
func (s *Service) GetUserByPhone(ctx context.Context, phone string) (User, error) {
	var found sqlc.User
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetUserByPhone(ctx, pgText(phone))
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("look up user by phone: %w", err)
	}
	return toUser(found), nil
}

// looksLikeEmail is the same identifier-format detection docs/PHASE_
// INVITATIONS_WHATSAPP.md settled on for a login identifier that could be
// either an email or a phone number -- an email always contains '@',
// which E.164 phone numbers never do.
func looksLikeEmail(identifier string) bool {
	return strings.Contains(identifier, "@")
}

// Authenticate verifies identifier (an email or a phone number, detected
// by format) and password, returning ErrInvalidCredentials for an unknown
// identifier, a wrong password, or a suspended account — deliberately the
// same error in every case, so a caller can never learn which one occurred
// (docs/API_CONTRACT.md §8: "one generic invalid-credentials response").
func (s *Service) Authenticate(ctx context.Context, identifier, password string) (User, error) {
	var found sqlc.User
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		if looksLikeEmail(identifier) {
			found, err = q.GetUserByEmail(ctx, identifier)
		} else {
			found, err = q.GetUserByPhone(ctx, pgText(identifier))
		}
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, fmt.Errorf("look up user: %w", err)
	}

	ok, verifyErr := auth.VerifyPassword(toText(found.PasswordHash), password)
	if verifyErr != nil || !ok || found.Status != "active" {
		return User{}, ErrInvalidCredentials
	}
	return toUser(found), nil
}

// AddPhoneToUser attaches phone to an already-existing user -- called only
// after internal/verification confirms the user actually controls it
// (docs/PHASE_INVITATIONS_WHATSAPP.md's verify-before-link rule: never
// trust a match alone). Returns ErrPhoneTaken if another account already
// has it.
func (s *Service) AddPhoneToUser(ctx context.Context, userID uuid.UUID, phone string) (User, error) {
	var updated sqlc.User
	err := store.WithApp(ctx, s.pool, userID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.SetUserPhone(ctx, sqlc.SetUserPhoneParams{ID: userID, Phone: pgText(phone)})
		if err != nil {
			return err
		}
		updated = row
		return nil
	})
	if err != nil {
		if pgErrorCode(err) == pgUniqueViolation {
			return User{}, ErrPhoneTaken
		}
		return User{}, fmt.Errorf("add phone to user: %w", err)
	}
	return toUser(updated), nil
}

// AddEmailToUser attaches email to an already-existing user -- the email
// counterpart of AddPhoneToUser, same verify-before-link precondition.
// Returns ErrEmailTaken if another account already has it.
func (s *Service) AddEmailToUser(ctx context.Context, userID uuid.UUID, email string) (User, error) {
	var updated sqlc.User
	err := store.WithApp(ctx, s.pool, userID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.SetUserEmail(ctx, sqlc.SetUserEmailParams{ID: userID, Email: pgText(email)})
		if err != nil {
			return err
		}
		updated = row
		return nil
	})
	if err != nil {
		if pgErrorCode(err) == pgUniqueViolation {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("add email to user: %w", err)
	}
	return toUser(updated), nil
}

func (s *Service) GetUserByID(ctx context.Context, userID uuid.UUID) (User, error) {
	var found sqlc.User
	err := store.WithApp(ctx, s.pool, userID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetUserByID(ctx, userID)
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("look up user: %w", err)
	}
	return toUser(found), nil
}

// CreateBusinessWithOwner creates a business, an owner membership for
// ownerUserID, and a default location, all in one transaction — see
// docs/API_CONTRACT.md §8 "Business creation". The authenticated creator
// always becomes Owner; there is no other way to create a business.
func (s *Service) CreateBusinessWithOwner(ctx context.Context, ownerUserID uuid.UUID, name string) (Business, Membership, Location, error) {
	businessID, err := newID()
	if err != nil {
		return Business{}, Membership{}, Location{}, err
	}
	locationID, err := newID()
	if err != nil {
		return Business{}, Membership{}, Location{}, err
	}

	var (
		business   sqlc.Business
		membership sqlc.BusinessMembership
		location   sqlc.Location
	)
	err = store.WithTenant(ctx, s.pool, ownerUserID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		business, err = q.CreateBusiness(ctx, sqlc.CreateBusinessParams{ID: businessID, Name: name})
		if err != nil {
			return fmt.Errorf("create business: %w", err)
		}
		membership, err = q.CreateMembership(ctx, sqlc.CreateMembershipParams{
			BusinessID: businessID,
			UserID:     ownerUserID,
			Role:       tenancy.RoleOwner,
		})
		if err != nil {
			return fmt.Errorf("create owner membership: %w", err)
		}
		location, err = q.CreateDefaultLocation(ctx, sqlc.CreateDefaultLocationParams{ID: locationID, BusinessID: businessID})
		if err != nil {
			return fmt.Errorf("create default location: %w", err)
		}
		return nil
	})
	if err != nil {
		return Business{}, Membership{}, Location{}, err
	}
	return toBusiness(business), toMembership(membership, business.Name), toLocation(location), nil
}

// AddMembership adds userID to an already-existing business with role --
// unlike CreateBusinessWithOwner, this never creates the business itself.
// For internal/invitations: an accepted invitation attaches its role to
// whichever user completed signup/login, without creating a second
// business. Returns ErrAlreadyMember if userID already has a membership
// (active or not) for businessID.
func (s *Service) AddMembership(ctx context.Context, businessID, userID uuid.UUID, role string) (Membership, error) {
	var (
		business   sqlc.Business
		membership sqlc.BusinessMembership
	)
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		business, err = q.GetBusinessByID(ctx, businessID)
		if err != nil {
			return fmt.Errorf("look up business: %w", err)
		}
		membership, err = q.CreateMembership(ctx, sqlc.CreateMembershipParams{
			BusinessID: businessID, UserID: userID, Role: role,
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Membership{}, ErrBusinessNotFound
		}
		if pgErrorCode(err) == pgUniqueViolation {
			return Membership{}, ErrAlreadyMember
		}
		return Membership{}, fmt.Errorf("add membership: %w", err)
	}
	return toMembership(membership, business.Name), nil
}

// GetMembership looks up userID's membership in businessID. It returns
// ErrMembershipNotFound if none exists and ErrMembershipNotActive if the
// membership has been revoked — callers resolving tenant context must treat
// both as "this request has no valid business context" and fail closed.
func (s *Service) GetMembership(ctx context.Context, userID, businessID uuid.UUID) (Membership, error) {
	var found sqlc.BusinessMembership
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetMembership(ctx, sqlc.GetMembershipParams{BusinessID: businessID, UserID: userID})
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Membership{}, ErrMembershipNotFound
		}
		return Membership{}, fmt.Errorf("look up membership: %w", err)
	}
	if found.Status != "active" {
		return Membership{}, ErrMembershipNotActive
	}
	return toMembership(found, ""), nil
}

// ListMembershipsForUser lists every business userID belongs to, active or
// not, for GET /api/v1/me. It relies on the business_memberships_self_access
// RLS policy rather than a resolved tenant context, since a user may belong
// to more than one business.
func (s *Service) ListMembershipsForUser(ctx context.Context, userID uuid.UUID) ([]Membership, error) {
	var rows []sqlc.ListMembershipsForUserRow
	err := store.WithApp(ctx, s.pool, userID, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		rows, err = q.ListMembershipsForUser(ctx, userID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list memberships: %w", err)
	}

	out := make([]Membership, 0, len(rows))
	for _, r := range rows {
		out = append(out, Membership{
			BusinessID:   r.BusinessID,
			BusinessName: r.BusinessName,
			UserID:       r.UserID,
			Role:         r.Role,
			Status:       r.Status,
			JoinedAt:     toTime(r.JoinedAt),
		})
	}
	return out, nil
}

// GetBusiness reads businessID. The businesses table carries no RLS of its
// own (it is the tenant root, not a tenant-owned row — see
// docs/ARCHITECTURE.md §7.3), so callers MUST have already verified an
// active membership before calling this; it is not a substitute for that
// check.
func (s *Service) GetBusiness(ctx context.Context, userID, businessID uuid.UUID) (Business, error) {
	var found sqlc.Business
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetBusinessByID(ctx, businessID)
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Business{}, ErrBusinessNotFound
		}
		return Business{}, fmt.Errorf("look up business: %w", err)
	}
	return toBusiness(found), nil
}

// CreateSession issues a new opaque session and CSRF token pair for userID.
// The raw tokens are returned exactly once, here, at issuance — only their
// hashes are ever persisted or returned again.
func (s *Service) CreateSession(ctx context.Context, userID uuid.UUID, userAgent string, ttl time.Duration) (rawSessionToken, rawCSRFToken string, sess Session, err error) {
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

	var created sqlc.Session
	err = store.WithApp(ctx, s.pool, userID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.CreateSession(ctx, sqlc.CreateSessionParams{
			ID:            id,
			UserID:        userID,
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
// touching last_used_at as a side effect. It returns ErrSessionNotFound for
// a token that is unknown, revoked, or expired — those are indistinguishable
// to a caller, which is the point: an attacker probing tokens learns nothing
// about which failure mode occurred.
func (s *Service) GetActiveSessionByToken(ctx context.Context, rawToken string) (Session, error) {
	tokenHash := auth.HashToken(rawToken)

	var found sqlc.Session
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetActiveSessionByTokenHash(ctx, tokenHash)
		if err != nil {
			return err
		}
		found = row
		return q.TouchSession(ctx, row.ID)
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
// session without disturbing the session itself, so a reloaded frontend can
// resume with a valid token via GET /api/v1/me (docs/API_CONTRACT.md §8).
// The previous CSRF token stops working immediately.
func (s *Service) RotateCSRFToken(ctx context.Context, sessionID uuid.UUID) (string, error) {
	rawCSRF, err := auth.GenerateToken()
	if err != nil {
		return "", fmt.Errorf("generate csrf token: %w", err)
	}
	err = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		return q.UpdateSessionCSRFTokenHash(ctx, sqlc.UpdateSessionCSRFTokenHashParams{
			ID:            sessionID,
			CsrfTokenHash: auth.HashToken(rawCSRF),
		})
	})
	if err != nil {
		return "", fmt.Errorf("rotate csrf token: %w", err)
	}
	return rawCSRF, nil
}

// RevokeSession revokes sessionID if and only if it belongs to userID —
// sessions carry no RLS of their own (they are not tenant-owned), so this
// ownership check is the only thing standing between a caller and revoking
// someone else's session.
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID uuid.UUID, reason string) error {
	var affected int64
	err := store.WithApp(ctx, s.pool, userID, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		affected, err = q.RevokeSession(ctx, sqlc.RevokeSessionParams{
			ID:            sessionID,
			RevokedReason: pgText(reason),
			UserID:        userID,
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

// CreateDevice enrolls a new device for userID within businessID/locationID.
// It returns ErrDevicePublicKeyTaken if that exact public key is already
// enrolled for this business.
func (s *Service) CreateDevice(ctx context.Context, userID, businessID, locationID uuid.UUID, publicKeyB64, displayName string) (Device, error) {
	id, err := newID()
	if err != nil {
		return Device{}, err
	}

	var created sqlc.Device
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.CreateDevice(ctx, sqlc.CreateDeviceParams{
			ID:          id,
			BusinessID:  businessID,
			UserID:      userID,
			LocationID:  locationID,
			PublicKey:   publicKeyB64,
			DisplayName: displayName,
		})
		if err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		if pgErrorCode(err) == pgUniqueViolation {
			return Device{}, ErrDevicePublicKeyTaken
		}
		return Device{}, fmt.Errorf("enroll device: %w", err)
	}
	return toDevice(created), nil
}

func (s *Service) ListDevices(ctx context.Context, userID, businessID uuid.UUID) ([]Device, error) {
	var rows []sqlc.Device
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		rows, err = q.ListDevicesForBusiness(ctx, businessID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	out := make([]Device, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDevice(r))
	}
	return out, nil
}

// RevokeDevice revokes deviceID within businessID. It returns
// ErrDeviceNotFound both when the device does not exist and when it is
// already revoked, since docs/ARCHITECTURE.md's "revoked device cannot
// sync" invariant means a second revoke attempt has nothing further to do.
func (s *Service) RevokeDevice(ctx context.Context, userID, businessID, deviceID uuid.UUID) (Device, error) {
	var revoked sqlc.Device
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.RevokeDevice(ctx, sqlc.RevokeDeviceParams{BusinessID: businessID, ID: deviceID})
		if err != nil {
			return err
		}
		revoked = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Device{}, ErrDeviceNotFound
		}
		return Device{}, fmt.Errorf("revoke device: %w", err)
	}
	return toDevice(revoked), nil
}

// GetDevice looks up deviceID within businessID, for building an offline
// lease at enrollment time.
func (s *Service) GetDevice(ctx context.Context, userID, businessID, deviceID uuid.UUID) (Device, error) {
	var found sqlc.Device
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetDeviceByID(ctx, sqlc.GetDeviceByIDParams{BusinessID: businessID, ID: deviceID})
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Device{}, ErrDeviceNotFound
		}
		return Device{}, fmt.Errorf("look up device: %w", err)
	}
	return toDevice(found), nil
}

// GetDefaultLocation returns businessID's single default location, created
// atomically alongside the business itself in CreateBusinessWithOwner.
func (s *Service) GetDefaultLocation(ctx context.Context, userID, businessID uuid.UUID) (Location, error) {
	var found sqlc.Location
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetDefaultLocation(ctx, businessID)
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Location{}, fmt.Errorf("default location: %w", ErrBusinessNotFound)
		}
		return Location{}, fmt.Errorf("look up default location: %w", err)
	}
	return toLocation(found), nil
}
