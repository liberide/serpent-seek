package httpd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/liberide/serpent-seek/internal/auth"
	"github.com/liberide/serpent-seek/internal/store"
)

// doAs issues a request with an explicit bearer key.
func (ts *testServer) doAs(t *testing.T, method, path, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	ts.server.Routes().ServeHTTP(rec, req)
	return rec
}

// TestSearchKeyScopeEnforcement checks scope-based access.
func TestSearchKeyScopeEnforcement(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()

	viewer := &store.User{ID: uuid.NewString(), Name: "viewer", Role: "viewer", CreatedAt: store.Now()}
	if err := ts.store.CreateUser(ctx, viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	plaintext, prefix, hash, err := auth.GenerateKey(false)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := ts.store.CreateAPIKey(ctx, &store.APIKey{
		ID: uuid.NewString(), UserID: viewer.ID, Name: "search key",
		Prefix: prefix, Hash: hash, Scopes: "search", CreatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("create key: %v", err)
	}

	if rec := ts.doAs(t, http.MethodPost, "/search", plaintext, map[string]any{"query": "hello"}); rec.Code != http.StatusOK {
		t.Fatalf("search key search status %d", rec.Code)
	}
	for _, path := range []string{"/api/requests", "/api/logs", "/api/settings", "/api/stats/summary"} {
		if rec := ts.doAs(t, http.MethodGet, path, plaintext, nil); rec.Code != http.StatusForbidden {
			t.Fatalf("search key %s status %d, want 403", path, rec.Code)
		}
	}
	if rec := ts.do(t, http.MethodGet, "/api/requests", nil, true); rec.Code != http.StatusOK {
		t.Fatalf("admin history status %d", rec.Code)
	}

	// Search key cannot open a browser session.
	if rec := ts.doAs(t, http.MethodPost, "/api/auth/login", "", map[string]any{"key": plaintext}); rec.Code != http.StatusForbidden {
		t.Fatalf("search key login status %d, want 403", rec.Code)
	}
}
