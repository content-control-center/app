package database

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/content-control-center/app/internal/models"
)

// Migrate runs auto-migrations for all registered models.
// Tables are created if they do not exist; existing columns are not dropped.
func Migrate(ctx context.Context, db *bun.DB) error {
	tables := []interface{}{
		(*models.User)(nil),
	}

	for _, model := range tables {
		if _, err := db.NewCreateTable().
			Model(model).
			IfNotExists().
			Exec(ctx); err != nil {
			return fmt.Errorf("migrate %T: %w", model, err)
		}
	}

	return nil
}
