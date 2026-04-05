package database

import (
	"database/sql"
	"fmt"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/extra/bundebug"
	_ "modernc.org/sqlite"
)

func New(dsn string, debug bool) (*bun.DB, error) {
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite works best with a single connection.
	sqldb.SetMaxOpenConns(1)
	sqldb.SetMaxIdleConns(1)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	if debug {
		db.AddQueryHook(bundebug.NewQueryHook(bundebug.WithVerbose(true)))
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}
