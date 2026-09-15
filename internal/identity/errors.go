package identity

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrEmailTaken           = errors.New("email is already registered")
	ErrInvalidCredentials   = errors.New("invalid email or password")
	ErrUserNotFound         = errors.New("user not found")
	ErrMembershipNotFound   = errors.New("membership not found")
	ErrMembershipNotActive  = errors.New("membership is not active")
	ErrBusinessNotFound     = errors.New("business not found")
	ErrSessionNotFound      = errors.New("session not found or expired")
	ErrDeviceNotFound       = errors.New("device not found")
	ErrDevicePublicKeyTaken = errors.New("device public key already enrolled for this business")
)

// pgErrorCode reports err's Postgres SQLSTATE code, if any.
func pgErrorCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

const pgUniqueViolation = "23505"
