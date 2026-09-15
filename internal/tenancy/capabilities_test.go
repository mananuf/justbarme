package tenancy

import "testing"

func TestOwnerHasEveryCapability(t *testing.T) {
	owner := ForRole(RoleOwner)
	for _, c := range allCapabilities {
		if !owner.Has(c) {
			t.Errorf("expected owner to have capability %q", c)
		}
	}
}

func TestStaffCannotApproveAdjustmentsOrWriteOffDebt(t *testing.T) {
	staff := ForRole(RoleStaff)
	forbidden := []Capability{
		CapabilityInventoryAdjustmentApprove,
		CapabilityBillsWriteOff,
		CapabilityMembersManage,
		CapabilityDevicesManage,
		CapabilityCatalogueManage,
		CapabilityReviewsResolve,
	}
	for _, c := range forbidden {
		if staff.Has(c) {
			t.Errorf("expected staff to NOT have capability %q", c)
		}
	}
	if !staff.Has(CapabilitySalesRecord) {
		t.Error("expected staff to have sales:record")
	}
}

func TestForRoleUnknownReturnsNil(t *testing.T) {
	if ForRole("superadmin") != nil {
		t.Fatal("expected unknown role to yield a nil capability set")
	}
}

func TestOfflineSafeExcludesOwnerOnlyCapabilities(t *testing.T) {
	safe := OfflineSafe(RoleOwner)
	mustExclude := []Capability{
		CapabilityMembersManage,
		CapabilityDevicesManage,
		CapabilityCatalogueManage,
		CapabilityReviewsResolve,
		CapabilityBillsWriteOff,
		CapabilitySalesReverse,
	}
	for _, c := range mustExclude {
		if safe.Has(c) {
			t.Errorf("expected offline-safe set to exclude %q even for an owner", c)
		}
	}
	if !safe.Has(CapabilitySalesRecord) {
		t.Error("expected offline-safe set to include sales:record")
	}
}
