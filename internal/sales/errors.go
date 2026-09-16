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
