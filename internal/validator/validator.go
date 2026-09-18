package validator

import (
	"regexp"
	"slices"
)

type Validator struct {
	Errors map[string]string
}

func New() *Validator {
	return &Validator{Errors: make(map[string]string)}
}

func (v *Validator) IsValid() bool {
	return len(v.Errors) == 0
}

func (v *Validator) AddError(key, message string) {
	if _, exists := v.Errors[key]; !exists {
		v.Errors[key] = message
	}
}

func (v *Validator) Check(ok bool, key, message string) {
	if !ok {
		v.AddError(key, message)
	}
}

func PermittedValue[T comparable](value T, permittedValues ...T) bool {
	return slices.Contains(permittedValues, value)
}

// e164Pattern matches a leading '+' followed by 7-14 digits, the first of
// which is non-zero -- the shape ITU-T E.164 and (per its own error
// message) Zavu's WhatsApp/SMS channels both require. No spaces, dashes,
// or a leading local-dialing '0': callers are expected to normalize to
// this shape before validating (see web/src/components/PhoneInput.tsx's
// dial-code + local-number composition on the frontend).
var e164Pattern = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

// IsE164Phone reports whether value is a plausible E.164 phone number.
// Every request path that accepts a phone number destined for
// internal/whatsapp (signup, invitations, identity verification) should
// check this at the boundary -- rejecting a malformed number here as a
// normal 422 is far better than discovering it as a raw 500 once Zavu
// itself rejects the send.
func IsE164Phone(value string) bool {
	return e164Pattern.MatchString(value)
}
