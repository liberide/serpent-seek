package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestEmitPostsPayload(t *testing.T) {
	var got atomic.Int32
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Add(1)
		body, _ = io.ReadAll(r.Body)
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content type %q", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	Emit(context.Background(), srv.URL, "quota_exceeded", map[string]any{"target": "key:1", "quota": 10}, nil)
	if got.Load() != 1 {
		t.Fatalf("expected one delivery, got %d", got.Load())
	}
	var payload struct {
		Event string         `json:"event"`
		TS    string         `json:"ts"`
		Data  map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode payload: %v (%s)", err, body)
	}
	if payload.Event != "quota_exceeded" || payload.Data["target"] != "key:1" || payload.TS == "" {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	// A repeat for the same url/event/target is throttled.
	Emit(context.Background(), srv.URL, "quota_exceeded", map[string]any{"target": "key:1"}, nil)
	if got.Load() != 1 {
		t.Fatalf("repeat should be throttled, got %d", got.Load())
	}
}

func TestEmitDisabledAndFailure(t *testing.T) {
	// An empty URL is a no-op.
	Emit(context.Background(), "  ", "provider_cooldown", map[string]any{"target": "p1"}, nil)

	// A non-2xx response must not panic and must not retry.
	var got atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		got.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	Emit(context.Background(), srv.URL, "provider_cooldown", map[string]any{"target": "p2"}, nil)
	if got.Load() != 1 {
		t.Fatalf("expected one delivery attempt, got %d", got.Load())
	}
	// The failure itself must not have leaked into the throttle map in a way
	// that blocks a later distinct event.
	Emit(context.Background(), srv.URL, "provider_cooldown", map[string]any{"target": "p3"}, nil)
	if got.Load() != 2 {
		t.Fatalf("expected a second delivery for a new target, got %d", got.Load())
	}
}
