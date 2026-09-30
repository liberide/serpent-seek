package httpd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/liberide/serpent-seek/internal/auth"
	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/engine"
	"github.com/liberide/serpent-seek/internal/jobs"
	"github.com/liberide/serpent-seek/internal/logging"
	"github.com/liberide/serpent-seek/internal/providers"
	"github.com/liberide/serpent-seek/internal/sse"
	"github.com/liberide/serpent-seek/internal/store"
	sqlitestore "github.com/liberide/serpent-seek/internal/store/sqlite"
)

type stubProvider struct{}

func (stubProvider) Code() string { return "stub_ok" }
func (stubProvider) Schema() providers.ProviderSchema {
	return providers.ProviderSchema{Code: "stub_ok", Name: "Stub"}
}
func (stubProvider) Search(context.Context, providers.Query, providers.Credentials, providers.Params) providers.Result {
	return providers.Result{OK: true, Kind: providers.KindOK, Rows: providers.Rows{{Link: "https://a", Title: "A"}}}
}

type testServer struct {
	server *Server
	store  store.Storage
	key    string
	user   *store.User
}

func newTestServer(t *testing.T, authEnabled bool) *testServer {
	t.Helper()
	ctx := context.Background()
	driver, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "http.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := driver.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	var st store.Storage = driver

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	settings := config.NewManager(cfg, st)
	if err := settings.Refresh(ctx); err != nil {
		t.Fatalf("settings: %v", err)
	}
	if authEnabled {
		_ = settings.Set(ctx, config.KeyAuthEnabled, "true")
	} else {
		_ = settings.Set(ctx, config.KeyAuthEnabled, "false")
	}

	registry := providers.NewRegistry(providers.NewHTTPClient("test"))
	registry.Register(stubProvider{})
	log := logging.New(logging.Options{Level: "error"})
	hub := sse.NewHub()
	eng := engine.New(st, registry, settings, engine.NewStoreSink(st, hub, log), log)

	user := &store.User{ID: uuid.NewString(), Name: "admin", Role: "admin", CreatedAt: store.Now()}
	if err := st.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	plaintext, prefix, hash, err := auth.GenerateKey(true)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := st.CreateAPIKey(ctx, &store.APIKey{
		ID: uuid.NewString(), UserID: user.ID, Name: "key", Prefix: prefix, Hash: hash,
		Scopes: "admin,search,read", CreatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("create key: %v", err)
	}

	// Seed one provider and an active chain using the stub provider.
	if err := st.UpsertProvider(ctx, &store.Provider{ID: "stub_ok", Code: "stub_ok", Name: "Stub", Enabled: true, Credentials: map[string]string{}, Params: map[string]string{}}); err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	chain := &store.Chain{
		ID: uuid.NewString(), Name: "test chain",
		Nodes: []store.ChainNode{{Key: "a", ProviderID: "stub_ok", TimeoutMS: 2000, OnSuccess: "stop", OnEmpty: "next", OnFail: "next", IsStart: true, Params: map[string]string{}}},
	}
	if err := st.SaveChain(ctx, chain); err != nil {
		t.Fatalf("save chain: %v", err)
	}
	if err := st.ActivateChain(ctx, chain.ID); err != nil {
		t.Fatalf("activate chain: %v", err)
	}

	authenticator := &auth.Authenticator{Store: st, Settings: settings, Log: log}
	jobManager := jobs.New(st, settings, log)
	server := New(cfg, settings, st, eng, hub, log, authenticator, nil, jobManager, "setup-token")
	return &testServer{server: server, store: st, key: plaintext, user: user}
}

func (ts *testServer) do(t *testing.T, method, path string, body any, withKey bool, headers ...map[string]string) *httptest.ResponseRecorder {
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
	if withKey {
		req.Header.Set("Authorization", "Bearer "+ts.key)
	}
	for _, h := range headers {
		for k, v := range h {
			req.Header.Set(k, v)
		}
	}
	rec := httptest.NewRecorder()
	ts.server.Routes().ServeHTTP(rec, req)
	return rec
}

func TestHealthAndVersion(t *testing.T) {
	ts := newTestServer(t, true)
	if rec := ts.do(t, http.MethodGet, "/healthz", nil, false); rec.Code != http.StatusOK {
		t.Fatalf("healthz status %d", rec.Code)
	}
	if rec := ts.do(t, http.MethodGet, "/version", nil, false); rec.Code != http.StatusOK {
		t.Fatalf("version status %d", rec.Code)
	}
}

func TestAPIRequiresAuth(t *testing.T) {
	ts := newTestServer(t, true)
	if rec := ts.do(t, http.MethodGet, "/api/me", nil, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without key, got %d", rec.Code)
	}
	rec := ts.do(t, http.MethodGet, "/api/me", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with key, got %d", rec.Code)
	}
	if rec.Header().Get("X-Serpent-Rid") != "" {
		t.Fatal("non-search requests must not carry a rid")
	}
}

func TestSearchAndHistory(t *testing.T) {
	ts := newTestServer(t, true)
	rec := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "hello", "count": 3}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("search status %d body %s", rec.Code, rec.Body.String())
	}
	var rows []providers.Row
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("unexpected rows %s (%v)", rec.Body.String(), err)
	}
	if rec.Header().Get("X-Serpent-Rid") == "" {
		t.Fatal("search requests must carry the engine rid in X-Serpent-Rid")
	}
	if rec.Header().Get("X-Serpent-Api") != "Stub" {
		t.Fatalf("unexpected X-Serpent-Api %q", rec.Header().Get("X-Serpent-Api"))
	}

	history := ts.do(t, http.MethodGet, "/api/requests?limit=10", nil, true)
	if history.Code != http.StatusOK || !bytes.Contains(history.Body.Bytes(), []byte(`"items"`)) {
		t.Fatalf("history status %d body %s", history.Code, history.Body.String())
	}
}

