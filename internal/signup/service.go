// Package signup implements public account creation with a one-time code,
// over either email or WhatsApp: anyone can start a signup, but the real
// users row is only created once they prove control of the
// email/phone. This is a deliberate deviation from docs/ARCHITECTURE.md
// §11.1's "for the pilot, support invited accounts" -- see CLAUDE.md's
// Signup section for why. WhatsApp support (docs/PHASE_INVITATIONS_
// WHATSAPP.md) generalizes the original email-only flow rather than
// bolting a second one alongside it -- one Start/Verify code path for
// either channel. It depends on internal/identity (to create the user and
// session), internal/email, and internal/whatsapp (to deliver the code);
// it does not touch identity's or any other package's storage directly.
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
	"github.com/mananuf/justbarme/internal/whatsapp"
)

// Channel picks which OTP delivery path a signup attempt uses -- see
// docs/PHASE_INVITATIONS_WHATSAPP.md §1: signup defaults to email
// (cost-sensitive, higher volume) but WhatsApp is always available as an
// explicit choice.
type Channel string

const (
	ChannelEmail    Channel = "email"
	ChannelWhatsApp Channel = "whatsapp"
)

// maxOTPAttempts bounds how many wrong codes a pending signup accepts
// before it is treated as invalid outright, even if not yet time-expired --
// a 6-digit code is only as safe as this limit makes brute-forcing it.
const maxOTPAttempts = 5

// otpEmailTemplate is parsed once at package init (a malformed template is
// a programmer error, not something a live signup attempt should ever be
// able to trigger) via internal/email's generic Template type. The HTML
// body is built with email.Layout so it shares the same header/footer
// chrome as every other transactional email in the app.
var otpEmailTemplate = email.MustNewHTMLTemplate("signup_otp",
	"Your justbarme verification code",
	"Your code is {{.Code}}. It expires in {{.ExpiresIn}}.",
	email.MustLayout(email.LayoutData{
		Preheader: "Your verification code: {{.Code}}",
		Content: `<h1 style="margin:0 0 12px;color:` + email.ColorInk + `;font-size:22px;line-height:1.3;font-weight:700;">Your verification code</h1>
<p style="margin:0 0 28px;color:rgba(22,32,26,0.55);font-size:14px;line-height:1.6;">Enter this code to finish setting up your justbarme account. It expires in {{.ExpiresIn}}.</p>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:0 0 28px;">
<tr><td align="center" style="background-color:` + email.ColorInk + `;border-radius:14px;padding:20px 24px;">
<span style="color:` + email.ColorCream + `;font-size:32px;font-weight:700;letter-spacing:0.3em;font-family:Menlo,Consolas,'SFMono-Regular',monospace;">{{.Code}}</span>
</td></tr>
</table>
<p style="margin:0;color:rgba(22,32,26,0.4);font-size:12px;line-height:1.6;">Didn't try to sign up? You can safely ignore this email.</p>`,
	}),
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
	whatsapp whatsapp.Provider
	otpTTL   time.Duration
}

func New(pool *pgxpool.Pool, argon2 config.Argon2, identitySvc *identity.Service, emailProvider email.Provider, whatsappProvider whatsapp.Provider, otpTTL time.Duration) *Service {
	return &Service{pool: pool, argon2: argon2, identity: identitySvc, email: emailProvider, whatsapp: whatsappProvider, otpTTL: otpTTL}
}

func newID() (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("generate id: %w", err)
	}
	return id, nil
}

// humanDuration formats a whole-minute duration as "N minute(s)" for the
// OTP message text -- Go's own time.Duration.String() (e.g. "10m0s") is
// fine for logs but reads as a bug to an actual recipient. Duplicated in
// internal/verification rather than shared, per this codebase's "each
// feature package stays self-contained" convention.
func humanDuration(d time.Duration) string {
	minutes := int(d.Round(time.Minute).Minutes())
	if minutes == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}

