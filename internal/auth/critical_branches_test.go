package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/store"
)

// TestRevokedKeyRejected verifies that a revoked key is rejected.
func TestRevokedKeyRejected(t *testing.T) {
	a, mgr, _, key := newAuthEnv(t)
	ctx := context.Background()
	_ = mgr.Set(ctx, config.KeyAuthEnabled, "true")
	m := &Middleware{Auth: a}

	pass := func() int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		m.Authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, req)
		return rec.Code
	}

	if pass() != http.StatusOK {
		t.Fatal("fresh key must authenticate")
	}
	stored, err := a.Store.GetAPIKey(ctx, "k1")
	if err != nil {
		t.Fatalf("load key: %v", err)
	}
	stored.RevokedAt = store.Now()
	if err := a.Store.UpdateAPIKey(ctx, stored); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if got := pass(); got != http.StatusUnauthorized {
		t.Fatalf("revoked key must 401, got %d", got)
	}
}

// TestDeveloperTokenGatedByMode verifies that developer tokens require developer mode.
func TestDeveloperTokenGatedByMode(t *testing.T) {
	a, mgr, user, _ := newAuthEnv(t)
	ctx := context.Background()
	_ = mgr.Set(ctx, config.KeyAuthEnabled, "true")

	plaintext, prefix, hash, err := GenerateDeveloperKey()
	if err != nil {
		t.Fatalf("generate developer key: %v", err)
	}
	if err := a.Store.CreateAPIKey(ctx, &store.APIKey{
		ID: uuid.NewString(), UserID: user.ID, Name: "dev", Prefix: prefix, Hash: hash,
		Scopes: "developer", CreatedAt: store.Now(),
	}); err != nil {
		t.Fatalf("store developer key: %v", err)
	}
	m := &Middleware{Auth: a}

	pass := func() (int, *Identity) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		var seen *Identity
		m.Authenticate(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = IdentityFrom(r.Context()) })).ServeHTTP(rec, req)
		return rec.Code, seen
	}

	// Mode off: reject.
	if code, _ := pass(); code != http.StatusUnauthorized {
		t.Fatalf("mode off: developer token must 401, got %d", code)
	}
	// Mode on: authenticate as developer admin.
	if err := mgr.Set(ctx, config.KeyDeveloperMode, "true"); err != nil {
		t.Fatalf("enable developer mode: %v", err)
	}
	code, id := pass()
	if code != http.StatusOK || id == nil || !id.IsDeveloper() || !id.IsAdmin() {
		t.Fatalf("mode on: code=%d identity=%+v, want developer admin", code, id)
	}
	// Mode off again: reject.
	_ = mgr.Set(ctx, config.KeyDeveloperMode, "false")
	if code, _ := pass(); code != http.StatusUnauthorized {
		t.Fatalf("mode disabled again: must 401, got %d", code)
	}
}

// TestExpiredSessionRejected verifies expired session rejection.
func TestExpiredSessionRejected(t *testing.T) {
	a, mgr, user, _ := newAuthEnv(t)
	ctx := context.Background()
	_ = mgr.Set(ctx, config.KeyAuthEnabled, "true")
	m := &Middleware{Auth: a}

	token, err := CreateSession(ctx, a.Store, user.ID, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	// Expire the stored session.
	sessions := a.Store
	sess, err := sessions.GetSessionByTokenHash(ctx, HashToken(token))
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if err := sessions.UpdateSessionExpiry(ctx, sess.ID, past); err != nil {
		t.Fatalf("expire session: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	m.Authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired session must 401, got %d", rec.Code)
	}
}

// TestKeyOfDisabledUserRejected verifies rejection for a disabled owner.
func TestKeyOfDisabledUserRejected(t *testing.T) {
	a, mgr, user, key := newAuthEnv(t)
	ctx := context.Background()
	_ = mgr.Set(ctx, config.KeyAuthEnabled, "true")
	m := &Middleware{Auth: a}

	user.Disabled = true
	if err := a.Store.UpdateUser(ctx, user); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	m.Authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("key of a disabled user must 401, got %d", rec.Code)
	}
}
