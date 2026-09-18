// Package verification implements the "verify before link" mechanism
// docs/PHASE_INVITATIONS_WHATSAPP.md requires whenever a phone or email is
// about to be attached to an ALREADY EXISTING user account -- whether from
// account settings ("add a phone/email to my account") or an invitation
// match (an invitee who already has an account under a different
// identifier). Distinct from internal/signup (a brand-new account) and
// internal/invitations (a brand-new membership): this package only ever
// mutates an existing users row, never creates one. Like internal/signup,
// it owns its own email.Provider/whatsapp.Provider dependencies and OTP
// mechanics, delegating the actual user mutation to internal/identity.
package verification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/auth"
	"github.com/mananuf/justbarme/internal/email"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/store"
	"github.com/mananuf/justbarme/internal/store/sqlc"
	"github.com/mananuf/justbarme/internal/whatsapp"
)

type Channel string

const (
	ChannelEmail    Channel = "email"
	ChannelWhatsApp Channel = "whatsapp"
)

type Purpose string

const (
	// PurposeAddIdentifier is account settings -- "add a phone/email to my
	// account."
	PurposeAddIdentifier Purpose = "add_identifier"
	// PurposeInvitationLink is an invitee who already has an account
	// under a different identifier, matched by an invitation.
	PurposeInvitationLink Purpose = "invitation_link"
)

// maxOTPAttempts and otpTTL are this package's own constants, deliberately
// not shared with internal/signup's (docs/PHASE_INVITATIONS_WHATSAPP.md
// §8: "own constant, deliberately not shared, since the two flows may
// need to diverge later"). 3 is tighter than signup's 5 -- this guards an
// account merge, a higher-stakes action than a brand-new signup. 10
// minutes matches signup's current default value, not by import.
const (
	maxOTPAttempts = 3
	otpTTL         = 10 * time.Minute
)

var otpEmailTemplate = email.MustNewHTMLTemplate("identity_verification_otp",
	"Your justbarme verification code",
	"Your code is {{.Code}}. It expires in {{.ExpiresIn}}.",
	email.MustLayout(email.LayoutData{
		Preheader: "Your verification code: {{.Code}}",
		Content: `<h1 style="margin:0 0 12px;color:` + email.ColorInk + `;font-size:22px;line-height:1.3;font-weight:700;">Your verification code</h1>
<p style="margin:0 0 28px;color:rgba(22,32,26,0.55);font-size:14px;line-height:1.6;">Enter this code to confirm it's really you before this is added to your justbarme account. It expires in {{.ExpiresIn}}.</p>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:0 0 28px;">
<tr><td align="center" style="background-color:` + email.ColorInk + `;border-radius:14px;padding:20px 24px;">
<span style="color:` + email.ColorCream + `;font-size:32px;font-weight:700;letter-spacing:0.3em;font-family:Menlo,Consolas,'SFMono-Regular',monospace;">{{.Code}}</span>
</td></tr>
</table>
<p style="margin:0;color:rgba(22,32,26,0.4);font-size:12px;line-height:1.6;">Didn't request this? You can safely ignore this email.</p>`,
	}),
)

type otpEmailData struct {
	Code      string
	ExpiresIn string
}

type Verification struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Channel    Channel
	Identifier string
	Purpose    Purpose
}

type Service struct {
	pool     *pgxpool.Pool
	identity *identity.Service
	email    email.Provider
	whatsapp whatsapp.Provider
}

