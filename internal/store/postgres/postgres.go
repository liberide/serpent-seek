// Package postgres opens the PostgreSQL storage driver using pgx/v5.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql adapter for pgx

	"github.com/liberide/serpent-seek/internal/store"
	"github.com/liberide/serpent-seek/internal/store/sqlstore"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open connects to PostgreSQL and returns a Storage.
func Open(ctx context.Context, url string) (*sqlstore.Store, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	st := sqlstore.New(db, "postgres")
	st.SetMigrate(func(ctx context.Context) error {
		return store.RunMigrations(ctx, db, "postgres", migrationsFS, "migrations")
	})
	return st, nil
}
