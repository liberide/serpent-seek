package metrics

import (
	"strings"
	"testing"
)

func TestRenderExposition(t *testing.T) {
	m := New()
	m.RequestStarted()
	m.RecordSearch("ok", 1500)
	m.RecordSearch("empty", 500)
	m.RecordProvider("brave", "ok", 100)
	m.RecordProvider(`a"b\c`, "fail", 2000)

	var b strings.Builder
	if err := m.Render(&b, 3); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := b.String()

	for _, want := range []string{
		"# TYPE serpentseek_build_info gauge",
		"# TYPE serpentseek_active_requests gauge",
		"serpentseek_active_requests 1",
		"serpentseek_sse_clients 3",
		`serpentseek_search_requests_total{status="ok"} 1`,
		`serpentseek_search_requests_total{status="empty"} 1`,
		`serpentseek_search_requests_total{status="fail"} 0`,
		"serpentseek_search_duration_seconds_sum 2.000000",
		"serpentseek_search_duration_seconds_count 2",
		`serpentseek_provider_requests_total{provider="brave",status="ok"} 1`,
		`serpentseek_provider_requests_total{provider="a\"b\\c",status="fail"} 1`,
		"serpentseek_provider_duration_seconds_count{provider=\"brave\"} 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	m.RequestEnded()
	if got := m.ActiveRequests(); got != 0 {
		t.Fatalf("ActiveRequests = %d, want 0", got)
	}
}

func TestEmptyRenderHasCoreSeries(t *testing.T) {
	var b strings.Builder
	if err := New().Render(&b, 0); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "serpentseek_search_requests_total{status=\"ok\"} 0") {
		t.Fatalf("fixed status series missing:\n%s", out)
	}
	if strings.Contains(out, "serpentseek_provider_requests_total{") {
		t.Fatalf("no providers recorded, but provider series present:\n%s", out)
	}
}
