// Package sqlite opens the SQLite storage driver (pure Go, no CGO).
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver

	"github.com/liberide/serpent-seek/internal/store"
	"github.com/liberide/serpent-seek/internal/store/sqlstore"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open opens (creating if needed) the SQLite database and returns a Storage.
func Open(ctx context.Context, path string) (*sqlstore.Store, error) {
	var dsn string
	if path == ":memory:" || path == "" {
		dsn = "file::memory:?cache=shared&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	} else {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("sqlite: create data dir: %w", err)
		}
		dsn = "file:" + path +
			"?_pragma=busy_timeout(5000)" +
			"&_pragma=journal_mode(WAL)" +
			"&_pragma=foreign_keys(ON)" +
			"&_pragma=synchronous(NORMAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	// A single connection avoids SQLITE_BUSY between concurrent writers.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: ping: %w", err)
	}
	st := sqlstore.New(db, "sqlite")
	st.SetMigrate(func(ctx context.Context) error {
		return store.RunMigrations(ctx, db, "sqlite3", migrationsFS, "migrations")
	})
	return st, nil
}
