package sse

import (
	"testing"
	"time"
)

func TestHubPublishReplayAndClients(t *testing.T) {
	h := NewHub()
	ch, past, unsub := h.Subscribe("r1", 0)
	defer unsub()
	if len(past) != 0 {
		t.Fatalf("fresh subscriber got replay: %+v", past)
	}
	if h.Clients() != 1 {
		t.Fatalf("Clients = %d, want 1", h.Clients())
	}

	h.Publish("r1", "step", map[string]int{"n": 1})
	select {
	case ev := <-ch:
		if ev.Type != "step" || ev.ID != 1 || string(ev.Data) != `{"n":1}` {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive the event")
	}

	// A late subscriber replays the buffered event.
	ch2, past2, unsub2 := h.Subscribe("r1", 0)
	defer unsub2()
	if len(past2) != 1 || past2[0].Type != "step" {
		t.Fatalf("late subscriber replay: %+v", past2)
	}
	_ = ch2

	// Events for another key are not delivered to r1.
	h.Publish("other", "step", nil)
	select {
	case ev := <-ch:
		t.Fatalf("cross-key delivery: %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHubCloseUnblocks(t *testing.T) {
	h := NewHub()
	ch, _, _ := h.Subscribe("r", 0)

	h.Close()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("Close must close subscriber channels")
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not unblock the subscriber")
	}
	if h.Clients() != 0 {
		t.Fatalf("Clients after Close = %d, want 0", h.Clients())
	}
	// Publishing after Close is a no-op and must not panic.
	h.Publish("r", "step", nil)
}
