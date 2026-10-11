package httpd

import (
	"encoding/json"
	"net/http"
	"testing"
)

// createKey creates one search key and returns its id.
func createKey(t *testing.T, ts *testServer, name string) string {
	t.Helper()
	rec := ts.do(t, http.MethodPost, "/api/keys", map[string]any{
		"user_id": ts.user.ID, "name": name, "scope": "search",
	}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key %s: %d %s", name, rec.Code, rec.Body.String())
	}
	var created struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Key.ID == "" {
		t.Fatalf("decode key: %v %s", err, rec.Body.String())
	}
	return created.Key.ID
}

func TestBulkKeys(t *testing.T) {
	ts := newTestServer(t, true)
	first := createKey(t, ts, "bulk one")
	second := createKey(t, ts, "bulk two")

	rec := ts.do(t, http.MethodPost, "/api/keys/bulk", map[string]any{
		"action": "revoke", "ids": []string{first, second, first, "missing"},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("bulk revoke: %d %s", rec.Code, rec.Body.String())
	}
	var result bulkResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Requested != 3 || result.Updated != 2 {
		t.Fatalf("expected requested=3 updated=2, got %+v", result)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].ID != "missing" || result.Skipped[0].Reason != "not_found" {
		t.Fatalf("unexpected skipped: %+v", result.Skipped)
	}

	// Both keys must now be revoked.
	rec = ts.do(t, http.MethodGet, "/api/keys", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("list keys: %d", rec.Code)
	}
	var keys []struct {
		ID        string `json:"id"`
		RevokedAt string `json:"revoked_at"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &keys)
	revoked := 0
	for _, k := range keys {
		if k.ID == first || k.ID == second {
			if k.RevokedAt == "" {
				t.Fatalf("key %s not revoked", k.ID)
			}
			revoked++
		}
	}
	if revoked != 2 {
		t.Fatalf("expected 2 revoked keys, got %d", revoked)
	}

	// An unknown action is rejected.
	rec = ts.do(t, http.MethodPost, "/api/keys/bulk", map[string]any{"action": "nuke", "ids": []string{first}}, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid action should be 400, got %d", rec.Code)
	}
}

func TestBulkProviders(t *testing.T) {
	ts := newTestServer(t, true)

	// Register a second, chain-free provider instance.
	rec := ts.do(t, http.MethodPost, "/api/providers", map[string]any{
		"code": "stub_ok", "name": "bulk spare",
	}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create provider: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	// The seeded stub_ok is referenced by the active chain, so deleting it is
	// skipped while the spare instance is removed.
	rec = ts.do(t, http.MethodPost, "/api/providers/bulk", map[string]any{
		"action": "delete", "ids": []string{"stub_ok", created.ID},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("bulk delete: %d %s", rec.Code, rec.Body.String())
	}
	var result bulkResult
	_ = json.Unmarshal(rec.Body.Bytes(), &result)
	if result.Updated != 1 || result.Requested != 2 {
		t.Fatalf("expected updated=1 requested=2, got %+v", result)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].ID != "stub_ok" || result.Skipped[0].Reason != "provider_in_use" {
		t.Fatalf("unexpected skipped: %+v", result.Skipped)
	}

	// Disabling the chain-used provider is allowed.
	rec = ts.do(t, http.MethodPost, "/api/providers/bulk", map[string]any{
		"action": "disable", "ids": []string{"stub_ok"},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("bulk disable: %d %s", rec.Code, rec.Body.String())
	}
	rec = ts.do(t, http.MethodGet, "/api/providers/stub_ok", nil, true)
	var view providerView
	_ = json.Unmarshal(rec.Body.Bytes(), &view)
	if view.Enabled {
		t.Fatalf("provider should be disabled: %+v", view)
	}
}
