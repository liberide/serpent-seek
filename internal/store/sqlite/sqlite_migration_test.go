package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/liberide/serpent-seek/internal/store"
)

// TestMigrationsApplyAndAreIdempotent guards the goose migration chain and the
// schema introduced by the latest migrations.
func TestMigrationsApplyAndAreIdempotent(t *testing.T) {
	ctx := context.Background()
	driver, err := Open(ctx, filepath.Join(t.TempDir(), "migrate.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	if err := driver.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	version, err := store.MigrationVersion(ctx, driver.DB(), "sqlite3")
	if err != nil {
		t.Fatalf("MigrationVersion: %v", err)
	}
	if version < 12 {
		t.Fatalf("expected schema version >= 12, got %d", version)
	}

	// Re-running must be a no-op.
	if err := driver.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	// Columns introduced by the latest migrations must exist.
	for _, q := range []string{
		`SELECT chain_id FROM api_keys LIMIT 1`,
		`SELECT permissions FROM api_keys LIMIT 1`,
		`SELECT subject, day, count FROM usage_daily LIMIT 1`,
		`SELECT action, target, details FROM audit_log LIMIT 1`,
	} {
		if _, err := driver.DB().ExecContext(ctx, q); err != nil {
			t.Fatalf("schema check %q: %v", q, err)
		}
	}
}
