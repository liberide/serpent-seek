package main

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liberide/serpent-seek/internal/crypt"
	"github.com/liberide/serpent-seek/internal/store"
	"github.com/liberide/serpent-seek/internal/store/sqlite"
	"github.com/liberide/serpent-seek/internal/store/sqlstore"
)

func testCipher(t *testing.T, seedByte byte) *crypt.Cipher {
	t.Helper()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = seedByte
	}
	c, err := crypt.New(hex.EncodeToString(raw))
	if err != nil {
		t.Fatalf("crypt.New: %v", err)
	}
	return c
}

func newEncryptedStore(t *testing.T, cipher *crypt.Cipher) *sqlstore.Store {
	t.Helper()
	ctx := context.Background()
	st, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st.SetCipher(cipher)
	return st
}

// seedSecrets writes one value for each encrypted column kind.
func seedSecrets(t *testing.T, ctx context.Context, st store.Storage) {
	t.Helper()
	if err := st.SetSetting(ctx, "searxng_api_key", "setting-secret"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if err := st.UpsertProvider(ctx, &store.Provider{
		ID: uuid.NewString(), Code: "searxng", Name: "S", Enabled: true,
		Credentials: map[string]string{"api_key": "provider-secret"}, Params: map[string]string{},
	}); err != nil {
		t.Fatalf("UpsertProvider: %v", err)
	}
	if err := st.UpsertProxy(ctx, &store.Proxy{
		ID: uuid.NewString(), Name: "p", Type: "http", Host: "127.0.0.1", Port: "8080",
		Username: "u", Password: "proxy-secret",
	}); err != nil {
		t.Fatalf("UpsertProxy: %v", err)
	}
}

func readBack(t *testing.T, ctx context.Context, st *sqlstore.Store, cipher *crypt.Cipher) {
	t.Helper()
	st.SetCipher(cipher)

	v, err := st.GetSetting(ctx, "searxng_api_key")
	if err != nil || v != "setting-secret" {
		t.Fatalf("setting = %q (err %v), want setting-secret", v, err)
	}
	providers, err := st.ListProviders(ctx)
	if err != nil || len(providers) != 1 {
		t.Fatalf("ListProviders: %v (%d rows)", err, len(providers))
	}
	if got := providers[0].Credentials["api_key"]; got != "provider-secret" {
		t.Fatalf("provider credential = %q, want provider-secret", got)
	}
	proxies, err := st.ListProxies(ctx)
	if err != nil || len(proxies) != 1 {
		t.Fatalf("ListProxies: %v (%d rows)", err, len(proxies))
	}
	if got := proxies[0].Password; got != "proxy-secret" {
		t.Fatalf("proxy password = %q, want proxy-secret", got)
	}
}

func TestRotateSecretsKeyToKey(t *testing.T) {
	ctx := context.Background()
	oldCipher := testCipher(t, 0xAA)
	newCipher := testCipher(t, 0xBB)
	st := newEncryptedStore(t, oldCipher)
	seedSecrets(t, ctx, st)

	stats, err := rotateSecrets(ctx, st, oldCipher, newCipher)
	if err != nil {
		t.Fatalf("rotateSecrets: %v", err)
	}
	if stats.Settings < 1 || stats.Providers != 1 || stats.Proxies != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	readBack(t, ctx, st, newCipher)
}

func TestRotateSecretsDisablesEncryption(t *testing.T) {
	ctx := context.Background()
	oldCipher := testCipher(t, 0xAA)
	noCipher, err := crypt.New("")
	if err != nil {
		t.Fatalf("crypt.New(\"\"): %v", err)
	}
	st := newEncryptedStore(t, oldCipher)
	seedSecrets(t, ctx, st)

	if _, err := rotateSecrets(ctx, st, oldCipher, noCipher); err != nil {
		t.Fatalf("rotateSecrets: %v", err)
	}
	readBack(t, ctx, st, noCipher)
}

func TestRotateSecretsAbortsOnWrongOldKey(t *testing.T) {
	ctx := context.Background()
	realOld := testCipher(t, 0xAA)
	wrongOld := testCipher(t, 0xCC)
	newCipher := testCipher(t, 0xBB)
	st := newEncryptedStore(t, realOld)
	seedSecrets(t, ctx, st)

	_, err := rotateSecrets(ctx, st, wrongOld, newCipher)
	if err == nil || !strings.Contains(err.Error(), "could not be decrypted") {
		t.Fatalf("expected a decryption-guard error, got: %v", err)
	}
	// The real old key must still decrypt the data.
	readBack(t, ctx, st, realOld)
}