func TestStatsAnalyticsEndpoint(t *testing.T) {
	ts := newTestServer(t, true)
	// Run a search so there is at least one request and one provider step today.
	if rec := ts.do(t, http.MethodPost, "/search", map[string]any{"query": "analytics", "count": 1}, true); rec.Code != http.StatusOK {
		t.Fatalf("search status %d body %s", rec.Code, rec.Body.String())
	}

	rec := ts.do(t, http.MethodGet, "/api/stats/analytics", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("analytics status %d body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{`"report"`, `"providers"`, `"range"`, `"daily"`, `"hourly"`, `"top_queries"`, `"Stub"`} {
		if !bytes.Contains(rec.Body.Bytes(), []byte(needle)) {
			t.Fatalf("analytics response missing %s: %s", needle, body)
		}
	}

	// Decode the payload to catch serialization/tag regressions.
	var payload struct {
		Report struct {
			TotalRequests int `json:"total_requests"`
			Daily         []struct {
				Date     string `json:"date"`
				Requests int    `json:"requests"`
			} `json:"daily"`
			Providers any `json:"providers"`
		} `json:"report"`
		Providers []struct {
			Provider string `json:"provider"`
			Code     string `json:"code"`
			Total    int    `json:"total"`
			OK       int    `json:"ok"`
			Fail     int    `json:"fail"`
		} `json:"providers"`
		Range struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"range"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode analytics: %v", err)
	}
	if payload.Report.TotalRequests < 1 || len(payload.Report.Daily) == 0 {
		t.Fatalf("unexpected report totals: %+v", payload.Report)
	}
	if payload.Report.Providers != nil {
		t.Fatalf("report.providers must not be serialised (got %v)", payload.Report.Providers)
	}
	if len(payload.Providers) == 0 || payload.Providers[0].Provider != "Stub" {
		t.Fatalf("expected Stub in provider table: %+v", payload.Providers)
	}
	if payload.Range.From == "" || payload.Range.To == "" {
		t.Fatalf("missing effective range: %+v", payload.Range)
	}

	bad := ts.do(t, http.MethodGet, "/api/stats/analytics?from=nope", nil, true)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid range, got %d", bad.Code)
	}
}

