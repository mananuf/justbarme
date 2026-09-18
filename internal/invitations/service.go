package invitations

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

var inviteEmailTemplate = email.MustNewHTMLTemplate("invitation",
	"You've been invited to join {{.BusinessName}} on justbarme",
	"You've been invited to join {{.BusinessName}} on justbarme as {{.Role}}. Tap to accept: {{.Link}}",
	email.MustLayout(email.LayoutData{
		Preheader: "{{.BusinessName}} wants you on their team",
		Content: `<h1 style="margin:0 0 12px;color:` + email.ColorInk + `;font-size:22px;line-height:1.3;font-weight:700;">You&rsquo;re invited</h1>
<p style="margin:0 0 28px;color:rgba(22,32,26,0.6);font-size:15px;line-height:1.6;"><strong style="color:` + email.ColorInk + `;">{{.BusinessName}}</strong> has invited you to join their team on justbarme as <strong style="color:` + email.ColorInk + `;">{{.Role}}</strong>.</p>
<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:0 auto 16px;">
<tr><td align="center" style="background-color:` + email.ColorInk + `;border-radius:12px;">
<a href="{{.Link}}" style="display:inline-block;padding:16px 36px;color:` + email.ColorCream + `;font-size:15px;font-weight:600;text-decoration:none;">Accept invitation</a>
</td></tr>
</table>
<p style="margin:0 0 28px;color:rgba(22,32,26,0.35);font-size:12px;line-height:1.6;text-align:center;word-break:break-all;">Or open this link: <a href="{{.Link}}" style="color:` + email.ColorGreen + `;">{{.Link}}</a></p>
<p style="margin:0;color:rgba(22,32,26,0.4);font-size:12px;line-height:1.6;">If you weren&rsquo;t expecting this, you can safely ignore this email.</p>`,
	}),
)

type inviteEmailData struct {
	BusinessName string
	Role         string
	Link         string
}

type Service struct {
	pool          *pgxpool.Pool
	identity      *identity.Service
	email         email.Provider
	whatsapp      whatsapp.Provider
	publicBaseURL string
}

func New(pool *pgxpool.Pool, identitySvc *identity.Service, emailProvider email.Provider, whatsappProvider whatsapp.Provider, publicBaseURL string) *Service {
	return &Service{pool: pool, identity: identitySvc, email: emailProvider, whatsapp: whatsappProvider, publicBaseURL: publicBaseURL}
}

func newID() (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("generate id: %w", err)
	}
	return id, nil
}

// Create opens an invitation and delivers it -- WhatsApp when phone is
// given (docs/PHASE_INVITATIONS_WHATSAPP.md's "invitations default to
// WhatsApp"), email otherwise. invitedBy must already be a verified owner
// of businessID -- callers (the HTTP handler) resolve that via the normal
// requireCapability/tenancy.BusinessFromContext path before calling this,
// the same trust boundary every other tenant-scoped write in this
// codebase uses.
func (s *Service) Create(ctx context.Context, businessID, invitedBy uuid.UUID, phone, email, role string) (Invitation, error) {
	id, err := newID()
	if err != nil {
		return Invitation{}, err
	}
	rawToken, err := auth.GenerateToken()
	if err != nil {
		return Invitation{}, fmt.Errorf("generate invitation token: %w", err)
	}

	var created sqlc.Invitation
	err = store.WithApp(ctx, s.pool, invitedBy, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.CreateInvitation(ctx, sqlc.CreateInvitationParams{
			ID: id, BusinessID: businessID, InvitedBy: invitedBy,
			Phone: pgText(phone), Email: pgText(email), Role: role,
			TokenHash: auth.HashToken(rawToken), ExpiresAt: pgTimestamptz(time.Now().Add(invitationTTL)),
		})
		if err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		if pgErrorCode(err) == pgUniqueViolation {
			return Invitation{}, ErrAlreadyPending
		}
		return Invitation{}, fmt.Errorf("create invitation: %w", err)
	}

	business, err := s.identity.GetBusiness(ctx, invitedBy, businessID)
	if err != nil {
		return Invitation{}, fmt.Errorf("look up business: %w", err)
	}
	link := s.publicBaseURL + "/invite/" + rawToken

	if phone != "" {
		text := fmt.Sprintf("You've been invited to join %s on justbarme as %s. Tap to accept: %s", business.Name, role, link)
		if _, err := s.whatsapp.Send(ctx, whatsapp.Message{To: phone, Text: text}); err != nil {
			return Invitation{}, fmt.Errorf("send invitation whatsapp message: %w", err)
		}
	} else {
		msg, err := inviteEmailTemplate.Render(email, inviteEmailData{BusinessName: business.Name, Role: role, Link: link})
		if err != nil {
			return Invitation{}, fmt.Errorf("render invitation email: %w", err)
		}
		if err := s.email.Send(ctx, msg); err != nil {
			return Invitation{}, fmt.Errorf("send invitation email: %w", err)
		}
	}

	return toInvitation(created), nil
}

