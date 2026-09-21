package reports

import (
	"time"

	"github.com/google/uuid"
)

// SalesDay is one day's bucketed sales figures, backing the Sales
// report's daily-revenue heatmap.
type SalesDay struct {
	Day       time.Time
	TotalKobo int64
	SaleCount int64
}

// ProductQuantity is one variant's units-sold total within a range.
type ProductQuantity struct {
	VariantID   uuid.UUID
	VariantName string
	ProductName string
	UnitsSold   int64
}

// StaffSales is one seller's totals within a range. SellerName is
// resolved by the HTTP layer (the same resolveSellerNames helper every
// other seller-facing endpoint already uses), not this package -- a
// report package has no business reaching into internal/identity.
type StaffSales struct {
	SellerID  uuid.UUID
	TotalKobo int64
	SaleCount int64
}

// ExpensesByCategory is one category's totals within a range.
type ExpensesByCategory struct {
	CategoryID   uuid.UUID
	CategoryName string
	TotalKobo    int64
	ExpenseCount int64
}

// GrossMargin is one variant's revenue/cost picture within a range --
// docs/PHASE_FIFO_COSTING.md §8. GrossMarginKobo is Revenue - ResolvedCOGS,
// but must never be presented as complete on its own: UnresolvedUnits is
// the count of sold units whose cost is still pending (an unresolved
// oversell, see docs/ARCHITECTURE.md §8.5's "omit or clearly label COGS/
// profit as unavailable while unresolved quantities exist") -- a nonzero
// value here means GrossMarginKobo is a lower bound on true cost, not the
// final number, and callers must say so rather than showing it bare.
type GrossMargin struct {
	VariantID        uuid.UUID
	VariantName      string
	ProductName      string
	RevenueKobo      int64
	ResolvedCogsKobo int64
	UnresolvedUnits  int64
}

// GrossMarginKobo is Revenue - ResolvedCOGS -- see the type's own doc
// comment on why this is never the final word when UnresolvedUnits != 0.
func (g GrossMargin) GrossMarginKobo() int64 {
	return g.RevenueKobo - g.ResolvedCogsKobo
}

// StockDiscrepancy is one stock-count line whose physical count didn't
// match its expected quantity -- reuses Phase 7's own data directly, no
// new schema (docs/PHASE_EXPENSES_DASHBOARD_ACTIVITY_REPORTS.md §3).
type StockDiscrepancy struct {
	ID               uuid.UUID
	VariantID        uuid.UUID
	VariantName      string
	ProductName      string
	ExpectedQuantity int32
	PhysicalQuantity int32
	Variance         int32
	IsStale          bool
	CountedAt        time.Time
	CountedBy        uuid.UUID
}
