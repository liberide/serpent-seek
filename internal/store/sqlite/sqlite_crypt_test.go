package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liberide/serpent-seek/internal/crypt"
	"github.com/liberide/serpent-seek/internal/store"
)

// TestAtRestEncryption checks encryption of stored secrets.
func TestAtRestEncryption(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "enc.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	c, err := crypt.New(strings.Repeat("a", 32))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	st.SetCipher(c)

	if err := st.SetSetting(ctx, "my_secret", "s3cr3t"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if got, _ := st.GetSetting(ctx, "my_secret"); got != "s3cr3t" {
		t.Fatalf("GetSetting = %q", got)
	}
	var raw string
	if err := st.DB().QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'my_secret'`).Scan(&raw); err != nil {
		t.Fatalf("raw scan: %v", err)
	}
	if !strings.HasPrefix(raw, "enc:v1:") {
		t.Fatalf("settings value not encrypted: %q", raw)
	}

	p := &store.Provider{ID: "p1", Code: "stub", Name: "stub", Enabled: true, Credentials: map[string]string{"api_key": "topsecret"}}
	if err := st.UpsertProvider(ctx, p); err != nil {
		t.Fatalf("UpsertProvider: %v", err)
	}
	got, err := st.GetProvider(ctx, "p1")
	if err != nil || got.Credentials["api_key"] != "topsecret" {
		t.Fatalf("GetProvider = %+v err=%v", got, err)
	}
	if err := st.DB().QueryRowContext(ctx, `SELECT credentials_json FROM providers WHERE id = 'p1'`).Scan(&raw); err != nil {
		t.Fatalf("raw provider scan: %v", err)
	}
	if !strings.HasPrefix(raw, "enc:v1:") {
		t.Fatalf("credentials not encrypted: %q", raw)
	}

	px := &store.Proxy{ID: "x1", Name: "px", Enabled: true, Type: "http", Host: "127.0.0.1", Port: "8080", Password: "hunter2"}
	if err := st.UpsertProxy(ctx, px); err != nil {
		t.Fatalf("UpsertProxy: %v", err)
	}
	gotPx, err := st.GetProxy(ctx, "x1")
	if err != nil || gotPx.Password != "hunter2" {
		t.Fatalf("GetProxy = %+v err=%v", gotPx, err)
	}
	if err := st.DB().QueryRowContext(ctx, `SELECT password FROM proxies WHERE id = 'x1'`).Scan(&raw); err != nil {
		t.Fatalf("raw proxy scan: %v", err)
	}
	if !strings.HasPrefix(raw, "enc:v1:") {
		t.Fatalf("proxy password not encrypted: %q", raw)
	}
}
