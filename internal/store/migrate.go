// Package store: shared migration runner.
package store

import (
	"context"
	"database/sql"
	"io/fs"

	"github.com/pressly/goose/v3"
)

// RunMigrations applies all pending goose migrations from an embedded FS.
// driver is the goose dialect name ("sqlite3" or "postgres").
func RunMigrations(ctx context.Context, db *sql.DB, driver string, fsys fs.FS, dir string) error {
	goose.SetBaseFS(fsys)
	if err := goose.SetDialect(driver); err != nil {
		return err
	}
	goose.SetLogger(goose.NopLogger())
	return goose.UpContext(ctx, db, dir)
}

// MigrationVersion returns the current applied goose version.
func MigrationVersion(ctx context.Context, db *sql.DB, driver string) (int64, error) {
	if err := goose.SetDialect(driver); err != nil {
		return 0, err
	}
	return goose.GetDBVersionContext(ctx, db)
}
