package httpd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/liberide/serpent-seek/internal/auth"
	"github.com/liberide/serpent-seek/internal/config"
)

func TestDeveloperModeAndTokens(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()

	// Force the mode off unless the environment pins it (in which case the
	// toggle endpoint is read-only and this test cannot run meaningfully).
	if err := ts.server.settings.Set(ctx, config.KeyDeveloperMode, "false"); err != nil {
		t.Skipf("DEVELOPER_MODE pinned by environment: %v", err)
	}

	rec := ts.do(t, http.MethodGet, "/api/developer", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("developer status: %d %s", rec.Code, rec.Body.String())
	}
	var status developerStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Enabled {
		t.Fatal("developer mode should start disabled")
	}
	if len(status.Tokens) != 0 {
		t.Fatalf("expected no tokens, got %d", len(status.Tokens))
	}

	// Minting is refused while the mode is off.
	if rec := ts.do(t, http.MethodPost, "/api/developer/tokens", map[string]any{"name": "x"}, true); rec.Code != http.StatusForbidden {
		t.Fatalf("create while disabled should be 403, got %d: %s", rec.Code, rec.Body.String())
	}

	// Enable via the dedicated endpoint.
	rec = ts.do(t, http.MethodPut, "/api/developer/mode", map[string]any{"enabled": true}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable mode: %d %s", rec.Code, rec.Body.String())
	}

	// Create a developer token.
	rec = ts.do(t, http.MethodPost, "/api/developer/tokens", map[string]any{"name": "n8n"}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create token: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Token     developerToken `json:"token"`
		Plaintext string         `json:"plaintext"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if !strings.HasPrefix(created.Plaintext, auth.DeveloperKeyPrefix) {
		t.Fatalf("plaintext %q should use the developer prefix", created.Plaintext)
	}
	if kind, _, _, ok := auth.ParseKey(created.Plaintext); !ok || kind != auth.KindDeveloper {
		t.Fatalf("developer token did not parse as developer (kind=%q ok=%v)", kind, ok)
	}

	// The token works against an admin endpoint while the mode is on.
	original := ts.key
	ts.key = created.Plaintext
	rec = ts.do(t, http.MethodGet, "/api/keys", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("developer token should reach admin endpoints, got %d: %s", rec.Code, rec.Body.String())
	}
	// It is also visible in the token list.
	rec = ts.do(t, http.MethodGet, "/api/developer", nil, true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(created.Token.ID)) {
		t.Fatalf("token missing from status: %d %s", rec.Code, rec.Body.String())
	}
	ts.key = original

	// Turning the mode off instantly rejects the token (without revoking it).
	if err := ts.server.settings.Set(ctx, config.KeyDeveloperMode, "false"); err != nil {
		t.Fatalf("disable mode: %v", err)
	}
	ts.key = created.Plaintext
	if rec := ts.do(t, http.MethodGet, "/api/keys", nil, true); rec.Code != http.StatusUnauthorized {
		t.Fatalf("disabled developer token should be 401, got %d: %s", rec.Code, rec.Body.String())
	}
	ts.key = original

	// Re-enable and revoke the token.
	_ = ts.server.settings.Set(ctx, config.KeyDeveloperMode, "true")
	rec = ts.do(t, http.MethodDelete, "/api/developer/tokens/"+created.Token.ID, nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke token: %d %s", rec.Code, rec.Body.String())
	}
	ts.key = created.Plaintext
	if rec := ts.do(t, http.MethodGet, "/api/keys", nil, true); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked developer token should be 401, got %d: %s", rec.Code, rec.Body.String())
	}
	ts.key = original
}

// TestDeveloperTokenPermissions covers the per-endpoint permission list: a
// restricted token may only call the granted endpoints, the list can be edited,
// and it cannot be used to sign in for a full-admin session.
func TestDeveloperTokenPermissions(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()
	if err := ts.server.settings.Set(ctx, config.KeyDeveloperMode, "true"); err != nil {
		t.Skipf("DEVELOPER_MODE pinned by environment: %v", err)
	}

	rec := ts.do(t, http.MethodPost, "/api/developer/tokens", map[string]any{
		"name": "restricted", "permissions": []string{"GET /api/keys"},
	}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Token     developerToken `json:"token"`
		Plaintext string         `json:"plaintext"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Token.AllEndpoints || len(created.Token.Permissions) != 1 {
		t.Fatalf("token should be restricted: %+v", created.Token)
	}

	original := ts.key
	ts.key = created.Plaintext

	// The granted endpoint works, as does the always-allowed /api/me.
	if rec := ts.do(t, http.MethodGet, "/api/keys", nil, true); rec.Code != http.StatusOK {
		t.Fatalf("granted endpoint should be 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := ts.do(t, http.MethodGet, "/api/me", nil, true); rec.Code != http.StatusOK {
		t.Fatalf("/api/me should always be allowed, got %d: %s", rec.Code, rec.Body.String())
	}
	// Anything else is forbidden by the permission guard.
	if rec := ts.do(t, http.MethodPost, "/api/chains", map[string]any{"name": "x"}, true); rec.Code != http.StatusForbidden {
		t.Fatalf("non-granted endpoint should be 403, got %d: %s", rec.Code, rec.Body.String())
	}
	// A restricted token must not escalate via browser sign-in.
	if rec := ts.do(t, http.MethodPost, "/api/auth/login", map[string]any{"key": created.Plaintext}, false); rec.Code != http.StatusForbidden {
		t.Fatalf("restricted login should be 403, got %d: %s", rec.Code, rec.Body.String())
	}
	ts.key = original

	// Edit the permission list (grant chains read as well).
	rec = ts.do(t, http.MethodPatch, "/api/developer/tokens/"+created.Token.ID, map[string]any{
		"permissions": []string{"GET /api/keys", "GET /api/chains"},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	var updated developerToken
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated: %v", err)
	}
	if updated.AllEndpoints || len(updated.Permissions) != 2 {
		t.Fatalf("permissions not updated: %+v", updated)
	}

	ts.key = created.Plaintext
	if rec := ts.do(t, http.MethodGet, "/api/chains", nil, true); rec.Code != http.StatusOK {
		t.Fatalf("newly granted endpoint should be 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ts.key = original

	// Unknown endpoint ids are rejected.
	if rec := ts.do(t, http.MethodPatch, "/api/developer/tokens/"+created.Token.ID, map[string]any{
		"permissions": []string{"GET /nope"},
	}, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown endpoint should be 400, got %d: %s", rec.Code, rec.Body.String())
	}

	// The endpoint catalog is served for the picker.
	rec = ts.do(t, http.MethodGet, "/api/developer/endpoints", nil, true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("GET /api/chains")) {
		t.Fatalf("endpoint catalog: %d %s", rec.Code, rec.Body.String())
	}
}

// TestDeveloperTokenLifecycleIsAudited covers the audit trail for token changes.
func TestDeveloperTokenLifecycleIsAudited(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()
	if err := ts.server.settings.Set(ctx, config.KeyDeveloperMode, "true"); err != nil {
		t.Skipf("DEVELOPER_MODE pinned by environment: %v", err)
	}
	if rec := ts.do(t, http.MethodPost, "/api/developer/tokens", map[string]any{"name": "audit"}, true); rec.Code != http.StatusCreated {
		t.Fatalf("create token: %d %s", rec.Code, rec.Body.String())
	}
	rec := ts.do(t, http.MethodGet, "/api/audit?q=developer", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("audit list: %d %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("developer.token.create")) {
		t.Fatalf("developer token creation was not audited: %s", rec.Body.String())
	}
}

// TestDeveloperTokenPurge covers the revoke-then-delete flow: a revoked token
// can be removed for good with ?purge=true and then disappears from the list.
func TestDeveloperTokenPurge(t *testing.T) {
	ts := newTestServer(t, true)
	ctx := context.Background()
	if err := ts.server.settings.Set(ctx, config.KeyDeveloperMode, "true"); err != nil {
		t.Skipf("DEVELOPER_MODE pinned by environment: %v", err)
	}

	rec := ts.do(t, http.MethodPost, "/api/developer/tokens", map[string]any{"name": "purge"}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create token: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Token developerToken `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	id := created.Token.ID

	// Revoke first, then delete permanently.
	if rec := ts.do(t, http.MethodDelete, "/api/developer/tokens/"+id, nil, true); rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
	}
	rec = ts.do(t, http.MethodDelete, "/api/developer/tokens/"+id+"?purge=true", nil, true)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"deleted":true`)) {
		t.Fatalf("purge: %d %s", rec.Code, rec.Body.String())
	}

	// The token is gone from the list.
	rec = ts.do(t, http.MethodGet, "/api/developer", nil, true)
	if bytes.Contains(rec.Body.Bytes(), []byte(id)) {
		t.Fatalf("purged token still listed: %s", rec.Body.String())
	}

	// Purging a missing token is a 404, not a silent success.
	if rec := ts.do(t, http.MethodDelete, "/api/developer/tokens/"+id+"?purge=true", nil, true); rec.Code != http.StatusNotFound {
		t.Fatalf("purge missing should be 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
