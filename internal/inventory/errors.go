package inventory

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNoLines                   = errors.New("a stock receipt must include at least one line")
	ErrVariantNotFound           = errors.New("product variant not found")
	ErrNoCountLines              = errors.New("a stock count must include at least one line")
	ErrAdjustmentRequestNotFound = errors.New("adjustment request not found or already decided")
	ErrInventoryReviewNotFound   = errors.New("inventory review not found or already resolved")
	// ErrVariantNotTracked is returned by ReceiveStock when the variant is
	// marked as a non-stocked service item (tracks_inventory = false, see
	// catalogue.Variant's own doc comment) -- there is no such thing as
	// "restocking" a snooker game.
	ErrVariantNotTracked = errors.New("this item does not track inventory and cannot be restocked")
	ErrReceiptNotFound   = errors.New("stock receipt not found")
	// ErrReceiptAlreadyReversed mirrors sales.ErrAlreadyReversed -- at most
	// one reversal per receipt, checked query-time (no DB-level unique),
	// same tolerance ReverseSale already accepts.
	ErrReceiptAlreadyReversed = errors.New("this stock receipt has already been reversed")
	// ErrReceiptPartiallyConsumed is returned when some of the receipt's
	// stock has already been sold (a lot's remaining_quantity is less than
	// what it received) -- undoing the receipt would either understate
	// what was genuinely sold or drive the balance negative. The owner
	// should use a manual inventory adjustment instead, which corrects a
	// quantity without rewriting receiving history.
	ErrReceiptPartiallyConsumed = errors.New("some of this receipt's stock has already been sold; use a manual adjustment instead")
	// ErrReceiptFullyConsumed is returned by AdminForceReverseStockReceipt
	// when every line's lot has nothing left to give back -- there would be
	// nothing for the override to actually do. ReverseStockReceipt never
	// returns this; it returns ErrReceiptPartiallyConsumed instead, which
	// covers this case too (it doesn't distinguish "some" from "all"
	// consumed since neither is safe for the ordinary, non-override path).
	ErrReceiptFullyConsumed = errors.New("this receipt's stock has been fully consumed; there is nothing left to give back")
	// ErrBalanceAlreadyCorrect is returned by AdminCorrectBalance when the
	// requested target already equals the live balance -- there is no
	// delta to post, and posting a zero-quantity adjustment would violate
	// inventory_adjustment_requests' own CHECK (quantity_delta <> 0).
	ErrBalanceAlreadyCorrect = errors.New("this variant's balance already matches the requested value")
)

// pgErrorCode reports err's Postgres SQLSTATE code, if any. Duplicated
// rather than shared across feature packages, per docs/ARCHITECTURE.md's
// "each feature package stays self-contained" (see internal/catalogue's
// identical helper).
func pgErrorCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

const (
	pgForeignKeyViolation = "23503"
	pgUniqueViolation     = "23505"
)