func TestAuthDisabledBypass(t *testing.T) {
	ts := newTestServer(t, false)
	rec := ts.do(t, http.MethodGet, "/api/me", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected open admin API, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"admin":true`)) {
		t.Fatalf("expected admin identity, got %s", rec.Body.String())
	}
}

func TestSettingsEnvAndSecretMasking(t *testing.T) {
	ts := newTestServer(t, true)
	rec := ts.do(t, http.MethodGet, "/api/settings", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("settings status %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"descriptors"`)) {
		t.Fatalf("settings response missing descriptors: %s", rec.Body.String())
	}
}

func TestChainsCRUDAndValidation(t *testing.T) {
	ts := newTestServer(t, true)
	// Invalid chain (cycle) is rejected.
	cycle := map[string]any{
		"name": "cycle",
		"nodes": []map[string]any{
			{"key": "a", "provider": "stub_ok", "timeout_ms": 1000, "retries": 0, "retry_delay_ms": 0, "delay_policy": "none", "on_success": "stop", "on_empty": "next", "on_fail": "next", "is_start": true},
			{"key": "b", "provider": "stub_ok", "timeout_ms": 1000, "retries": 0, "retry_delay_ms": 0, "delay_policy": "none", "on_success": "stop", "on_empty": "next", "on_fail": "next"},
		},
		"edges": []map[string]any{
			{"from_key": "a", "to_key": "b", "condition": "fail"},
			{"from_key": "b", "to_key": "a", "condition": "fail"},
		},
	}
	if rec := ts.do(t, http.MethodPost, "/api/chains", cycle, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("cycle should be rejected, got %d", rec.Code)
	}
	if rec := ts.do(t, http.MethodGet, "/api/chains", nil, true); rec.Code != http.StatusOK {
		t.Fatalf("list chains status %d", rec.Code)
	}
}

