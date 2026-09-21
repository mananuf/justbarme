// Package business implements branding/settings for a tenant's own
// business record -- docs/PHASE_PILOT_RELEASE.md §5. It is deliberately
// self-contained (no dependency on internal/identity/internal/catalogue,
// same "each feature package stays self-contained" convention CLAUDE.md's
// Stock receiving section describes), reading and writing the businesses
// table directly through its own sqlc queries.
package business

import (
	"time"

	"github.com/google/uuid"
)

// Business is the full tenant-owned settings record -- a superset of
// identity.Business, which only carries the fields needed for tenancy
// (name/timezone/currency/status). LogoURL is derived from
// businesses.logo_object_key at read time via storage.Provider.URL, never
// stored itself -- see storage.Provider's own doc comment for why.
type Business struct {
	ID                  uuid.UUID
	Name                string
	Timezone            string
	Currency            string
	Phone               string
	Address             string
	ReceiptWording      string
	ReceiptFooter       string
	PaymentInstructions string
	LogoURL             string
	Status              string
	UpdatedAt           time.Time
}

// UpdateBrandingParams is a full replacement of the mutable branding
// fields, matching this codebase's PATCH convention (see
// catalogue.UpdateProductParams) -- the caller resends the whole set, not
// a partial merge where an omitted field means "leave unchanged."
type UpdateBrandingParams struct {
	Phone               string
	Address             string
	ReceiptWording      string
	ReceiptFooter       string
	PaymentInstructions string
}
