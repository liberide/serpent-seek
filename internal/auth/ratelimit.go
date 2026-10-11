package auth

import (
	"sync"
	"time"
)

// RateLimiter is a fixed-window, per-key in-memory limiter.
type RateLimiter struct {
	mu        sync.Mutex
	hits      map[string][]time.Time
	limit     int
	window    time.Duration
	lastSweep time.Time
}

// NewRateLimiter builds a limiter allowing `limit` events per window.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{hits: map[string][]time.Time{}, limit: limit, window: window, lastSweep: time.Now()}
}

// Allow reports whether an event for key is permitted right now.
func (l *RateLimiter) Allow(key string) bool {
	return l.AllowLimit(key, l.limit, l.window)
}

// AllowLimit is Allow with a per-call limit and window. A limit <= 0 disables
// the check. It lets callers read the limit from settings on every request and
// keeps the limiter reusable for several endpoints.
func (l *RateLimiter) AllowLimit(key string, limit int, window time.Duration) bool {
	if limit <= 0 {
		return true
	}
	if window <= 0 {
		window = time.Minute
	}
	now := time.Now()
	cutoff := now.Add(-window)
	l.mu.Lock()
	defer l.mu.Unlock()
	// Drop expired buckets periodically.
	if now.Sub(l.lastSweep) >= window {
		l.sweep(cutoff)
		l.lastSweep = now
	}
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}

// sweep drops entries older than cutoff. Callers hold l.mu.
func (l *RateLimiter) sweep(cutoff time.Time) {
	for k, times := range l.hits {
		kept := times[:0]
		for _, t := range times {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(l.hits, k)
		} else {
			l.hits[k] = kept
		}
	}
}
