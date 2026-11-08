package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/crypt"
	"github.com/liberide/serpent-seek/internal/store/postgres"
	"github.com/liberide/serpent-seek/internal/store/sqlite"
	"github.com/liberide/serpent-seek/internal/store/sqlstore"
)

// rotateKey re-encrypts secrets at rest from ENCRYPTION_KEY to
// ENCRYPTION_KEY_NEW. The service must be stopped while it runs.
func rotateKey(ctx context.Context, out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	newMaterial := strings.TrimSpace(os.Getenv("ENCRYPTION_KEY_NEW"))
	oldCipher, err := crypt.New(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("old key: %w", err)
	}
	newCipher, err := crypt.New(newMaterial)
	if err != nil {
		return fmt.Errorf("ENCRYPTION_KEY_NEW: %w", err)
	}
	if !oldCipher.Enabled() && !newCipher.Enabled() {
		return fmt.Errorf("neither ENCRYPTION_KEY nor ENCRYPTION_KEY_NEW is set: nothing to rotate")
	}

	var storeImpl *sqlstore.Store
	switch cfg.StorageDriver {
	case "postgres":
		storeImpl, err = postgres.Open(ctx, cfg.DatabaseURL)
	default:
		storeImpl, err = sqlite.Open(ctx, cfg.SQLitePath)
	}
	if err != nil {
		return err
	}
	defer func() { _ = storeImpl.Close() }()
	if err := storeImpl.Migrate(ctx); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	stats, err := rotateSecrets(ctx, storeImpl, oldCipher, newCipher)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "rotated at-rest secrets: %d settings, %d providers, %d proxies (%s)\n",
		stats.Settings, stats.Providers, stats.Proxies, describeCipher(newCipher))
	return nil
}

// rotateStats counts re-encrypted rows by kind.
type rotateStats struct {
	Settings  int
	Providers int
	Proxies   int
}

// rotateSecrets re-encrypts every protected column from oldCipher to newCipher.
func rotateSecrets(ctx context.Context, st *sqlstore.Store, oldCipher, newCipher *crypt.Cipher) (rotateStats, error) {
	var stats rotateStats

	st.SetCipher(oldCipher)
	settings, err := st.AllSettings(ctx)
	if err != nil {
		return stats, fmt.Errorf("read settings: %w", err)
	}
	providers, err := st.ListProviders(ctx)
	if err != nil {
		return stats, fmt.Errorf("read providers: %w", err)
	}
	proxies, err := st.ListProxies(ctx)
	if err != nil {
		return stats, fmt.Errorf("read proxies: %w", err)
	}

	// A value that still has the encryption marker after decryption means the
	// old key is wrong; abort before writing to avoid double-encrypting secrets.
	badLeftovers := 0
	for _, v := range settings {
		if strings.HasPrefix(v, crypt.EncryptedPrefix) {
			badLeftovers++
		}
	}
	for _, p := range providers {
		for _, v := range p.Credentials {
			if strings.HasPrefix(v, crypt.EncryptedPrefix) {
				badLeftovers++
			}
		}
	}
	for _, p := range proxies {
		if strings.HasPrefix(p.Password, crypt.EncryptedPrefix) {
			badLeftovers++
		}
	}
	if badLeftovers > 0 {
		return stats, fmt.Errorf("%d stored value(s) could not be decrypted: ENCRYPTION_KEY does not match the key the data was written with; aborting without changes", badLeftovers)
	}

	st.SetCipher(newCipher)
	stats.Settings = len(settings)
	for k, v := range settings {
		if err := st.SetSetting(ctx, k, v); err != nil {
			return stats, fmt.Errorf("rewrite setting %q: %w", k, err)
		}
	}
	for _, p := range providers {
		if err := st.UpsertProvider(ctx, p); err != nil {
			return stats, fmt.Errorf("rewrite provider %q: %w", p.ID, err)
		}
		stats.Providers++
	}
	for _, p := range proxies {
		if err := st.UpsertProxy(ctx, p); err != nil {
			return stats, fmt.Errorf("rewrite proxy %q: %w", p.ID, err)
		}
		stats.Proxies++
	}
	return stats, nil
}

func describeCipher(c *crypt.Cipher) string {
	if !c.Enabled() {
		return "encryption disabled"
	}
	return "new key applied"
}