func TestProviderTestEndpoint(t *testing.T) {
	ts := newTestServer(t, true)
	rec := ts.do(t, http.MethodPost, "/api/providers/stub_ok/test", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("provider test status %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"ok":true`)) {
		t.Fatalf("provider test did not succeed: %s", rec.Body.String())
	}
}

func TestMaintenanceClearLogs(t *testing.T) {
	ts := newTestServer(t, true)
	if rec := ts.do(t, http.MethodPost, "/api/maintenance/clear-logs", nil, true); rec.Code != http.StatusOK {
		t.Fatalf("clear logs status %d", rec.Code)
	}
	if rec := ts.do(t, http.MethodPost, "/api/maintenance/clear-history", nil, true); rec.Code != http.StatusOK {
		t.Fatalf("clear history status %d", rec.Code)
	}
}

func TestProviderInstances(t *testing.T) {
	ts := newTestServer(t, true)

	create := func(body map[string]any) *httptest.ResponseRecorder {
		return ts.do(t, http.MethodPost, "/api/providers", body, true)
	}

	// Two instances of the same driver with different names and keys.
	rec := create(map[string]any{"code": "stub_ok", "name": "work", "credentials": map[string]string{"api_key": "k1"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create work: %d %s", rec.Code, rec.Body.String())
	}
	var work providerView
	_ = json.Unmarshal(rec.Body.Bytes(), &work)

	rec = create(map[string]any{"code": "stub_ok", "name": "personal", "credentials": map[string]string{"api_key": "k2"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create personal: %d %s", rec.Code, rec.Body.String())
	}
	var personal providerView
	_ = json.Unmarshal(rec.Body.Bytes(), &personal)

	if work.ID == personal.ID || work.Code != personal.Code {
		t.Fatalf("instances should share the driver code but have distinct ids: %+v / %+v", work, personal)
	}
	if !work.CredentialsSet["api_key"] || !personal.CredentialsSet["api_key"] {
		t.Fatalf("credentials_set should reflect stored keys")
	}

	// Duplicate name is rejected.
	if rec := create(map[string]any{"code": "stub_ok", "name": "work"}); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate name should be 409, got %d", rec.Code)
	}
	// Unknown driver is rejected.
	if rec := create(map[string]any{"code": "nope", "name": "x"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown driver should be 400, got %d", rec.Code)
	}

	// Toggle enabled off via PATCH.
	rec = ts.do(t, http.MethodPatch, "/api/providers/"+work.ID, map[string]any{"enabled": false}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle status %d: %s", rec.Code, rec.Body.String())
	}
	var toggled providerView
	_ = json.Unmarshal(rec.Body.Bytes(), &toggled)
	if toggled.Enabled {
		t.Fatalf("provider should be disabled after PATCH")
	}

	// The seeded stub instance is referenced by the active chain: delete refused.
	if rec := ts.do(t, http.MethodDelete, "/api/providers/stub_ok", nil, true); rec.Code != http.StatusConflict {
		t.Fatalf("deleting a provider used by a chain should be 409, got %d", rec.Code)
	}
}

func TestSetupFlow(t *testing.T) {
	ctx := context.Background()
	driver, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "setup.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := driver.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	var st store.Storage = driver

	srv := &Server{store: st, log: logging.New(logging.Options{Level: "error"}), setupToken: "abc123"}

	call := func(body map[string]any) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.handleSetup(rec, req)
		return rec
	}

	if rec := call(map[string]any{"token": "wrong", "name": "admin"}); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong token should be 403, got %d: %s", rec.Code, rec.Body.String())
	}

	// The token is trimmed, so surrounding whitespace must still work.
	rec := call(map[string]any{"token": "  abc123  ", "name": "admin"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("valid token should create the admin, got %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("api_key")) {
		t.Fatalf("setup did not return an api key: %s", rec.Body.String())
	}

	// Token is invalidated and setup is complete.
	if rec := call(map[string]any{"token": "abc123", "name": "admin2"}); rec.Code != http.StatusConflict {
		t.Fatalf("second setup should be 409, got %d", rec.Code)
	}
	if stored, err := st.GetSetting(ctx, config.KeySetupToken); err == nil && stored != "" {
		t.Fatalf("persisted setup token should be cleared, got %q", stored)
	}
}

func TestProxiesCRUDAndDeleteClearsProvider(t *testing.T) {
	ts := newTestServer(t, true)

	rec := ts.do(t, http.MethodPost, "/api/proxies", map[string]any{
		"name": "work", "type": "socks5", "host": "proxy.local", "port": "1080",
		"username": "u", "password": "p",
	}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create proxy: %d %s", rec.Code, rec.Body.String())
	}
	var created proxyView
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode proxy: %v", err)
	}
	if created.Type != "socks5" || !created.PasswordSet || created.Host != "proxy.local" || created.Port != "1080" {
		t.Fatalf("unexpected proxy view: %+v", created)
	}

	// Attach the proxy to the seeded provider.
	rec = ts.do(t, http.MethodPut, "/api/providers/stub_ok", map[string]any{"proxy_id": created.ID}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("attach proxy: %d %s", rec.Code, rec.Body.String())
	}
	p, err := ts.store.GetProvider(context.Background(), "stub_ok")
	if err != nil || p.ProxyID != created.ID {
		t.Fatalf("provider proxy not set: %+v %v", p, err)
	}

	// Enable/disable via PATCH.
	rec = ts.do(t, http.MethodPatch, "/api/proxies/"+created.ID, map[string]any{"enabled": false}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch proxy: %d %s", rec.Code, rec.Body.String())
	}
	var patched proxyView
	_ = json.Unmarshal(rec.Body.Bytes(), &patched)
	if patched.Enabled {
		t.Fatalf("proxy should be disabled: %+v", patched)
	}

	// Deleting clears the provider reference rather than refusing.
	rec = ts.do(t, http.MethodDelete, "/api/proxies/"+created.ID, nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete proxy: %d %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"cleared":1`)) {
		t.Fatalf("expected cleared=1 in %s", rec.Body.String())
	}
	p, _ = ts.store.GetProvider(context.Background(), "stub_ok")
	if p.ProxyID != "" {
		t.Fatalf("provider proxy should be cleared, got %q", p.ProxyID)
	}

	// Reject a provider pointing at an unknown proxy.
	rec = ts.do(t, http.MethodPut, "/api/providers/stub_ok", map[string]any{"proxy_id": "missing"}, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown proxy should be rejected, got %d: %s", rec.Code, rec.Body.String())
	}
}
