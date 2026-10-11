package auth

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/logging"
	"github.com/liberide/serpent-seek/internal/store"
	sqlitestore "github.com/liberide/serpent-seek/internal/store/sqlite"
)

// newAuthEnv builds an Authenticator backed by a migrated SQLite store with one
// admin user and one admin API key.
func newAuthEnv(t *testing.T) (*Authenticator, *config.Manager, *store.User, string) {
	t.Helper()
	ctx := context.Background()
	driver, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "auth.db"))
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
	mgr := config.NewManager(cfg, st)
	if err := mgr.Refresh(ctx); err != nil {
		t.Fatalf("settings refresh: %v", err)
	}
	user := &store.User{ID: "u1", Name: "admin", Role: "admin", CreatedAt: store.Now()}
	if err := st.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	plaintext, prefix, hash, err := GenerateKey(true)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := st.CreateAPIKey(ctx, &store.APIKey{
		ID: "k1", UserID: user.ID, Name: "key", Prefix: prefix, Hash: hash,
		Scopes: "admin,search,read", CreatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("create key: %v", err)
	}
	a := &Authenticator{Store: st, Settings: mgr, Log: logging.New(logging.Options{Level: "error"})}
	return a, mgr, user, plaintext
}

func TestAuthenticateVariants(t *testing.T) {
	a, mgr, _, key := newAuthEnv(t)
	ctx := context.Background()
	_ = mgr.Set(ctx, config.KeyAuthEnabled, "true")
	m := &Middleware{Auth: a}

	// Anonymous request is rejected.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	called := false
	m.Authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || called {
		t.Fatalf("anonymous request must 401, got %d called=%v", rec.Code, called)
	}

	// Bearer key resolves to an admin identity.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	var seen *Identity
	m.Authenticate(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = IdentityFrom(r.Context()) })).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || seen == nil || seen.KeyID != "k1" || !seen.IsAdmin() || seen.Via != "key" {
		t.Fatalf("bearer key not resolved: code=%d identity=%+v", rec.Code, seen)
	}

	// An invalid bearer key is rejected.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer seek_admin_bad_bad")
	m.Authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid key must 401, got %d", rec.Code)
	}

	// When auth is disabled the middleware grants an anonymous admin.
	_ = mgr.Set(ctx, config.KeyAuthEnabled, "false")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	seen = nil
	m.Authenticate(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = IdentityFrom(r.Context()) })).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || seen == nil || seen.Via != "disabled" || !seen.IsAdmin() {
		t.Fatalf("disabled auth must grant admin: code=%d identity=%+v", rec.Code, seen)
	}
}

func TestSessionCSRFEnforcement(t *testing.T) {
	a, mgr, user, _ := newAuthEnv(t)
	ctx := context.Background()
	_ = mgr.Set(ctx, config.KeyAuthEnabled, "true")
	m := &Middleware{Auth: a}

	token, err := CreateSession(ctx, a.Store, user.ID, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Safe method works without a CSRF token.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	var seen *Identity
	m.Authenticate(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = IdentityFrom(r.Context()) })).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || seen == nil || seen.Via != "session" {
		t.Fatalf("session GET failed: code=%d identity=%+v", rec.Code, seen)
	}

	// Unsafe method without the double-submit token is forbidden.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	m.Authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("session POST without CSRF must 403, got %d", rec.Code)
	}

	// With a matching cookie/header pair it passes.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "csrf-token"})
	req.Header.Set(CSRFHeaderName, "csrf-token")
	m.Authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("session POST with CSRF must 200, got %d", rec.Code)
	}
}

func TestRequireAdminAndScope(t *testing.T) {
	m := &Middleware{}
	admin := WithIdentity(context.Background(), &Identity{Scopes: map[string]bool{"admin": true, "read": true}})
	viewer := WithIdentity(context.Background(), &Identity{Scopes: map[string]bool{"read": true}})

	cases := []struct {
		name  string
		ctx   context.Context
		guard func(http.Handler) http.Handler
		want  int
	}{
		{"admin ok", admin, m.RequireAdmin, http.StatusOK},
		{"viewer forbidden", viewer, m.RequireAdmin, http.StatusForbidden},
		{"anonymous forbidden", context.Background(), m.RequireAdmin, http.StatusForbidden},
		{"read scope ok", viewer, m.RequireScope("read"), http.StatusOK},
		{"admin scope on viewer", viewer, m.RequireScope("admin"), http.StatusForbidden},
		{"missing identity", context.Background(), m.RequireScope("read"), http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(tc.ctx)
			tc.guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("want %d, got %d", tc.want, rec.Code)
			}
		})
	}
}

func TestOptionalAndSecure(t *testing.T) {
	a, _, _, key := newAuthEnv(t)
	m := &Middleware{Auth: a}

	// Optional passes anonymous requests through without an identity.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	var seen *Identity
	m.Optional(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = IdentityFrom(r.Context()) })).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || seen != nil {
		t.Fatalf("optional anonymous: code=%d identity=%+v", rec.Code, seen)
	}

	// Optional resolves a provided key.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	m.Optional(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = IdentityFrom(r.Context()) })).ServeHTTP(rec, req)
	if seen == nil {
		t.Fatal("optional did not resolve the key")
	}

	if _, err := a.LoginWithKey(context.Background(), key); err != nil {
		t.Fatalf("LoginWithKey: %v", err)
	}

	// Secure: TLS, forwarded proto and https origin.
	tlsReq := httptest.NewRequest(http.MethodGet, "/", nil)
	tlsReq.TLS = &tls.ConnectionState{}
	if !a.Secure(tlsReq) {
		t.Fatal("TLS request must be secure")
	}
	fwdReq := httptest.NewRequest(http.MethodGet, "/", nil)
	fwdReq.Header.Set("X-Forwarded-Proto", "https")
	if !a.Secure(fwdReq) {
		t.Fatal("forwarded https must be secure")
	}
	plainReq := httptest.NewRequest(http.MethodGet, "/", nil)
	if a.Secure(plainReq) {
		t.Fatal("plain HTTP must not be secure")
	}
}

func TestCSRFHelpersAndTokens(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	EnsureCSRFCookie(rec, req, true)
	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == CSRFCookieName && c.Value != "" && c.Secure {
			found = true
		}
	}
	if !found {
		t.Fatal("EnsureCSRFCookie did not set a secure cookie")
	}

	// A GET is always allowed; a POST requires the header/cookie match.
	if !ValidateCSRF(httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Fatal("GET must pass CSRF")
	}
	post := httptest.NewRequest(http.MethodPost, "/", nil)
	if ValidateCSRF(post) {
		t.Fatal("POST without tokens must fail CSRF")
	}

	if tok, err := RandomToken(8); err != nil || len(tok) != 16 {
		t.Fatalf("RandomToken: %q %v", tok, err)
	}
	if ip := ClientIP(httptest.NewRequest(http.MethodGet, "/", nil)); ip == "" {
		t.Fatal("ClientIP returned empty")
	}
}
