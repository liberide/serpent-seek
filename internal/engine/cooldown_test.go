package engine

import (
	"testing"
	"time"
)

func TestProviderCooldownsTripAndRecover(t *testing.T) {
	c := newProviderCooldowns()
	now := time.Now()

	if c.blocked("p", now) {
		t.Fatal("a fresh provider must not be blocked")
	}
	c.observe("p", false, 2, time.Minute, now)
	if c.blocked("p", now) {
		t.Fatal("one failure below the threshold must not trip")
	}
	c.observe("p", false, 2, time.Minute, now)
	if !c.blocked("p", now.Add(time.Second)) {
		t.Fatal("reaching the threshold must trip the breaker")
	}
	if c.active(now.Add(time.Second)) != 1 {
		t.Fatal("tripped provider must be reported active")
	}
	if c.blocked("p", now.Add(2*time.Minute)) {
		t.Fatal("cooldown must expire")
	}
	if c.active(now.Add(2*time.Minute)) != 0 {
		t.Fatal("expired provider must not stay active")
	}

	// A success clears the failure counter.
	c.observe("p", false, 2, time.Minute, now)
	c.observe("p", true, 2, time.Minute, now)
	c.observe("p", false, 2, time.Minute, now)
	if c.blocked("p", now) {
		t.Fatal("a success must reset the failure counter")
	}

	// A disabled threshold never trips.
	c.observe("p", false, 0, time.Minute, now)
	if c.blocked("p", now) {
		t.Fatal("threshold 0 must disable the breaker")
	}
}
