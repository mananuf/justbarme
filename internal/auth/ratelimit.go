package auth

import (
	"sync"
	"time"
)

// Limiter is a per-key fixed-window counter. It is explicitly a
// process-local, single-instance mitigation — docs/IMPLEMENTATION_PLAN.md
// calls this acceptable only for a single-instance pilot deployment, not a
// substitute for distributed rate limiting once more than one application
// instance is running.
type Limiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	visitors map[string]*visitor
}

type visitor struct {
	count     int
	windowEnd time.Time
}

// evictThreshold bounds how large visitors can grow before a stale-entry
// sweep runs, so an attacker cycling through keys cannot exhaust memory.
const evictThreshold = 10_000

func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, visitors: make(map[string]*visitor)}
}

// Allow increments key's counter for the current window and reports whether
// it is still within limit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	v, ok := l.visitors[key]
	if !ok || now.After(v.windowEnd) {
		v = &visitor{windowEnd: now.Add(l.window)}
		l.visitors[key] = v
	}
	v.count++

	if len(l.visitors) > evictThreshold {
		l.evictExpiredLocked(now)
	}
	return v.count <= l.limit
}

func (l *Limiter) evictExpiredLocked(now time.Time) {
	for key, v := range l.visitors {
		if now.After(v.windowEnd) {
			delete(l.visitors, key)
		}
	}
}
