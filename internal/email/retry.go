package email

import (
	"context"
	"fmt"
	"time"
)

// RetryConfig bounds how many times a Provider.Send is attempted and how
// long to wait between attempts. Exported so any caller assembling a
// Provider (internal/app today, tests, or a future provider) can reuse the
// same backoff without reimplementing it per provider.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
}

// DefaultRetryConfig is 3 attempts total (the original send plus up to 2
// retries) with backoff doubling from 500ms -- generous enough to ride out
// a relay's transient hiccup (a dropped connection, a momentary 4xx from
// the relay), short enough not to block a signup request for long.
var DefaultRetryConfig = RetryConfig{MaxAttempts: 3, BaseDelay: 500 * time.Millisecond}

// retryingProvider wraps another Provider and is itself a Provider -- it
// composes with any implementation (SMTPProvider today, whatever sends
// email next) without either side knowing the other exists.
type retryingProvider struct {
	inner Provider
	cfg   RetryConfig
	sleep func(ctx context.Context, d time.Duration) error
}

// WithRetry wraps inner so a failed Send is retried, with exponential
// backoff, up to cfg.MaxAttempts total attempts (not cfg.MaxAttempts
// *additional* retries) before giving up. A zero RetryConfig falls back to
// DefaultRetryConfig field-by-field, so callers can override just
// MaxAttempts or just BaseDelay.
func WithRetry(inner Provider, cfg RetryConfig) Provider {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = DefaultRetryConfig.MaxAttempts
	}
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = DefaultRetryConfig.BaseDelay
	}
	return &retryingProvider{inner: inner, cfg: cfg, sleep: ctxSleep}
}

func ctxSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (p *retryingProvider) Send(ctx context.Context, msg Message) error {
	var lastErr error
	for attempt := 1; attempt <= p.cfg.MaxAttempts; attempt++ {
		lastErr = p.inner.Send(ctx, msg)
		if lastErr == nil {
			return nil
		}
		if attempt == p.cfg.MaxAttempts {
			break
		}
		// Exponential backoff: BaseDelay, 2*BaseDelay, 4*BaseDelay, ...
		delay := p.cfg.BaseDelay * time.Duration(int64(1)<<uint(attempt-1))
		if err := p.sleep(ctx, delay); err != nil {
			return fmt.Errorf("send email: attempt %d/%d failed (%w); then cancelled before retrying: %w",
				attempt, p.cfg.MaxAttempts, lastErr, err)
		}
	}
	return fmt.Errorf("send email: giving up after %d attempts: %w", p.cfg.MaxAttempts, lastErr)
}
