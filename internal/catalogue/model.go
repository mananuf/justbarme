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
}

// Variant is the sold and stocked unit, e.g. "50cl Bottle".
type Variant struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	ProductID  uuid.UUID
	Name       string
	Active     bool
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

// VariantWithPrice pairs a variant with its current price. Every variant
// carries exactly one from the moment it is created (Service.CreateVariant
// requires an initial price), so this is never a zero Price for an active
// read path.
type VariantWithPrice struct {
	Variant
	CurrentPrice Price
}

// ProductWithVariants is one product and its variants, each with its
// current price -- the shape a single catalogue read assembles for a
// business (Service.ListCatalogue).
type ProductWithVariants struct {
	Product
	Variants []VariantWithPrice
}
