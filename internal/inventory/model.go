// Package inventory implements stock receiving -- a deliberately thin
// slice of docs/ARCHITECTURE.md §8.4's full inventory model, pulled forward
// ahead of the frozen phase order. See docs/PHASE_STOCK_RECEIVING.md for
// what this package does and does not cover: receipts, lots, movements,
// and a balance projection, no counts/approvals/reviews.
package inventory

import (
	"time"

	"github.com/google/uuid"
)

// ReceiptLine is one variant's quantity and total cost within a receipt --
// the caller-supplied side of Service.ReceiveStock. TotalCostKobo is the
// one number actually stored; per-unit cost is always a display-time
// calculation (see docs/ARCHITECTURE.md §8.4: "Store exact line total in
// kobo, not only a rounded unit cost").
type ReceiptLine struct {
	VariantID     uuid.UUID
	Quantity      int32
	TotalCostKobo int64
}

// ReceiptLineResult is one posted line, with the values Postgres actually
// assigned. NewBalance is this variant's stock at the receiving location
// immediately after this line posted -- captured from the same
// transaction's balance upsert, not a separate read, so it can never
// reflect a concurrent receipt landing in between. (GetBalances, used for
// display elsewhere, sums across locations; this is single-location by
// construction since it comes from one upsert row.)
type ReceiptLineResult struct {
	ID            uuid.UUID
	VariantID     uuid.UUID
	Quantity      int32
	TotalCostKobo int64
	NewBalance    int64
}

// Receipt is one posted stock receipt and its lines.
type Receipt struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	LocationID uuid.UUID
	ReceivedBy uuid.UUID
	ReceivedAt time.Time
	Lines      []ReceiptLineResult
}

// Adjustment reason categories -- docs/PHASE_INVENTORY_COUNTS_AND_
// ADJUSTMENTS.md §3: complimentary/broken/spoiled/staff-use consumption
// all share this one pipeline rather than a separate "consumption event"
// concept; the category plus the required note is what keeps "what
// happened" unambiguous.
const (
	AdjustmentReasonComplimentary   = "complimentary"
	AdjustmentReasonBroken          = "broken"
	AdjustmentReasonSpoiled         = "spoiled"
	AdjustmentReasonStaffUse        = "staff_use"
	AdjustmentReasonManual          = "manual"
	AdjustmentReasonCountCorrection = "count_correction"
)

const (
	AdjustmentStatusPending  = "pending"
	AdjustmentStatusApproved = "approved"
	AdjustmentStatusRejected = "rejected"
)

const (
	ReviewTypeNegativeInventory = "negative_inventory"
	ReviewTypeStaleStockCount   = "stale_stock_count"
)

// StockCountLineInput is the caller-submitted side of one counted variant.
// ExpectedQuantity is what the counting device believed the balance was
// (a live fetch, or web/src/lib/db.ts's catalogueCache if it was offline)
// -- staleness is decided by comparing this against the server's live
// balance at processing time, never by comparing timestamps.
type StockCountLineInput struct {
	VariantID        uuid.UUID
	ExpectedQuantity int32
	PhysicalQuantity int32
}

// StockCountLineResult is one processed count line, with Variance and
// IsStale as the server actually determined them.
type StockCountLineResult struct {
	ID               uuid.UUID
	VariantID        uuid.UUID
	ExpectedQuantity int32
	PhysicalQuantity int32
	Variance         int32
	IsStale          bool
}

// StockCount is one posted physical count and its lines. A non-stale line
// with a nonzero variance produces a pending AdjustmentRequest
// (ReasonCategory=AdjustmentReasonCountCorrection); a stale line produces
// an open InventoryReview (ReviewTypeStaleStockCount) instead of guessing.
type StockCount struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	LocationID uuid.UUID
	CountedBy  uuid.UUID
	StartedAt  time.Time
	Lines      []StockCountLineResult
}

// AdjustmentRequest is any signed inventory change that isn't a receipt or
// a sale. Staff may create one (pending); only an Owner may approve
// (posting the real movement) or reject (posting nothing) it.
// VariantName/ProductName are only populated by ListPendingAdjustmentRequests.
type AdjustmentRequest struct {
	ID                uuid.UUID
	LocationID        uuid.UUID
	VariantID         uuid.UUID
	RequestedBy       uuid.UUID
	QuantityDelta     int32
	ReasonCategory    string
	ReasonNote        string
	SourceCountLineID uuid.UUID // uuid.Nil if not sourced from a count line
	Status            string
	CreatedAt         time.Time
	VariantName       string
	ProductName       string
}

// InventoryReview flags a negative balance or a stale count for an owner
// to look at -- it never changes the movement/count it's attached to.
// VariantName/ProductName/Count* are only populated by ListOpenReviews.
type InventoryReview struct {
	ID                    uuid.UUID
	Type                  string
	VariantID             uuid.UUID
	LocationID            uuid.UUID
	RelatedMovementID     uuid.UUID
	RelatedCountLineID    uuid.UUID
	Status                string
	CreatedAt             time.Time
	VariantName           string
	ProductName           string
	CountExpectedQuantity int32
	CountPhysicalQuantity int32
}

// MovementHistoryEntry is one row of a variant's stock history -- a plain
// signed quantity change plus enough context (event type, and for
// adjustments the reason) to read as a real explanation, not just a number.
type MovementHistoryEntry struct {
	ID                       uuid.UUID
	QuantityDelta            int32
	CreatedAt                time.Time
	EventType                string
	ActorID                  uuid.UUID
	AdjustmentReasonCategory string
	AdjustmentReasonNote     string
	AdjustmentDecidedBy      uuid.UUID
}
