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
	// HTML is the optional HTML alternative for this message ("" if this
	// email has no HTML variant -- see NewHTMLTemplate). A Provider that
	// supports it (SMTPProvider) sends a multipart/alternative message
	// with Text as the fallback part; Text is always required regardless,
	// since some clients and most spam filters weigh it.
	HTML string
}

// Provider sends one transactional email. Implementations must not log the
// message body (it may carry an OTP code) -- only that a send was
// attempted and its outcome.
type Provider interface {
	Send(ctx context.Context, msg Message) error
}