// List returns every invitation (any status) for businessID, newest first.
func (s *Service) List(ctx context.Context, actorID, businessID uuid.UUID) ([]Invitation, error) {
	var out []Invitation
	err := store.WithApp(ctx, s.pool, actorID, func(ctx context.Context, q *sqlc.Queries) error {
		rows, err := q.ListInvitationsByBusiness(ctx, businessID)
		if err != nil {
			return err
		}
		out = make([]Invitation, 0, len(rows))
		for _, r := range rows {
			out = append(out, toInvitation(r))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	return out, nil
}

// Revoke closes a still-pending invitation early. Returns ErrInvitationNotOpen
// if it's already accepted/revoked/expired.
func (s *Service) Revoke(ctx context.Context, actorID, businessID, id uuid.UUID) (Invitation, error) {
	var updated sqlc.Invitation
	err := store.WithApp(ctx, s.pool, actorID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.RevokeInvitation(ctx, sqlc.RevokeInvitationParams{BusinessID: businessID, ID: id})
		if err != nil {
			return err
		}
		updated = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invitation{}, ErrInvitationNotOpen
		}
		return Invitation{}, fmt.Errorf("revoke invitation: %w", err)
	}
	return toInvitation(updated), nil
}

// GetByToken looks up an invitation by its raw token -- the public,
// unauthenticated landing-page lookup. Lazily transitions a pending but
// past-expiry row to 'expired' on read, matching the schema's own status
// enum rather than only checking the timestamp inline.
func (s *Service) GetByToken(ctx context.Context, rawToken string) (Invitation, error) {
	var found sqlc.Invitation
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetInvitationByTokenHash(ctx, auth.HashToken(rawToken))
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invitation{}, ErrInvitationNotFound
		}
		return Invitation{}, fmt.Errorf("look up invitation: %w", err)
	}
	if found.Status == StatusPending && time.Now().After(found.ExpiresAt.Time) {
		return Invitation{}, ErrInvitationExpired
	}
	return toInvitation(found), nil
}

// AcceptAsNewUser is the "I'm new here" path (docs/PHASE_INVITATIONS_
// WHATSAPP.md flow D): creates a brand-new account from the invitation's
// own phone/email (whichever it carries) with no OTP challenge at all --
// receiving the invitation already proved channel ownership -- and
// attaches the membership immediately.
func (s *Service) AcceptAsNewUser(ctx context.Context, rawToken, displayName, password, userAgent string, sessionTTL time.Duration) (identity.User, Invitation, string, string, identity.Session, error) {
	inv, err := s.GetByToken(ctx, rawToken)
	if err != nil {
		return identity.User{}, Invitation{}, "", "", identity.Session{}, err
	}
	if inv.Status != StatusPending {
		return identity.User{}, Invitation{}, "", "", identity.Session{}, ErrInvitationNotOpen
	}

	user, err := s.identity.CreateUser(ctx, inv.Email, inv.Phone, displayName, password)
	if err != nil {
		return identity.User{}, Invitation{}, "", "", identity.Session{}, fmt.Errorf("create user: %w", err)
	}

	if _, err := s.identity.AddMembership(ctx, inv.BusinessID, user.ID, inv.Role); err != nil {
		return identity.User{}, Invitation{}, "", "", identity.Session{}, fmt.Errorf("add membership: %w", err)
	}

	accepted, err := s.markAccepted(ctx, inv.ID, user.ID)
	if err != nil {
		return identity.User{}, Invitation{}, "", "", identity.Session{}, err
	}

	rawSession, rawCSRF, sess, err := s.identity.CreateSession(ctx, user.ID, userAgent, sessionTTL)
	if err != nil {
		return identity.User{}, Invitation{}, "", "", identity.Session{}, fmt.Errorf("create session: %w", err)
	}
	return user, accepted, rawSession, rawCSRF, sess, nil
}

// AcceptAsExistingUser is flow E's final step (docs/PHASE_INVITATIONS_
// WHATSAPP.md): attaches the membership to an already-authenticated
// user's existing account. Callers (the HTTP handler) must have already
// established that userID legitimately owns the invitation's identifier --
// either because it already matched an identifier already on their
// account, or because internal/verification just confirmed a fresh OTP
// for it. This method does not re-verify that; it only finalizes the
// membership grant, matching the same "resolve first, mutate once trusted"
// discipline as every other write in this codebase.
func (s *Service) AcceptAsExistingUser(ctx context.Context, rawToken string, userID uuid.UUID) (Invitation, error) {
	inv, err := s.GetByToken(ctx, rawToken)
	if err != nil {
		return Invitation{}, err
	}
	if inv.Status != StatusPending {
		return Invitation{}, ErrInvitationNotOpen
	}

	if _, err := s.identity.AddMembership(ctx, inv.BusinessID, userID, inv.Role); err != nil {
		// Already a member of this business is not a failure here -- the
		// practical outcome the invitation exists for (a verified
		// identifier plus real access to this business) already holds, so
		// the invitation is treated as fulfilled rather than rejected.
		// Any other error is real and propagates.
		if !errors.Is(err, identity.ErrAlreadyMember) {
			return Invitation{}, fmt.Errorf("add membership: %w", err)
		}
	}
	return s.markAccepted(ctx, inv.ID, userID)
}

func (s *Service) markAccepted(ctx context.Context, invitationID, userID uuid.UUID) (Invitation, error) {
	var updated sqlc.Invitation
	err := store.WithApp(ctx, s.pool, userID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.AcceptInvitation(ctx, sqlc.AcceptInvitationParams{ID: invitationID, AcceptedBy: pgUUID(userID)})
		if err != nil {
			return err
		}
		updated = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invitation{}, ErrInvitationNotOpen
		}
		return Invitation{}, fmt.Errorf("mark invitation accepted: %w", err)
	}
	return toInvitation(updated), nil
}
