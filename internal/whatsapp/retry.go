package whatsapp

import (
	"context"
	"fmt"
	"time"
)

// RetryConfig bounds how many times a Provider.Send is attempted and how
// long to wait between attempts -- same shape as internal/email.RetryConfig,
// kept as its own type (not shared) so the two channels can diverge later
// without one accidentally changing the other.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
}

// DefaultRetryConfig mirrors internal/email's: 3 attempts total, backoff
// doubling from 500ms.
var DefaultRetryConfig = RetryConfig{MaxAttempts: 3, BaseDelay: 500 * time.Millisecond}

type retryingProvider struct {
	inner Provider
	cfg   RetryConfig
	sleep func(ctx context.Context, d time.Duration) error
}

// WithRetry wraps inner so a failed Send is retried, with exponential
// backoff, up to cfg.MaxAttempts total attempts before giving up. A zero
// RetryConfig falls back to DefaultRetryConfig field-by-field.
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

func (p *retryingProvider) Send(ctx context.Context, msg Message) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= p.cfg.MaxAttempts; attempt++ {
		messageID, err := p.inner.Send(ctx, msg)
		if err == nil {
			return messageID, nil
		}
		lastErr = err
		if attempt == p.cfg.MaxAttempts {
			break
		}
		delay := p.cfg.BaseDelay * time.Duration(int64(1)<<uint(attempt-1))
		if err := p.sleep(ctx, delay); err != nil {
			return "", fmt.Errorf("send whatsapp message: attempt %d/%d failed (%w); then cancelled before retrying: %w",
				attempt, p.cfg.MaxAttempts, lastErr, err)
		}
	}
	return "", fmt.Errorf("send whatsapp message: giving up after %d attempts: %w", p.cfg.MaxAttempts, lastErr)
}
