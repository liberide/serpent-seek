package httpd

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/liberide/serpent-seek/internal/config"
)

func TestSearchGuardDailyQuota(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()
	if err := ts.server.settings.Set(ctx, config.KeySearchDailyQuota, "1"); err != nil {
		t.Fatalf("set quota: %v", err)
	}

	first := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "one", "count": 1}, true)
	if first.Code != http.StatusOK {
		t.Fatalf("first search status %d: %s", first.Code, first.Body.String())
	}
	if got := first.Header().Get("X-Serpent-Quota-Limit"); got != "1" {
		t.Fatalf("quota limit header = %q, want 1", got)
	}
	if got := first.Header().Get("X-Serpent-Quota-Remaining"); got != "0" {
		t.Fatalf("quota remaining header = %q, want 0", got)
	}

	second := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "two", "count": 1}, true)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second search status %d, want 429: %s", second.Code, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), "quota_exceeded") {
		t.Fatalf("expected quota_exceeded, got %s", second.Body.String())
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("quota rejection must set Retry-After")
	}
}

func TestSearchGuardRateLimit(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()
	if err := ts.server.settings.Set(ctx, config.KeySearchRateLimit, "2"); err != nil {
		t.Fatalf("set rate limit: %v", err)
	}

	for i := 0; i < 2; i++ {
		if rec := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "ok", "count": 1}, true); rec.Code != http.StatusOK {
			t.Fatalf("request %d status %d: %s", i, rec.Code, rec.Body.String())
		}
	}
	third := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "blocked", "count": 1}, true)
	if third.Code != http.StatusTooManyRequests {
		t.Fatalf("third search status %d, want 429: %s", third.Code, third.Body.String())
	}
	if !strings.Contains(third.Body.String(), "rate_limited") {
		t.Fatalf("expected rate_limited, got %s", third.Body.String())
	}
}

func TestSearchGuardAppliesToMCP(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()
	if err := ts.server.settings.Set(ctx, config.KeySearchDailyQuota, "1"); err != nil {
		t.Fatalf("set quota: %v", err)
	}

	call := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "search", "arguments": map[string]any{"query": "one", "count": 1}},
	}
	if rec := ts.do(t, http.MethodPost, "/mcp", call, true); rec.Code != http.StatusOK {
		t.Fatalf("first mcp status %d: %s", rec.Code, rec.Body.String())
	}
	if rec := ts.do(t, http.MethodPost, "/mcp", call, true); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second mcp status %d, want 429: %s", rec.Code, rec.Body.String())
	}
}
