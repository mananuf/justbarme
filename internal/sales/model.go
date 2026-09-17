// Package sales implements walk-in selling -- a single-device slice of
// docs/ARCHITECTURE.md §8.3, deliberately built without Phase 4's
// multi-device sync foundation (deferred until invitations bring a second
// device into a real business). See docs/PHASE_STOCK_RECEIVING.md's
// sibling reasoning; this package follows the same "thin, honest slice"
// approach.
package sales

import (
	"time"

	"github.com/google/uuid"
)

const (
	PaymentMethodCash     = "cash"
	PaymentMethodTransfer = "transfer"
	PaymentMethodCard     = "card"

	ReviewReasonDeactivatedVariant = "deactivated_variant"
	ReviewReasonPriceMismatch      = "price_mismatch"

	BillStatusOpen         = "open"
	BillStatusClosedUnpaid = "closed_unpaid"
	BillStatusSettled      = "settled"
	BillStatusVoid         = "void"
)

// SaleItemInput is one line of a sale as submitted by the caller.
// UnitPriceKobo is preserved exactly as the actual charge
// (docs/ARCHITECTURE.md §8.3: "Preserve description and actual unit-price
// snapshots") -- never overwritten with a server-recomputed price. Only
// the line and header TOTALS are recomputed server-side; a client-supplied
// total or line total is never trusted.
type SaleItemInput struct {
	VariantID     uuid.UUID
	Quantity      int32
	UnitPriceKobo int64
}

type PaymentInput struct {
	AmountKobo int64
	Method     string
}

type SaleItem struct {
	ID            uuid.UUID
	VariantID     uuid.UUID
	Description   string
	Quantity      int32
	UnitPriceKobo int64
	LineTotalKobo int64
}

type Payment struct {
	ID         uuid.UUID
	AmountKobo int64
	Method     string
}

// Review flags a sale item whose variant was deactivated, or whose
// submitted price didn't match anything ever actually in effect, by the
// time the sale posted. It never changes the sale it's attached to -- the
// sale always posts exactly as submitted; a review is a flag for the
// owner, never a rejection or a silent correction.
type Review struct {
	ID         uuid.UUID
	SaleItemID uuid.UUID
	Reason     string
	Status     string
}

// Sale is one posted walk-in sale (or, when ReversalOf is not uuid.Nil, a
// reversal of an earlier one).
type Sale struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	BillID     uuid.UUID
	LocationID uuid.UUID
	SellerID   uuid.UUID
	OccurredAt time.Time
	ReceivedAt time.Time
	TotalKobo  int64
	ReversalOf uuid.UUID
	Items      []SaleItem
	Payment    Payment
	Reviews    []Review
}

// Table is a business-configured label (T1-T5 style, docs/ARCHITECTURE.md
// §8.3). No kitchen/routing behavior.
type Table struct {
	ID     uuid.UUID
	Label  string
	Active bool
}

// Customer is optional lightweight identity. Name is mandatory on the row
// itself -- the "optional" part of customer identity is whether a *bill*
// references one at all, not whether a customer record can be nameless.
type Customer struct {
	ID    uuid.UUID
	Name  string
	Phone string
	Email string
	Notes string
}

// Bill is the unified tab/table/walk-in concept (docs/ARCHITECTURE.md
// §8.3). TableID/CustomerID are uuid.Nil when not associated with either
// (an ordinary walk-in). BalanceKobo is a rebuildable projection --
// sum(sales.total_kobo) - sum(payments.amount_kobo) -
// sum(bill_write_offs.amount_kobo) for this bill -- recomputed and
// persisted by every bill-mutating Service method, in the same
// transaction as the write that changed it. It is never itself the
// source of truth.
type Bill struct {
	ID          uuid.UUID
	LocationID  uuid.UUID
	Status      string
	TableID     uuid.UUID
	CustomerID  uuid.UUID
	BalanceKobo int64
	OpenedAt    time.Time
}

// WriteOff is an owner-only, reasoned forgiveness of part or all of a
// bill's outstanding balance. Never a payment -- no money changes hands.
type WriteOff struct {
	ID         uuid.UUID
	BillID     uuid.UUID
	AmountKobo int64
	Reason     string
	CreatedAt  time.Time
}

// BillDetail is a bill with everything needed to render it: its rounds
// (sales), payments, and write-offs.
type BillDetail struct {
	Bill
	Sales     []Sale
	Payments  []Payment
	WriteOffs []WriteOff
}
