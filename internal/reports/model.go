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
