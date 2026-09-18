// Package whatsapp is the WhatsApp messaging boundary
// docs/PHASE_INVITATIONS_WHATSAPP.md introduces, kept behind an interface
// the same way internal/email keeps SMTP behind one -- Zavu is one
// Provider-shaped dependency, not a special case, and a future vendor
// swap (or a second provider for a different market) needs no caller
// changes.
package whatsapp

import "context"

type Message struct {
	To   string // E.164 phone number
	Text string
}

// Provider sends one WhatsApp message. Implementations must not log Text
// (may carry an OTP code or invite link) -- only that a send was
// attempted and its outcome. messageID is returned so a caller can later
// correlate a delivery-status webhook event back to this send.
type Provider interface {
	Send(ctx context.Context, msg Message) (messageID string, err error)
}
