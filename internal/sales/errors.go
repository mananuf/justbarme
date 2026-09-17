package sales

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNoItems           = errors.New("a sale must include at least one item")
	ErrVariantNotFound   = errors.New("product variant not found")
	ErrSaleNotFound      = errors.New("sale not found")
	ErrAlreadyReversed   = errors.New("this sale has already been reversed")
	ErrReviewNotFound    = errors.New("sale review not found or already resolved")
	ErrInvalidPaymentAmt = errors.New("payment amount must equal the sale total")

	ErrTableNotFound              = errors.New("table not found")
	ErrTableLabelTaken            = errors.New("a table with this label already exists")
	ErrCustomerNotFound           = errors.New("customer not found")
	ErrBillNotFound               = errors.New("bill not found")
	ErrPaymentNotFound            = errors.New("payment not found")
	ErrBillNotOpen                = errors.New("bill is not open")
	ErrBillNotClosedUnpaid        = errors.New("bill is not closed with an outstanding balance")
	ErrBillNotPayable             = errors.New("bill cannot receive a payment in its current state")
	ErrCreditRequiresCustomer     = errors.New("a named customer is required to close a bill with an outstanding balance")
	ErrOverpayment                = errors.New("payment exceeds the outstanding balance")
	ErrWriteOffExceedsBalance     = errors.New("write-off amount exceeds the outstanding balance")
	ErrBillHasOutstandingActivity = errors.New("bill cannot be voided: it still has effective sales or an outstanding balance")
	ErrPaymentAlreadyReversed     = errors.New("this payment has already been reversed")
	ErrPaymentIsReversal          = errors.New("a reversal payment cannot itself be reversed")
	ErrWriteOffReasonRequired     = errors.New("a write-off reason is required")
	ErrBillNotEditable            = errors.New("bill can only be edited while open or awaiting payment")
	ErrInsufficientQuantityOnBill = errors.New("cannot remove more than is currently on the bill")
	ErrBillFullyPaid              = errors.New("this bill is already fully paid; removing an item would leave a negative balance")
)

// pgErrorCode reports err's Postgres SQLSTATE code, if any. Duplicated
// rather than shared across feature packages, per docs/ARCHITECTURE.md's
// "each feature package stays self-contained" (see internal/catalogue and
// internal/inventory's identical helpers).
func pgErrorCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
)
