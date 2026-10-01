package activity

import (
	"time"

	"github.com/google/uuid"
)

const (
	TypeSale                = "sale"
	TypeExpense             = "expense"
	TypeInventoryAdjustment = "inventory_adjustment"
	TypeStockReceipt        = "stock_receipt"
)

// Entry is one row of the unified activity feed (docs/PHASE_EXPENSES_
// DASHBOARD_ACTIVITY_REPORTS.md §3) -- a query-time UNION across sales,
// expenses, approved inventory adjustments, and stock receipts, not a new
// writer table. AmountKobo is cash impact (positive = money in, negative
// = money out), not each source table's own storage sign convention.
type Entry struct {
	ID         uuid.UUID
	Type       string
	ActorID    uuid.UUID
	OccurredAt time.Time
	Summary    string
	AmountKobo int64
}

// Filter narrows ListActivity. A zero-value field means "no filter on
// this dimension" -- ActorID == uuid.Nil, Type == "", and a zero
// time.Time for StartAt/EndAt.
type Filter struct {
	ActorID uuid.UUID
	Type    string
	StartAt time.Time
	EndAt   time.Time
}

const (
	FlagSourceSale                = "sale"
	FlagSourceExpense             = "expense"
	FlagSourceInventoryAdjustment = "inventory_adjustment"
	FlagSourceStockReceipt        = "stock_receipt"
)

// Flag is a staff-raised concern about one activity entry -- "staff must
// request review" generalized across all four activity types, rather than
// the sale-specific/inventory-specific review tables. It never changes the
// record it points at; an owner who agrees still performs the actual fix
// through that record's own correction mechanism (a reversal, or a new
// adjustment).
type Flag struct {
	ID             uuid.UUID
	SourceType     string
	SourceID       uuid.UUID
	FlaggedBy      uuid.UUID
	Reason         string
	Status         string
	ResolvedBy     uuid.UUID
	ResolutionNote string
	CreatedAt      time.Time
	ResolvedAt     time.Time
}

// DayCount is one day's activity count, backing the Activity page's
// GitHub-style heatmap. Day is a local calendar date (already bucketed in
// the business's own timezone) with a zero time-of-day.
type DayCount struct {
	Day   time.Time
	Count int64
}
