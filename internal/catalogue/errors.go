package catalogue

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrTemplateNotFound  = errors.New("catalogue template not found")
	ErrCategoryNotFound  = errors.New("category not found")
	ErrCategoryNameTaken = errors.New("a category with this name already exists")
	ErrProductNotFound   = errors.New("product not found")
	ErrProductNameTaken  = errors.New("a product with this name already exists")
	ErrVariantNotFound   = errors.New("product variant not found")
	ErrVariantNameTaken  = errors.New("a variant with this name already exists for this product")
	ErrNoCurrentPrice    = errors.New("variant has no current price")
)

// pgErrorCode reports err's Postgres SQLSTATE code, if any. Duplicated from
// internal/identity rather than shared, per docs/ARCHITECTURE.md's
// "do not force every table behind one generic repository" -- each feature
// package stays self-contained.
func pgErrorCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

const pgUniqueViolation = "23505"
