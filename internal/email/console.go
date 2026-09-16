package email

import (
	"context"
	"log/slog"
)

// ConsoleProvider logs that a send happened instead of actually sending
// anything -- for local development only, when SMTP isn't configured.
// config.Load refuses to start in production without real SMTP
// credentials, mirroring how internal/app.ensureOfflineSigningKeys treats
// the offline-lease signing keys: a convenient, clearly-logged fallback
// outside production, never a silent one.
type ConsoleProvider struct {
	logger *slog.Logger
}

func NewConsoleProvider(logger *slog.Logger) *ConsoleProvider {
	return &ConsoleProvider{logger: logger}
}

func (p *ConsoleProvider) Send(_ context.Context, msg Message) error {
	p.logger.Warn("email not actually sent (no SMTP configured, development only)",
		"to", msg.To, "subject", msg.Subject, "body", msg.Text)
	return nil
}
