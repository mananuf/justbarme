package email_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mananuf/justbarme/internal/email"
)

type countingProvider struct {
	failures int32
	calls    int32
	err      error
}

func (p *countingProvider) Send(context.Context, email.Message) error {
	n := atomic.AddInt32(&p.calls, 1)
	if n <= p.failures {
		return p.err
	}
	return nil
}

func fastRetryConfig() email.RetryConfig {
	// Real durations would make this test slow for no benefit -- what's
	// under test is attempt counting and give-up behavior, not timing.
	return email.RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}
}

func TestWithRetrySucceedsAfterTransientFailures(t *testing.T) {
	inner := &countingProvider{failures: 2, err: errors.New("relay unavailable")}
	provider := email.WithRetry(inner, fastRetryConfig())

	if err := provider.Send(context.Background(), email.Message{To: "a@example.com"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if inner.calls != 3 {
		t.Fatalf("expected 3 attempts (2 failures then a success), got %d", inner.calls)
	}
}

func TestWithRetryGivesUpAfterMaxAttempts(t *testing.T) {
	wantErr := errors.New("relay permanently down")
	inner := &countingProvider{failures: 100, err: wantErr}
	provider := email.WithRetry(inner, fastRetryConfig())

	err := provider.Send(context.Background(), email.Message{To: "a@example.com"})
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
	// A deliberately large BaseDelay: if this provider slept even once,
	// the test would time out under `go test`'s default deadline.
	provider := email.WithRetry(inner, email.RetryConfig{MaxAttempts: 3, BaseDelay: time.Hour})

	if err := provider.Send(context.Background(), email.Message{To: "a@example.com"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("expected exactly 1 attempt, got %d", inner.calls)
	}
}

func TestWithRetryStopsWhenContextIsCancelled(t *testing.T) {
	inner := &countingProvider{failures: 100, err: errors.New("relay unavailable")}
	provider := email.WithRetry(inner, email.RetryConfig{MaxAttempts: 5, BaseDelay: time.Hour})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	err := provider.Send(ctx, email.Message{To: "a@example.com"})
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
	// MaxAttempts <= 0 and BaseDelay <= 0 should fall back to
	// DefaultRetryConfig rather than retrying forever or busy-looping.
	provider := email.WithRetry(inner, email.RetryConfig{})

	err := provider.Send(context.Background(), email.Message{To: "a@example.com"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if inner.calls != int32(email.DefaultRetryConfig.MaxAttempts) {
		t.Fatalf("expected %d attempts (DefaultRetryConfig.MaxAttempts), got %d", email.DefaultRetryConfig.MaxAttempts, inner.calls)
	}
}
