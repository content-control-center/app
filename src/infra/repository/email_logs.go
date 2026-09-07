package repository

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/domain/models"
)

// EmailLogRepository is the append-only send-audit surface (CON-154 §7).
type EmailLogRepository interface {
	Insert(ctx context.Context, l *models.EmailLog) error
	// UpdateStatusByProviderMessageID updates the row a delivery webhook refers
	// to (matched on the Resend message id). Returns whether a row matched.
	UpdateStatusByProviderMessageID(ctx context.Context, providerMessageID string, status models.EmailLogStatus) (bool, error)
	// DeleteOlderThan drops rows created before cutoff (retention sweep),
	// mirroring PostLogRepository. Returns the number removed.
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
	// ExistsByIdempotencyKey reports whether any log row already carries this
	// idempotency key (CON-219). The connection-expiry sweep calls it before
	// enqueuing so a multi-day expiry window swept many times notifies each
	// (account, stage, expiry, owner) at most once — durable dedupe that outlives
	// the provider's own idempotency-key TTL. The key is globally unique (partial
	// unique index), so no tenant scope is needed.
	ExistsByIdempotencyKey(ctx context.Context, key string) (bool, error)
}

type emailLogRepository struct {
	db *bun.DB
}

// NewEmailLogRepository returns a Bun-backed EmailLogRepository.
func NewEmailLogRepository(db *bun.DB) EmailLogRepository {
	return &emailLogRepository{db: db}
}

func (r *emailLogRepository) Insert(ctx context.Context, l *models.EmailLog) error {
	_, err := r.db.NewInsert().Model(l).Exec(ctx)
	return err
}

func (r *emailLogRepository) UpdateStatusByProviderMessageID(ctx context.Context, providerMessageID string, status models.EmailLogStatus) (bool, error) {
	if providerMessageID == "" {
		return false, nil
	}
	res, err := r.db.NewUpdate().
		Model((*models.EmailLog)(nil)).
		Set("status = ?", status).
		Set("updated_at = now()").
		Where("provider_message_id = ?", providerMessageID).
		Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *emailLogRepository) ExistsByIdempotencyKey(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	return r.db.NewSelect().
		Model((*models.EmailLog)(nil)).
		Where("idempotency_key = ?", key).
		Exists(ctx)
}

func (r *emailLogRepository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.NewDelete().
		Model((*models.EmailLog)(nil)).
		Where("created_at < ?", cutoff).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
