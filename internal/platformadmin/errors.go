package platformadmin

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrEmailTaken         = errors.New("email is already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrStaffNotFound      = errors.New("platform staff not found")
	ErrSessionNotFound    = errors.New("session not found or expired")
	ErrBusinessNotFound   = errors.New("business not found")
)

// pgErrorCode reports err's Postgres SQLSTATE code, if any. Duplicated from
// internal/identity/internal/catalogue rather than shared -- each feature
// package stays self-contained, per docs/ARCHITECTURE.md.
func pgErrorCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

const pgUniqueViolation = "23505"
