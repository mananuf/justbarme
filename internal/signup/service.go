// Package signup implements public account creation with email
// verification: anyone can start a signup, but the real users row is only
// created once they prove control of the email via a one-time code. This
// is a deliberate deviation from docs/ARCHITECTURE.md §11.1's "for the
// pilot, support invited accounts" -- see CLAUDE.md's Signup section for
// why. It depends on internal/identity (to create the user and session)
// and internal/email (to deliver the code); it does not touch identity's
// or any other package's storage directly.
package signup

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
	"github.com/mananuf/justbarme/internal/email"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/store"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

// maxOTPAttempts bounds how many wrong codes a pending signup accepts
// before it is treated as invalid outright, even if not yet time-expired --
// a 6-digit code is only as safe as this limit makes brute-forcing it.
const maxOTPAttempts = 5

// otpEmailTemplate is parsed once at package init (a malformed template is
// a programmer error, not something a live signup attempt should ever be
// able to trigger) via internal/email's generic Template type -- the same
// mechanism any future transactional email (a receipt, a password-reset
// notice) would use, not something signup-specific.
var otpEmailTemplate = email.MustNewTemplate("signup_otp",
	"Your justbarme verification code",
	"Your code is {{.Code}}. It expires in {{.ExpiresIn}}.",
)

type otpEmailData struct {
	Code      string
	ExpiresIn string
}

type Service struct {
	pool     *pgxpool.Pool
	argon2   config.Argon2
	identity *identity.Service
	email    email.Provider
	otpTTL   time.Duration
}

func New(pool *pgxpool.Pool, argon2 config.Argon2, identitySvc *identity.Service, emailProvider email.Provider, otpTTL time.Duration) *Service {
	return &Service{pool: pool, argon2: argon2, identity: identitySvc, email: emailProvider, otpTTL: otpTTL}
}

func newID() (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("generate id: %w", err)
	}
	return id, nil
}

// Start hashes password immediately (never held in plaintext across the
// wait for a code, even briefly), generates and emails a 6-digit OTP, and
// upserts one pending signup row for email -- calling Start again for the
// same address before it is verified replaces the code, which is the
// entire "resend" mechanism; there is no separate resend endpoint. Returns
// ErrEmailAlreadyRegistered if email already belongs to a real account,
// without sending anything.
func (s *Service) Start(ctx context.Context, emailAddr, displayName, password string) error {
	registered, err := s.identity.EmailIsRegistered(ctx, emailAddr)
	if err != nil {
		return fmt.Errorf("check existing registration: %w", err)
	}
	if registered {
		return ErrEmailAlreadyRegistered
	}

	passwordHash, err := auth.HashPassword(password, auth.Argon2Params{
		MemoryKiB: s.argon2.MemoryKiB, Iterations: s.argon2.Iterations, Parallelism: s.argon2.Parallelism,
	})
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	code, err := auth.GenerateOTP()
	if err != nil {
		return err
	}

	id, err := newID()
	if err != nil {
		return err
	}

	err = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		_, err := q.UpsertSignupVerification(ctx, sqlc.UpsertSignupVerificationParams{
			ID: id, Email: emailAddr, DisplayName: displayName, PasswordHash: passwordHash,
			OtpHash: auth.HashToken(code), ExpiresAt: pgTimestamptz(time.Now().Add(s.otpTTL)),
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("store pending signup: %w", err)
	}

	msg, err := otpEmailTemplate.Render(emailAddr, otpEmailData{
		Code:      code,
		ExpiresIn: s.otpTTL.Round(time.Minute).String(),
	})
	if err != nil {
		return fmt.Errorf("render verification email: %w", err)
	}
	if err := s.email.Send(ctx, msg); err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}
	return nil
}

// Verify checks code against the pending signup for email. On success it
// creates the real user (from the password hashed at Start, never
// re-touching plaintext) and a normal session -- identical to what
// POST /auth/login produces -- and deletes the pending row. Returns
// ErrNoPendingSignup if Start was never called (or the code already
// expired and was cleaned up), ErrInvalidOrExpiredCode for a wrong code, an
// expired-but-still-present row, or a row that has exhausted
// maxOTPAttempts.
func (s *Service) Verify(ctx context.Context, emailAddr, code, userAgent string, sessionTTL time.Duration) (identity.User, string, string, identity.Session, error) {
	var pending sqlc.SignupVerification
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetSignupVerificationByEmail(ctx, emailAddr)
		if err != nil {
			return err
		}
		pending = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.User{}, "", "", identity.Session{}, ErrNoPendingSignup
		}
		return identity.User{}, "", "", identity.Session{}, fmt.Errorf("look up pending signup: %w", err)
	}

	if pending.Attempts >= maxOTPAttempts || time.Now().After(pending.ExpiresAt.Time) {
		return identity.User{}, "", "", identity.Session{}, ErrInvalidOrExpiredCode
	}
	if auth.HashToken(code) != pending.OtpHash {
		_ = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
			return q.IncrementSignupVerificationAttempts(ctx, pending.ID)
		})
		return identity.User{}, "", "", identity.Session{}, ErrInvalidOrExpiredCode
	}

	user, err := s.identity.CreateUserWithHashedPassword(ctx, pending.Email, "", pending.DisplayName, pending.PasswordHash)
	if err != nil {
		if errors.Is(err, identity.ErrEmailTaken) {
			return identity.User{}, "", "", identity.Session{}, ErrEmailAlreadyRegistered
		}
		return identity.User{}, "", "", identity.Session{}, fmt.Errorf("create user: %w", err)
	}

	rawToken, rawCSRF, sess, err := s.identity.CreateSession(ctx, user.ID, userAgent, sessionTTL)
	if err != nil {
		return identity.User{}, "", "", identity.Session{}, fmt.Errorf("create session: %w", err)
	}

	// Best-effort: the account is already created and usable even if this
	// cleanup fails (the row would just sit there until it naturally
	// expires; it can never be re-verified into a second account since
	// CreateUserWithHashedPassword above would now hit ErrEmailTaken).
	_ = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		return q.DeleteSignupVerification(ctx, pending.ID)
	})

	return user, rawToken, rawCSRF, sess, nil
}
