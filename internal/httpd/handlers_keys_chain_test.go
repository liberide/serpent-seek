package httpd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestKeyChainBinding(t *testing.T) {
	ts := newTestServer(t, true)

	// Create a second, inactive chain using the stub provider.
	rec := ts.do(t, http.MethodPost, "/api/chains", map[string]any{
		"name": "bound chain",
		"nodes": []map[string]any{{
			"key": "a", "provider_id": "stub_ok", "timeout_ms": 2000,
			"on_success": "stop", "on_empty": "next", "on_fail": "next", "is_start": true,
		}},
		"edges": []map[string]any{},
	}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create chain: %d %s", rec.Code, rec.Body.String())
	}
	var chain struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &chain); err != nil || chain.ID == "" {
		t.Fatalf("decode chain: %v %s", err, rec.Body.String())
	}

	// An unknown chain id is rejected at creation time.
	rec = ts.do(t, http.MethodPost, "/api/keys", map[string]any{
		"user_id": ts.user.ID, "name": "bad", "scope": "search", "chain_id": "missing",
	}, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown chain should be rejected: %d %s", rec.Code, rec.Body.String())
	}

	// Create a key bound to the chain.
	rec = ts.do(t, http.MethodPost, "/api/keys", map[string]any{
		"user_id": ts.user.ID, "name": "bound key", "scope": "search", "chain_id": chain.ID,
	}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Key struct {
			ID      string `json:"id"`
			ChainID string `json:"chain_id"`
		} `json:"key"`
		Plaintext string `json:"plaintext"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode key: %v", err)
	}
	if created.Key.ChainID != chain.ID {
		t.Fatalf("key not bound: %+v", created.Key)
	}

	// The chain advertises its bound key count, active and total.
	chains := fetchChains(t, ts)
	var boundKeys, activeBoundKeys int
	for _, c := range chains {
		if c.ID == chain.ID {
			boundKeys = c.BoundKeys
			activeBoundKeys = c.ActiveBoundKeys
		}
	}
	if boundKeys != 1 || activeBoundKeys != 1 {
		t.Fatalf("expected bound_keys=1 active_bound_keys=1, got %d/%d", boundKeys, activeBoundKeys)
	}

	// A token bound to an *inactive* chain must run that chain, not error out or
	// silently fall back to the active one. The machine endpoint answers 200 even
	// on failure, so assert the provider actually ran.
	rec = ts.doAs(t, http.MethodPost, "/search", created.Plaintext, map[string]any{"query": "inactive chain", "count": 1})
	if rec.Code != http.StatusOK {
		t.Fatalf("search with inactive bound chain: %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Serpent-Api"); got != "Stub" {
		t.Fatalf("inactive bound chain did not run: X-Serpent-Api=%q body=%s", got, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("https://a")) {
		t.Fatalf("expected stub row from inactive bound chain, got %s", rec.Body.String())
	}

	// Clear then rebind through PATCH.
	rec = ts.do(t, http.MethodPatch, "/api/keys/"+created.Key.ID, map[string]any{"chain_id": ""}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch clear: %d %s", rec.Code, rec.Body.String())
	}
	var patched struct {
		ChainID string `json:"chain_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &patched)
	if patched.ChainID != "" {
		t.Fatalf("chain binding should be cleared, got %q", patched.ChainID)
	}
	rec = ts.do(t, http.MethodPatch, "/api/keys/"+created.Key.ID, map[string]any{"chain_id": chain.ID}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch rebind: %d %s", rec.Code, rec.Body.String())
	}

	// A revoked token keeps its binding but is no longer "active", so the
	// deletion warning must not count it.
	rec = ts.do(t, http.MethodDelete, "/api/keys/"+created.Key.ID, nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke key: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range fetchChains(t, ts) {
		if c.ID == chain.ID && (c.BoundKeys != 1 || c.ActiveBoundKeys != 0) {
			t.Fatalf("revoked token must keep binding but not be active: %+v", c)
		}
	}

	// Deleting the chain unbinds the key and reports the count.
	rec = ts.do(t, http.MethodDelete, "/api/chains/"+chain.ID, nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete chain: %d %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"unbound_keys":1`)) {
		t.Fatalf("expected unbound_keys=1 in %s", rec.Body.String())
	}
	key, err := ts.store.GetAPIKey(context.Background(), created.Key.ID)
	if err != nil || key.ChainID != "" {
		t.Fatalf("key binding should be cleared after chain delete: %+v %v", key, err)
	}

	// The active chain cannot be deleted.
	activeID := ""
	for _, c := range fetchChains(t, ts) {
		if c.Active {
			activeID = c.ID
		}
	}
	if activeID == "" {
		t.Fatal("no active chain found")
	}
	rec = ts.do(t, http.MethodDelete, "/api/chains/"+activeID, nil, true)
	if rec.Code != http.StatusConflict {
		t.Fatalf("active chain delete should conflict: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteBoundChainMovesTokenToDefault(t *testing.T) {
	ts := newTestServer(t, true)

	// A second, inactive chain with the stub provider.
	rec := ts.do(t, http.MethodPost, "/api/chains", map[string]any{
		"name": "in-use chain",
		"nodes": []map[string]any{{
			"key": "a", "provider_id": "stub_ok", "timeout_ms": 2000,
			"on_success": "stop", "on_empty": "next", "on_fail": "next", "is_start": true,
		}},
		"edges": []map[string]any{},
	}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create chain: %d %s", rec.Code, rec.Body.String())
	}
	var chain struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &chain); err != nil || chain.ID == "" {
		t.Fatalf("decode chain: %v %s", err, rec.Body.String())
	}

	// An active token bound to it.
	rec = ts.do(t, http.MethodPost, "/api/keys", map[string]any{
		"user_id": ts.user.ID, "name": "in-use key", "scope": "search", "chain_id": chain.ID,
	}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode key: %v", err)
	}

	// Deleting the chain always succeeds: the bound token is unbound and falls
	// back to the default (active) chain — the delete endpoint must not return a
	// confirmation error.
	rec = ts.do(t, http.MethodDelete, "/api/chains/"+chain.ID, nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete bound chain: %d %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"unbound_keys":1`)) ||
		!bytes.Contains(rec.Body.Bytes(), []byte(`"moved_to_default":1`)) {
		t.Fatalf("expected unbound_keys=1 and moved_to_default=1 in %s", rec.Body.String())
	}
	if _, err := ts.store.GetChain(context.Background(), chain.ID); err == nil {
		t.Fatal("chain should be deleted")
	}
	if key, err := ts.store.GetAPIKey(context.Background(), created.Key.ID); err != nil || key.ChainID != "" {
		t.Fatalf("key binding should be cleared after chain delete: %+v %v", key, err)
	}
}

type chainView struct {
	ID              string `json:"id"`
	Active          bool   `json:"active"`
	BoundKeys       int    `json:"bound_keys"`
	ActiveBoundKeys int    `json:"active_bound_keys"`
}

func fetchChains(t *testing.T, ts *testServer) []chainView {
	t.Helper()
	rec := ts.do(t, http.MethodGet, "/api/chains", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("list chains: %d %s", rec.Code, rec.Body.String())
	}
	var chains []chainView
	if err := json.Unmarshal(rec.Body.Bytes(), &chains); err != nil {
		t.Fatalf("decode chains: %v", err)
	}
	return chains
}
