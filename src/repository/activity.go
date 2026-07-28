package repository

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
)

// ActivityRepository persists activity_events in the isolated analytics
// database (CON-125). It is constructed with the analytics *bun.DB, not the
// main pool. Writes come from the async activity.Recorder in a system context
// (tenant pre-set per row). A read/query surface is deferred to the consumers
// (CON-119 / CON-120); v1 is collection-only.
type ActivityRepository interface {
	// Insert writes a batch of events. A nil/empty batch is a no-op.
	Insert(ctx context.Context, events []*models.ActivityEvent) error
}

type activityRepository struct {
	db *bun.DB
}

// NewActivityRepository builds an ActivityRepository. Pass the analytics *bun.DB.
func NewActivityRepository(db *bun.DB) ActivityRepository {
	return &activityRepository{db: db}
}

func (r *activityRepository) Insert(ctx context.Context, events []*models.ActivityEvent) error {
	if len(events) == 0 {
		return nil
	}
	_, err := r.db.NewInsert().Model(&events).Exec(ctx)
	return err
}
