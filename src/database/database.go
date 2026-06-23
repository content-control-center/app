package database

import (
	"database/sql"
	"fmt"

	// Registers the "pgx" database/sql driver. bun runs on the resulting
	// *sql.DB; the same pool (exposed as db.DB) is shared with the River job
	// queue so bun and River draw from one connection pool.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/extra/bundebug"
)

// Default connection-pool sizing. Postgres removes SQLite's single-writer
// ceiling, so we open a real pool. cmd/server overrides these from config.
const (
	defaultMaxOpenConns = 25
	defaultMaxIdleConns = 5
)

// New opens a Postgres connection pool via the pgx stdlib driver and wraps it
// with bun using the Postgres dialect.
func New(dsn string, debug bool) (*bun.DB, error) {
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	sqldb.SetMaxOpenConns(defaultMaxOpenConns)
	sqldb.SetMaxIdleConns(defaultMaxIdleConns)

	db := bun.NewDB(sqldb, pgdialect.New())

	if debug {
		db.AddQueryHook(bundebug.NewQueryHook(bundebug.WithVerbose(true)))
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}
