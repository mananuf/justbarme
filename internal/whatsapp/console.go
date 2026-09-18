package whatsapp

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// ConsoleProvider logs that a send happened instead of actually sending
// anything -- for local development only, when Zavu isn't configured.
// config.Load refuses to start in production without real Zavu
// credentials, the same enforcement email.ConsoleProvider's SMTP
// counterpart already has: a convenient, clearly-logged fallback outside
// production, never a silent one. Deliberately not Zavu's own sandbox mode
// (its test key still sends real messages, just restricted to
// team-registered numbers) -- logging is strictly better for local dev.
type ConsoleProvider struct {
	logger *slog.Logger
}

func NewConsoleProvider(logger *slog.Logger) *ConsoleProvider {
	return &ConsoleProvider{logger: logger}
}

func (p *ConsoleProvider) Send(_ context.Context, msg Message) (string, error) {
	id := "console-" + uuid.NewString()
	p.logger.Warn("whatsapp message not actually sent (no Zavu credentials configured, development only)",
		"to", msg.To, "text", msg.Text, "message_id", id)
	return id, nil
}
