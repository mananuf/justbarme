package invitations

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrInvitationNotFound = errors.New("invitation not found")
	ErrInvitationExpired  = errors.New("this invitation has expired")
	ErrInvitationNotOpen  = errors.New("this invitation is no longer open")
	ErrAlreadyPending     = errors.New("an invitation to this phone or email is already pending for this business")
)

// pgErrorCode reports err's Postgres SQLSTATE code, if any. Duplicated
// rather than shared across feature packages, per this codebase's
// established "each feature package stays self-contained" convention.
func pgErrorCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

const pgUniqueViolation = "23505"
