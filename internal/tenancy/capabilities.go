package tenancy

// Capability is a stable string constant naming one thing a principal may
// do. Capabilities are mapped from the two MVP roles in Go rather than
// stored as a configurable permission schema — see docs/API_CONTRACT.md §6.
type Capability string

const (
	CapabilityBusinessRead   Capability = "business:read"
	CapabilityBusinessUpdate Capability = "business:update"

	CapabilityMembersRead   Capability = "members:read"
	CapabilityMembersManage Capability = "members:manage"

	CapabilityDevicesManage Capability = "devices:manage"

	CapabilityCatalogueRead   Capability = "catalogue:read"
	CapabilityCatalogueManage Capability = "catalogue:manage"

	CapabilitySalesRecord  Capability = "sales:record"
	CapabilitySalesReverse Capability = "sales:reverse"

	CapabilityBillsRead     Capability = "bills:read"
	CapabilityBillsManage   Capability = "bills:manage"
	CapabilityBillsShare    Capability = "bills:share"
	CapabilityBillsWriteOff Capability = "bills:write_off"

	CapabilityCreditGrant Capability = "credit:grant"

	CapabilityPaymentsRecord  Capability = "payments:record"
	CapabilityPaymentsReverse Capability = "payments:reverse"

	CapabilityExpensesRecord  Capability = "expenses:record"
	CapabilityExpensesReverse Capability = "expenses:reverse"

	CapabilityInventoryRead              Capability = "inventory:read"
	CapabilityInventoryReceive           Capability = "inventory:receive"
	CapabilityInventoryCount             Capability = "inventory:count"
	CapabilityInventoryAdjustmentRequest Capability = "inventory:adjustment_request"
	CapabilityInventoryAdjustmentApprove Capability = "inventory:adjustment_approve"

	CapabilityActivityRead Capability = "activity:read"

	CapabilityReviewsRead    Capability = "reviews:read"
	CapabilityReviewsResolve Capability = "reviews:resolve"

	CapabilityReportsRead Capability = "reports:read"
)

// Role names. Stored on business_memberships.role and checked verbatim.
const (
	RoleOwner = "owner"
	RoleStaff = "staff"
)

// staffCapabilities is the fixed Staff grant. Whether Staff also receive
// inventory:receive remains a pilot observation (docs/API_CONTRACT.md §6);
// it defaults to Owner-only until confirmed.
var staffCapabilities = []Capability{
	CapabilityBusinessRead,
	CapabilityMembersRead,
	CapabilityCatalogueRead,
	CapabilitySalesRecord,
	CapabilityBillsRead,
	CapabilityBillsManage,
	CapabilityBillsShare,
	CapabilityCreditGrant,
	CapabilityPaymentsRecord,
	CapabilityExpensesRecord,
	CapabilityInventoryRead,
	CapabilityInventoryCount,
	CapabilityInventoryAdjustmentRequest,
}

// allCapabilities is every capability that exists. Owners receive all of
// them explicitly rather than a wildcard, so a signed offline lease never
// needs to carry one either (see OfflineSafe).
var allCapabilities = []Capability{
	CapabilityBusinessRead, CapabilityBusinessUpdate,
	CapabilityMembersRead, CapabilityMembersManage,
	CapabilityDevicesManage,
	CapabilityCatalogueRead, CapabilityCatalogueManage,
	CapabilitySalesRecord, CapabilitySalesReverse,
	CapabilityBillsRead, CapabilityBillsManage, CapabilityBillsShare, CapabilityBillsWriteOff,
	CapabilityCreditGrant,
	CapabilityPaymentsRecord, CapabilityPaymentsReverse,
	CapabilityExpensesRecord, CapabilityExpensesReverse,
	CapabilityInventoryRead, CapabilityInventoryReceive, CapabilityInventoryCount,
	CapabilityInventoryAdjustmentRequest, CapabilityInventoryAdjustmentApprove,
	CapabilityActivityRead,
	CapabilityReviewsRead, CapabilityReviewsResolve,
	CapabilityReportsRead,
}

// offlineSafeCapabilities is the subset a signed offline lease may carry.
// Owner-only approval, reversal, write-off, member, device, catalogue
// mutation, and review resolution all require online server authorization.
var offlineSafeCapabilities = []Capability{
	CapabilityCatalogueRead,
	CapabilitySalesRecord,
	CapabilityBillsRead,
	CapabilityBillsManage,
	CapabilityBillsShare,
	CapabilityCreditGrant,
	CapabilityPaymentsRecord,
	CapabilityExpensesRecord,
	CapabilityInventoryRead,
	CapabilityInventoryCount,
	CapabilityInventoryAdjustmentRequest,
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

// List returns the set's members. Order is not significant.
func (s Set) List() []Capability {
	out := make([]Capability, 0, len(s))
	for c := range s {
		out = append(out, c)
	}
	return out
}

// ForRole returns the fixed capability set for role, or nil if role is not
// a recognized MVP role.
func ForRole(role string) Set {
	switch role {
	case RoleOwner:
		return newSet(allCapabilities)
	case RoleStaff:
		return newSet(staffCapabilities)
	default:
		return nil
	}
}

// OfflineSafe intersects a role's capabilities with the subset a device may
// hold offline, for embedding in a signed lease.
func OfflineSafe(role string) Set {
	full := ForRole(role)
	safe := make(Set, len(offlineSafeCapabilities))
	for _, c := range offlineSafeCapabilities {
		if full.Has(c) {
			safe[c] = struct{}{}
		}
	}
	return safe
}
