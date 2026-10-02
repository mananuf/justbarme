// Package catalogue implements platform templates, categories, products,
// variants, and price history -- what a business sells, configured before
// any sale can be accepted. Domain types here are deliberately distinct
// from the generated sqlc structs, matching internal/identity's convention.
package catalogue

import (
	"time"

	"github.com/google/uuid"
)

// Template is a platform-owned onboarding suggestion, not a live business
// product. See docs/ARCHITECTURE.md §8.2.
type Template struct {
	ID           uuid.UUID
	Name         string
	CategoryName string
	SortOrder    int32
}

type TemplateVariant struct {
	ID                 uuid.UUID
	TemplateID         uuid.UUID
	Name               string
	SuggestedPriceKobo int64
	SortOrder          int32
}

// TemplateWithVariants is one template and its suggested variants, as
// returned by a catalogue-templates listing.
type TemplateWithVariants struct {
	Template
	Variants []TemplateVariant
}

type Category struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	Name       string
	SortOrder  int32
	Active     bool
}

// Product is the conceptual product, e.g. "Guinness". CategoryID is
// uuid.Nil when the product is uncategorized.
type Product struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	CategoryID uuid.UUID
	Name       string
	Active     bool
	// TemplateID is the platform catalogue template this product came from,
	// or uuid.Nil for a genuinely custom product. It survives renames --
	// that is its whole point (see migration 000028).
	TemplateID uuid.UUID
}

// Variant is the sold and stocked unit, e.g. "50cl Bottle".
type Variant struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	ProductID  uuid.UUID
	Name       string
	Active     bool
	// TracksInventory is false for a non-stocked service item (a snooker
	// game, table time) -- internal/sales.postSaleRound skips the
	// inventory movement/balance/negative-review pipeline entirely for
	// such a variant, and internal/inventory.ReceiveStock refuses to
	// "restock" it. Defaults true for an ordinary physical drink.
	TracksInventory bool
}

// Price is one append-oriented effective-dated row. ValidTo is nil while
// this is the variant's current price.
type Price struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	VariantID  uuid.UUID
	AmountKobo int64
	ValidFrom  time.Time
	ValidTo    *time.Time
	CreatedBy  uuid.UUID
}

// PricePack is one "N units for a flat total" multi-buy rule for a
// variant -- see docs/PHASE_MULTIBUY_PRICING.md. ValidTo is nil while this
// is one of the variant's current packs. A variant with no current packs
// behaves exactly as it always has: plain linear quantity × current-price.
// internal/sales is what actually turns a set of these into what a
// specific quantity costs (it reads this table directly via the shared
// sqlc layer, per this codebase's "each feature package stays
// self-contained" convention -- see the Stock receiving section of
// CLAUDE.md -- rather than calling into this package's Service).
type PricePack struct {
	ID            uuid.UUID
	BusinessID    uuid.UUID
	VariantID     uuid.UUID
	PackQuantity  int32
	PackPriceKobo int64
	ValidFrom     time.Time
	ValidTo       *time.Time
	CreatedBy     uuid.UUID
}

// VariantWithPrice pairs a variant with its current price and current
// multi-buy packs, if any. Every variant carries exactly one current price
// from the moment it is created (Service.CreateVariant requires an initial
// price), so CurrentPrice is never zero-valued for an active read path;
// PricePacks is simply empty for a plain, linearly-priced variant (the
// common case).
type VariantWithPrice struct {
	Variant
	CurrentPrice Price
	PricePacks   []PricePack
}

// ProductWithVariants is one product and its variants, each with its
// current price -- the shape a single catalogue read assembles for a
// business (Service.ListCatalogue).
type ProductWithVariants struct {
	Product
	Variants []VariantWithPrice
}
