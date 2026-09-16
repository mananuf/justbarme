package platformadmin

// Capability is a stable string constant naming one thing a platform staff
// member may do. Fixed roles, same philosophy as tenancy.Capability: no
// configurable permission-builder, a small explicit set.
type Capability string

const (
	CapabilityBusinessesRead    Capability = "platform:businesses:read"
	CapabilityBusinessesSuspend Capability = "platform:businesses:suspend"
	CapabilityAuditRead         Capability = "platform:audit:read"
)

// Role names. Stored on platform_staff.role and checked verbatim.
const (
	RoleSupport    = "support"
	RoleSuperadmin = "superadmin"
)

// supportCapabilities is the fixed Support grant: oversight, never mutation.
var supportCapabilities = []Capability{
	CapabilityBusinessesRead,
	CapabilityAuditRead,
}

// allCapabilities is every platform capability that exists. Superadmin
// receives all of them explicitly rather than a wildcard, matching
// tenancy.ForRole's reasoning exactly.
var allCapabilities = []Capability{
	CapabilityBusinessesRead,
	CapabilityBusinessesSuspend,
	CapabilityAuditRead,
}

// Set is an unordered collection of capabilities with O(1) membership
// checks.
type Set map[Capability]struct{}

func newSet(caps []Capability) Set {
	s := make(Set, len(caps))
	for _, c := range caps {
		s[c] = struct{}{}
	}
	return s
}

func (s Set) Has(c Capability) bool {
	_, ok := s[c]
	return ok
}

// ForRole returns the fixed capability set for role, or nil if role is not
// a recognized platform role.
func ForRole(role string) Set {
	switch role {
	case RoleSuperadmin:
		return newSet(allCapabilities)
	case RoleSupport:
		return newSet(supportCapabilities)
	default:
		return nil
	}
}
