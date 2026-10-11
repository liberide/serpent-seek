package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.1.0", "1.1.0", 0},
		{"v1.1.0", "1.1.1", -1},
		{"1.2.0", "1.1.9", 1},
		{"1.1.0", "1.1.0-beta", 1},
		{"1.1.0-beta", "1.1.0", -1},
		{"1.1", "1.1.0", 0},
		{"1.1.0+build", "1.1.0", 0},
		{"garbage", "1.0.0", 0}, // unparseable never reports an update
	}
	for _, tc := range cases {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersions(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
	if Parseable("nightly") || Parseable("") || !Parseable("v1.2.3") {
		t.Fatal("Parseable mismatch")
	}
}

func TestCheckerFetchesAndCaches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","html_url":"https://example/rel","name":"1.2.0","published_at":"2026-10-01T00:00:00Z","prerelease":false,"draft":false}`))
	}))
	defer srv.Close()

	c := New(srv.Client(), "test-agent")
	if _, ok := c.Status(); ok {
		t.Fatal("fresh checker must have no cached status")
	}
	res, err := c.Check(context.Background(), srv.URL, "1.1.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !res.UpdateAvailable || res.Latest != "1.2.0" || res.URL == "" || res.CheckedAt == "" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if cached, ok := c.Status(); !ok || cached.Latest != "1.2.0" {
		t.Fatalf("result not cached: %+v ok=%v", cached, ok)
	}

	// Same version -> no update.
	res, err = c.Check(context.Background(), srv.URL, "1.2.0")
	if err != nil || res.UpdateAvailable {
		t.Fatalf("same version must not report an update: %+v err=%v", res, err)
	}
}

func TestCheckerErrorsOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()
	if _, err := New(srv.Client(), "t").Check(context.Background(), srv.URL, "1.0.0"); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}
