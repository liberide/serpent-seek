package engine

import (
	"sync"
	"time"
)

// providerCooldowns is an in-memory circuit breaker keyed by provider instance
// id. After `threshold` consecutive failed attempts an instance is skipped for
// `cooldown` without waiting for its timeout; a success clears the counter.
// threshold <= 0 disables it.
type providerCooldowns struct {
	mu    sync.Mutex
	fails map[string]int
	until map[string]time.Time
}

func newProviderCooldowns() *providerCooldowns {
	return &providerCooldowns{fails: map[string]int{}, until: map[string]time.Time{}}
}

// blocked reports whether the provider is currently in cooldown.
func (c *providerCooldowns) blocked(key string, now time.Time) bool {
	if c == nil || key == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	until, ok := c.until[key]
	if !ok {
		return false
	}
	if !now.Before(until) {
		delete(c.until, key)
		delete(c.fails, key)
		return false
	}
	return true
}

// observe records one provider attempt outcome and reports whether this failure
// tripped the breaker.
func (c *providerCooldowns) observe(key string, ok bool, threshold int, cooldown time.Duration, now time.Time) bool {
	if c == nil || key == "" || threshold <= 0 {
		return false
	}
	if cooldown <= 0 {
		cooldown = time.Minute
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ok {
		delete(c.fails, key)
		delete(c.until, key)
		return false
	}
	c.fails[key]++
	if c.fails[key] >= threshold {
		c.until[key] = now.Add(cooldown)
		delete(c.fails, key)
		return true
	}
	return false
}

// active reports how many providers are currently in cooldown.
func (c *providerCooldowns) active(now time.Time) int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for key, until := range c.until {
		if now.Before(until) {
			n++
			continue
		}
		delete(c.until, key)
		delete(c.fails, key)
	}
	return n
}
