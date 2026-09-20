package expenses

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrCategoryNameTaken = errors.New("a category with this name already exists")
	ErrExpenseNotFound   = errors.New("expense not found")
	ErrAlreadyReversed   = errors.New("this expense has already been reversed")
)

// pgErrorCode reports err's Postgres SQLSTATE code, if any. Duplicated
// rather than shared across feature packages, per docs/ARCHITECTURE.md's
// "each feature package stays self-contained" (see internal/catalogue,
// internal/inventory, and internal/sales' identical helpers).
func pgErrorCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

const (
	pgUniqueViolation = "23505"
)
