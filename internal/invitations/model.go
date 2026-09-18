// Package invitations implements staff invitations
// (docs/IMPLEMENTATION_PLAN.md Phase 2 deferred POST /invitations from day
// one; docs/PHASE_INVITATIONS_WHATSAPP.md builds it now, defaulting to
// WhatsApp delivery with email as an explicit fallback). Like
// internal/signup, it owns its own email.Provider/whatsapp.Provider
// dependencies and delivery mechanics, delegating the actual user/
// membership mutation to internal/identity. It does not depend on
// internal/verification -- the "an invitee already has an account under a
// different identifier" verify-before-link flow is orchestrated by the
// HTTP handler layer across both packages, keeping each package's own
// responsibility narrow (docs/PHASE_INVITATIONS_WHATSAPP.md §4, flow E).
package invitations

import (
	"time"

	"github.com/google/uuid"
)

const (
	RoleOwner = "owner"
	RoleStaff = "staff"

	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRevoked  = "revoked"
	StatusExpired  = "expired"
)

// invitationTTL is how long an invite link stays valid -- a week is
// standard practice for this kind of link (long enough that a staff
// member checking WhatsApp a day later isn't locked out, short enough
// that a stale, unaccepted invite doesn't linger indefinitely).
const invitationTTL = 7 * 24 * time.Hour

type Invitation struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	InvitedBy  uuid.UUID
	Phone      string
	Email      string
	Role       string
	Status     string
	ExpiresAt  time.Time
	AcceptedBy uuid.UUID
	AcceptedAt time.Time
	CreatedAt  time.Time
}
