package signup

import "errors"

var (
	// ErrEmailAlreadyRegistered is returned by Start (the email already
	// belongs to a real account -- signup deliberately discloses this,
	// unlike login's "one generic invalid-credentials response") and by
	// Verify (a race: the email became a real account between Start and
	// Verify, e.g. an operator ran cmd/seed for it in the meantime).
	ErrEmailAlreadyRegistered = errors.New("this email is already registered")
	ErrNoPendingSignup        = errors.New("no pending signup found for this email")
	ErrInvalidOrExpiredCode   = errors.New("invalid or expired verification code")
)
