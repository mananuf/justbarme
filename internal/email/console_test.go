package email_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/mananuf/justbarme/internal/email"
)

func TestConsoleProviderLogsWithoutErroring(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	provider := email.NewConsoleProvider(logger)

	if err := provider.Send(context.Background(), email.Message{
		To: "owner@example.com", Subject: "Your code", Text: "Your code is 123456.",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	logged := buf.String()
	if !strings.Contains(logged, "owner@example.com") || !strings.Contains(logged, "123456") {
		t.Fatalf("expected the log line to include the recipient and message body, got: %s", logged)
	}
}

func TestConsoleProviderDiscardsSilently(t *testing.T) {
	provider := email.NewConsoleProvider(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := provider.Send(context.Background(), email.Message{To: "a@example.com", Subject: "s", Text: "t"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
}