func New(pool *pgxpool.Pool, identitySvc *identity.Service, emailProvider email.Provider, whatsappProvider whatsapp.Provider) *Service {
	return &Service{pool: pool, identity: identitySvc, email: emailProvider, whatsapp: whatsappProvider}
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
// internal/signup rather than shared, per this codebase's "each feature
// package stays self-contained" convention.
func humanDuration(d time.Duration) string {
	minutes := int(d.Round(time.Minute).Minutes())
	if minutes == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}

// Start proves control of identifier before it can be attached to userID's
// account: generates and delivers a 6-digit OTP over channel, and stores a
// pending identity_verifications row. Returns ErrIdentifierAlreadyInUse if
// identifier already belongs to a different account (a duplicate on this
// account -- re-verifying the same phone/email you already have -- is not
// this package's concern; callers should short-circuit that themselves).
func (s *Service) Start(ctx context.Context, userID uuid.UUID, channel Channel, identifier string, purpose Purpose) (Verification, error) {
	var registered bool
	var err error
	switch channel {
	case ChannelEmail:
		registered, err = s.identity.EmailIsRegistered(ctx, identifier)
	case ChannelWhatsApp:
		registered, err = s.identity.PhoneIsRegistered(ctx, identifier)
	default:
		return Verification{}, ErrUnsupportedChannel
	}
	if err != nil {
		return Verification{}, fmt.Errorf("check existing registration: %w", err)
	}
	if registered {
		return Verification{}, ErrIdentifierAlreadyInUse
	}

	code, err := auth.GenerateOTP()
	if err != nil {
		return Verification{}, err
	}
	id, err := newID()
	if err != nil {
		return Verification{}, err
	}

	err = store.WithApp(ctx, s.pool, userID, func(ctx context.Context, q *sqlc.Queries) error {
		_, err := q.CreateIdentityVerification(ctx, sqlc.CreateIdentityVerificationParams{
			ID: id, UserID: userID, Channel: string(channel), Identifier: identifier,
			OtpHash: auth.HashToken(code), Purpose: string(purpose),
			ExpiresAt: pgTimestamptz(time.Now().Add(otpTTL)),
		})
		return err
	})
	if err != nil {
		return Verification{}, fmt.Errorf("store identity verification: %w", err)
	}

	if channel == ChannelEmail {
		msg, err := otpEmailTemplate.Render(identifier, otpEmailData{Code: code, ExpiresIn: humanDuration(otpTTL)})
		if err != nil {
			return Verification{}, fmt.Errorf("render verification email: %w", err)
		}
		if err := s.email.Send(ctx, msg); err != nil {
			return Verification{}, fmt.Errorf("send verification email: %w", err)
		}
	} else {
		text := fmt.Sprintf("Your justbarme verification code is %s. It expires in %s.", code, humanDuration(otpTTL))
		if _, err := s.whatsapp.Send(ctx, whatsapp.Message{To: identifier, Text: text}); err != nil {
			return Verification{}, fmt.Errorf("send verification whatsapp message: %w", err)
		}
	}

	return Verification{ID: id, UserID: userID, Channel: channel, Identifier: identifier, Purpose: purpose}, nil
}

// Confirm checks code against the pending verification and, on success,
// attaches its identifier to its user (AddEmailToUser/AddPhoneToUser) and
// deletes the pending row. Returns ErrVerificationNotFound if id doesn't
// exist (or was already confirmed and cleaned up), ErrInvalidOrExpiredCode
// for a wrong code, an expired-but-still-present row, or a row that has
// exhausted maxOTPAttempts.
func (s *Service) Confirm(ctx context.Context, userID, id uuid.UUID, code string) (identity.User, error) {
	var pending sqlc.IdentityVerification
	err := store.WithApp(ctx, s.pool, userID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetIdentityVerificationByID(ctx, id)
		if err != nil {
			return err
		}
		pending = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.User{}, ErrVerificationNotFound
		}
		return identity.User{}, fmt.Errorf("look up identity verification: %w", err)
	}
	if pending.UserID != userID {
		// Never disclose that a verification exists for someone else's
		// account -- same shape as it simply not existing.
		return identity.User{}, ErrVerificationNotFound
	}

	if pending.Attempts >= maxOTPAttempts || time.Now().After(pending.ExpiresAt.Time) {
		return identity.User{}, ErrInvalidOrExpiredCode
	}
	if auth.HashToken(code) != pending.OtpHash {
		_ = store.WithApp(ctx, s.pool, userID, func(ctx context.Context, q *sqlc.Queries) error {
			return q.IncrementIdentityVerificationAttempts(ctx, pending.ID)
		})
		return identity.User{}, ErrInvalidOrExpiredCode
	}

	var user identity.User
	if pending.Channel == string(ChannelEmail) {
		user, err = s.identity.AddEmailToUser(ctx, pending.UserID, pending.Identifier)
	} else {
		user, err = s.identity.AddPhoneToUser(ctx, pending.UserID, pending.Identifier)
	}
	if err != nil {
		if errors.Is(err, identity.ErrEmailTaken) || errors.Is(err, identity.ErrPhoneTaken) {
			return identity.User{}, ErrIdentifierAlreadyInUse
		}
		return identity.User{}, fmt.Errorf("attach verified identifier: %w", err)
	}

	// Best-effort: the identifier is already attached and usable even if
	// this cleanup fails.
	_ = store.WithApp(ctx, s.pool, userID, func(ctx context.Context, q *sqlc.Queries) error {
		return q.DeleteIdentityVerification(ctx, pending.ID)
	})

	return user, nil
}
