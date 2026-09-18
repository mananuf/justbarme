package verification

import "errors"

var (
	ErrUnsupportedChannel     = errors.New("channel must be email or whatsapp")
	ErrIdentifierAlreadyInUse = errors.New("this email or phone already belongs to a different account")
	ErrVerificationNotFound   = errors.New("verification not found or already confirmed")
	ErrInvalidOrExpiredCode   = errors.New("invalid or expired verification code")
)