// Start hashes password immediately (never held in plaintext across the
// wait for a code, even briefly), generates and delivers a 6-digit OTP over
// channel, and upserts one pending signup row for identifier -- calling
// Start again for the same identifier before it is verified replaces the
// code, which is the entire "resend" mechanism; there is no separate
// resend endpoint. Returns ErrAlreadyRegistered if identifier already
// belongs to a real account, without sending anything.
func (s *Service) Start(ctx context.Context, channel Channel, identifier, displayName, password string) error {
	var registered bool
	var err error
	switch channel {
	case ChannelEmail:
		registered, err = s.identity.EmailIsRegistered(ctx, identifier)
	case ChannelWhatsApp:
		registered, err = s.identity.PhoneIsRegistered(ctx, identifier)
	default:
		return ErrUnsupportedChannel
	}
	if err != nil {
		return fmt.Errorf("check existing registration: %w", err)
	}
	if registered {
		return ErrAlreadyRegistered
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
		switch channel {
		case ChannelEmail:
			_, err := q.UpsertSignupVerificationByEmail(ctx, sqlc.UpsertSignupVerificationByEmailParams{
				ID: id, Email: pgText(identifier), DisplayName: displayName, PasswordHash: passwordHash,
				OtpHash: auth.HashToken(code), ExpiresAt: pgTimestamptz(time.Now().Add(s.otpTTL)),
			})
			return err
		default: // ChannelWhatsApp
			_, err := q.UpsertSignupVerificationByPhone(ctx, sqlc.UpsertSignupVerificationByPhoneParams{
				ID: id, Phone: pgText(identifier), DisplayName: displayName, PasswordHash: passwordHash,
				OtpHash: auth.HashToken(code), ExpiresAt: pgTimestamptz(time.Now().Add(s.otpTTL)),
			})
			return err
		}
	})
	if err != nil {
		return fmt.Errorf("store pending signup: %w", err)
	}

	if channel == ChannelEmail {
		msg, err := otpEmailTemplate.Render(identifier, otpEmailData{
			Code:      code,
			ExpiresIn: humanDuration(s.otpTTL),
		})
		if err != nil {
			return fmt.Errorf("render verification email: %w", err)
		}
		if err := s.email.Send(ctx, msg); err != nil {
			return fmt.Errorf("send verification email: %w", err)
		}
		return nil
	}

	text := fmt.Sprintf("Your justbarme verification code is %s. It expires in %s.", code, humanDuration(s.otpTTL))
	if _, err := s.whatsapp.Send(ctx, whatsapp.Message{To: identifier, Text: text}); err != nil {
		return fmt.Errorf("send verification whatsapp message: %w", err)
	}
	return nil
}

// Verify checks code against the pending signup for identifier (looked up
// by channel). On success it creates the real user (from the password
// hashed at Start, never re-touching plaintext) and a normal session --
// identical to what POST /auth/login produces -- and deletes the pending
// row. Returns ErrNoPendingSignup if Start was never called (or the code
// already expired and was cleaned up), ErrInvalidOrExpiredCode for a wrong
// code, an expired-but-still-present row, or a row that has exhausted
// maxOTPAttempts.
func (s *Service) Verify(ctx context.Context, channel Channel, identifier, code, userAgent string, sessionTTL time.Duration) (identity.User, string, string, identity.Session, error) {
	var pending sqlc.SignupVerification
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		switch channel {
		case ChannelEmail:
			pending, err = q.GetSignupVerificationByEmail(ctx, identifier)
		default: // ChannelWhatsApp
			pending, err = q.GetSignupVerificationByPhone(ctx, pgText(identifier))
		}
		return err
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

	user, err := s.identity.CreateUserWithHashedPassword(ctx, toText(pending.Email), toText(pending.Phone), pending.DisplayName, pending.PasswordHash)
	if err != nil {
		if errors.Is(err, identity.ErrEmailTaken) || errors.Is(err, identity.ErrPhoneTaken) {
			return identity.User{}, "", "", identity.Session{}, ErrAlreadyRegistered
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
	// CreateUserWithHashedPassword above would now hit ErrEmailTaken/
	// ErrPhoneTaken).
	_ = store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		return q.DeleteSignupVerification(ctx, pending.ID)
	})

	return user, rawToken, rawCSRF, sess, nil
}
