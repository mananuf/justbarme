// Package platformadmin implements platform-level staff: a completely
// separate identity space from internal/identity's business users, for
// operators who oversee businesses across the whole service rather than
// belonging to any one of them. See CLAUDE.md's platform admin section for
// the design rationale (no RLS bypass, everything here reads only
// non-tenant-owned tables, audited mutations).
package platformadmin

import (
	"time"

	"github.com/google/uuid"
)

type Staff struct {
	ID          uuid.UUID
	Email       string
	DisplayName string
	Role        string
	Status      string
	CreatedAt   time.Time
}

// Session is this package's view of a platform_sessions row. The raw
// bearer token is never stored and never returned from a lookup — only
// Service.CreateSession, at issuance time, ever sees it.
type Session struct {
	ID      uuid.UUID
	StaffID uuid.UUID
	// CSRFTokenHash is for internal comparison against an incoming
	// X-CSRF-Token header's hash only. Handlers must never serialize it.
	CSRFTokenHash string
	ExpiresAt     time.Time
}

// Business is the platform-oversight view of a business: name and status
// only. Its tenant-owned data is reachable solely through the audited
// aggregate activity summary (GET /platform/businesses/{id}/activity),
// composed at the HTTP layer -- never through this type.
type Business struct {
	ID        uuid.UUID
	Name      string
	Status    string
	Timezone  string
	Currency  string
	CreatedAt time.Time
}

// AuditEntry is one recorded platform-scoped action. TargetBusinessID is
// uuid.Nil for an action with no single business target.
type AuditEntry struct {
	ID               uuid.UUID
	StaffID          uuid.UUID
	Action           string
	TargetBusinessID uuid.UUID
	TargetStaffID    uuid.UUID
	Reason           string
	RequestID        string
	CreatedAt        time.Time
}

const (
	ActionLogin               = "login"
	ActionBusinessSuspended   = "business.suspended"
	ActionBusinessReactivated = "business.reactivated"
	// ActionBusinessActivityViewed is logged on every read of a business's
	// tenant-owned aggregate data, not just mutations.
	ActionBusinessActivityViewed = "business.activity_viewed"
	ActionStaffCreated           = "staff.created"
	ActionStaffRevoked           = "staff.revoked"
)

// Stats is the platform-wide overview, derived only from non-tenant tables.
type Stats struct {
	TotalBusinesses     int
	ActiveBusinesses    int
	SuspendedBusinesses int
	NewBusinessesLast7d int
	TotalUsers          int64
}
