package inventory

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNoLines         = errors.New("a stock receipt must include at least one line")
	ErrVariantNotFound = errors.New("product variant not found")
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

const pgForeignKeyViolation = "23503"
