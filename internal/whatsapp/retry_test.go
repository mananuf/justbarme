package whatsapp_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mananuf/justbarme/internal/whatsapp"
)

type countingProvider struct {
	failures int32
	calls    int32
	err      error
}

func (p *countingProvider) Send(context.Context, whatsapp.Message) (string, error) {
	n := atomic.AddInt32(&p.calls, 1)
	if n <= p.failures {
		return "", p.err
	}
	return "msg-id", nil
}

func fastRetryConfig() whatsapp.RetryConfig {
	return whatsapp.RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}
}

func TestWithRetrySucceedsAfterTransientFailures(t *testing.T) {
	inner := &countingProvider{failures: 2, err: errors.New("relay unavailable")}
	provider := whatsapp.WithRetry(inner, fastRetryConfig())

	id, err := provider.Send(context.Background(), whatsapp.Message{To: "+2348012345678"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id != "msg-id" {
		t.Fatalf("expected the successful attempt's message ID, got %q", id)
	}
	if inner.calls != 3 {
		t.Fatalf("expected 3 attempts (2 failures then a success), got %d", inner.calls)
	}
}

func TestWithRetryGivesUpAfterMaxAttempts(t *testing.T) {
	wantErr := errors.New("relay permanently down")
	inner := &countingProvider{failures: 100, err: wantErr}
	provider := whatsapp.WithRetry(inner, fastRetryConfig())

	_, err := provider.Send(context.Background(), whatsapp.Message{To: "+2348012345678"})
	if err == nil {
		t.Fatal("expected an error after exhausting retries, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the final error to wrap %v, got %v", wantErr, err)
	}
	if inner.calls != 3 {
		t.Fatalf("expected exactly 3 attempts, got %d", inner.calls)
	}
}

func TestWithRetrySucceedsFirstTryDoesNotSleep(t *testing.T) {
	inner := &countingProvider{failures: 0}
	provider := whatsapp.WithRetry(inner, whatsapp.RetryConfig{MaxAttempts: 3, BaseDelay: time.Hour})

	if _, err := provider.Send(context.Background(), whatsapp.Message{To: "+2348012345678"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("expected exactly 1 attempt, got %d", inner.calls)
	}
}

func TestWithRetryStopsWhenContextIsCancelled(t *testing.T) {
	inner := &countingProvider{failures: 100, err: errors.New("relay unavailable")}
	provider := whatsapp.WithRetry(inner, whatsapp.RetryConfig{MaxAttempts: 5, BaseDelay: time.Hour})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	_, err := provider.Send(ctx, whatsapp.Message{To: "+2348012345678"})
	if err == nil {
		t.Fatal("expected an error once the context was cancelled mid-backoff, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the error to wrap context.Canceled, got %v", err)
	}
	if inner.calls >= 5 {
		t.Fatalf("expected cancellation to cut retries short, got all %d attempts", inner.calls)
	}
}

func TestWithRetryDefaultsInvalidConfig(t *testing.T) {
	inner := &countingProvider{failures: 100, err: errors.New("down")}
	provider := whatsapp.WithRetry(inner, whatsapp.RetryConfig{})

	_, err := provider.Send(context.Background(), whatsapp.Message{To: "+2348012345678"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if inner.calls != int32(whatsapp.DefaultRetryConfig.MaxAttempts) {
		t.Fatalf("expected %d attempts (DefaultRetryConfig.MaxAttempts), got %d", whatsapp.DefaultRetryConfig.MaxAttempts, inner.calls)
	}
}
