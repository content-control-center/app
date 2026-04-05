package database

import (
	"context"
	"embed"
	"fmt"
	"log"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

//go:embed migrations/*.sql
var sqlMigrations embed.FS

// Migrate applies all pending SQL migrations on every startup.
// Applied migrations are recorded in the bun_migrations table and never re-run.
func Migrate(ctx context.Context, db *bun.DB) error {
	migrations := migrate.NewMigrations()
	if err := migrations.Discover(sqlMigrations); err != nil {
		return fmt.Errorf("discover migrations: %w", err)
	}

	migrator := migrate.NewMigrator(db, migrations)

	if err := migrator.Init(ctx); err != nil {
		return fmt.Errorf("init migrator: %w", err)
	}

	group, err := migrator.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	if group.IsZero() {
		log.Println("migrations: no new migrations to apply")
		return nil
	}

	log.Printf("migrations: applied migration group %s", group)
	return nil
}
