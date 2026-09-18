package signup

import "errors"

var (
	// ErrAlreadyRegistered is returned by Start (the identifier already
	// belongs to a real account -- signup deliberately discloses this,
	// unlike login's "one generic invalid-credentials response") and by
	// Verify (a race: the identifier became a real account between Start
	// and Verify, e.g. an operator ran cmd/seed for it in the meantime).
	// Covers both email and phone, whichever channel was used.
	ErrAlreadyRegistered    = errors.New("this email or phone is already registered")
	ErrNoPendingSignup      = errors.New("no pending signup found for this identifier")
	ErrInvalidOrExpiredCode = errors.New("invalid or expired verification code")
	ErrUnsupportedChannel   = errors.New("channel must be email or whatsapp")
)
