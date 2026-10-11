package auth

import (
	"testing"
	"time"
)

func TestRateLimiterAllowLimit(t *testing.T) {
	l := NewRateLimiter(0, time.Minute)

	if !l.AllowLimit("a", 0, time.Minute) {
		t.Fatal("limit 0 must disable the check")
	}
	for i := 0; i < 2; i++ {
		if !l.AllowLimit("a", 2, time.Minute) {
			t.Fatalf("event %d must be allowed", i)
		}
	}
	if l.AllowLimit("a", 2, time.Minute) {
		t.Fatal("third event within the window must be denied")
	}
	// Other keys have their own bucket.
	if !l.AllowLimit("b", 2, time.Minute) {
		t.Fatal("a different key must not be affected")
	}
	// A tiny window lets the bucket drain.
	if !l.AllowLimit("a", 2, time.Nanosecond) {
		t.Fatal("expired events must not count")
	}
}

func TestRateLimiterAllowUsesConfiguredLimit(t *testing.T) {
	l := NewRateLimiter(1, time.Minute)
	if !l.Allow("k") {
		t.Fatal("first event must pass")
	}
	if l.Allow("k") {
		t.Fatal("second event must be denied by the configured limit")
	}
}
