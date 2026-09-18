package whatsapp_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/mananuf/justbarme/internal/whatsapp"
)

func TestConsoleProviderNeverFailsAndReturnsAnID(t *testing.T) {
	p := whatsapp.NewConsoleProvider(slog.Default())
	id, err := p.Send(context.Background(), whatsapp.Message{To: "+2348012345678", Text: "test"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id == "" {
		t.Fatal("expected a non-empty message ID")
	}
}
