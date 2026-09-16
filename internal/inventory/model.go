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
