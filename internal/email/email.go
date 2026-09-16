// Package email is the transactional-email boundary docs/ARCHITECTURE.md
// anticipates ("API --> Email[Email provider]") but never built. It exists
// now specifically for signup OTP delivery (internal/signup), kept behind
// an interface per docs/IMPLEMENTATION_PLAN.md ("keep provider behind an
// interface") so the concrete provider can change without touching any
// caller.
package email

import "context"

type Message struct {
	To      string
	Subject string
	Text    string
}

// Provider sends one transactional email. Implementations must not log the
// message body (it may carry an OTP code) -- only that a send was
// attempted and its outcome.
type Provider interface {
	Send(ctx context.Context, msg Message) error
}
