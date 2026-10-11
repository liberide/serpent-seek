package httpd

import (
	"net/http"
	"strings"
	"testing"
)

func TestMetricsEndpoint(t *testing.T) {
	ts := newTestServer(t, true)
	if rec := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "metrics", "count": 1}, true); rec.Code != http.StatusOK {
		t.Fatalf("search status %d: %s", rec.Code, rec.Body.String())
	}

	// The endpoint is public (no key) and returns the text exposition format.
	rec := ts.do(t, http.MethodGet, "/metrics", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("unexpected content type %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"# TYPE serpentseek_build_info gauge",
		"serpentseek_active_requests 0",
		`serpentseek_search_requests_total{status="ok"} 1`,
		`serpentseek_provider_requests_total{provider="Stub",status="ok"} 1`,
		"serpentseek_search_duration_seconds_count 1",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q:\n%s", want, body)
		}
	}
}
