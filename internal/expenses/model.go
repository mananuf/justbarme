package expenses

import (
	"time"

	"github.com/google/uuid"
)

const (
	PaymentMethodCash     = "cash"
	PaymentMethodTransfer = "transfer"
	PaymentMethodCard     = "card"
)

// Category is a business-owned expense category (docs/ARCHITECTURE.md
// §8.6). Never hard-deleted -- Active is the only removal path, same
// convention as catalogue.Category/catalogue.Product.
type Category struct {
	ID     uuid.UUID
	Name   string
	Active bool
}

// Expense is one posted, immutable expense record. ReversalOf is
// uuid.Nil on an ordinary expense; a reversal is a second Expense row
// with ReversalOf pointing back at the original -- never an edit or
// delete of it (docs/ARCHITECTURE.md §8.6's "an edit UI creates a
// reversal and replacement").
type Expense struct {
	ID            uuid.UUID
	LocationID    uuid.UUID
	CategoryID    uuid.UUID
	CategoryName  string
	Description   string
	AmountKobo    int64
	PaymentMethod string
	RecordedBy    uuid.UUID
	ReversalOf    uuid.UUID
	OccurredAt    time.Time
	CreatedAt     time.Time
}

// Summary is SumExpensesTotalSince's result: Total nets every row
// including reversals (a reversal's AmountKobo is negative by
// construction, same sign convention as sales.total_kobo); Count
// excludes reversal rows, same reasoning as sales' today_sale_count.
type Summary struct {
	TotalKobo int64
	Count     int64
}
