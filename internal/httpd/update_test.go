package httpd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/liberide/serpent-seek/internal/config"
)

func TestUpdateEndpoints(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()

	// Without a completed check the cached endpoint reports the current version.
	rec := ts.do(t, http.MethodGet, "/api/update", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status %d: %s", rec.Code, rec.Body.String())
	}
	var status struct {
		Enabled         bool   `json:"enabled"`
		Current         string `json:"current"`
		Latest          string `json:"latest"`
		UpdateAvailable bool   `json:"update_available"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if !status.Enabled || status.Current == "" {
		t.Fatalf("unexpected update status: %+v", status)
	}

	// Point the checker at a local feed and force a check.
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v99.0.0","html_url":"https://example/rel","name":"99","published_at":"2026-10-01T00:00:00Z"}`))
	}))
	defer feed.Close()
	if err := ts.server.settings.Set(ctx, config.KeyUpdateCheckURL, feed.URL); err != nil {
		t.Fatalf("set url: %v", err)
	}
	rec = ts.do(t, http.MethodPost, "/api/update/check", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("force check status %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode forced check: %v", err)
	}
	if !status.UpdateAvailable || status.Latest != "99.0.0" {
		t.Fatalf("expected an available update, got %+v", status)
	}

	// Disabling the check turns the endpoint into a no-op.
	if err := ts.server.settings.Set(ctx, config.KeyUpdateCheck, "false"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	rec = ts.do(t, http.MethodGet, "/api/update", nil, true)
	_ = json.Unmarshal(rec.Body.Bytes(), &status)
	if status.Enabled {
		t.Fatalf("disabled check must report enabled=false: %+v", status)
	}
}
