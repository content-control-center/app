package repository

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
)

// SocialAccountRepository is the persistence surface for the Zernio
// integration's local account mirror (CON-62). The reconciler uses
// the batch operations under a single transaction (ApplyPlan) so each
// sync tick is atomic.
type SocialAccountRepository interface {
	// ListAll returns every row, including soft-deleted, scoped to a
	// profile. The reconciler needs the soft-deleted rows to detect
	// "previously disconnected, now reappearing" → revive.
	ListAll(ctx context.Context, profileID string) ([]models.SocialAccount, error)

	// ListActive returns only rows where deleted_at IS NULL, scoped to
	// a profile. Used by GET /api/integrations/zernio/accounts.
	ListActive(ctx context.Context, profileID string) ([]models.SocialAccount, error)

	// ListActiveByPlatform returns the active (deleted_at IS NULL) accounts
	// for one Zernio platform slug (e.g. "linkedin") under a profile,
	// oldest-connected first. Backs the CON-150 auto-vs-require account
	// selection: 0 → no account, 1 → auto-select, ≥2 → require a choice.
	ListActiveByPlatform(ctx context.Context, profileID, platform string) ([]models.SocialAccount, error)

	// GetActive returns the single active account with the given id under a
	// profile, or sql.ErrNoRows when it is missing, soft-deleted, or belongs
	// to another profile. Backs validation of an explicit CON-150 selection.
	GetActive(ctx context.Context, profileID, id string) (*models.SocialAccount, error)

	// ApplyPlan persists the reconciler's three sets in a single
	// transaction. Inserts and revives upsert on the primary key
	// (an existing soft-deleted row is brought back to life by
	// clearing deleted_at and refreshing fields). Soft-deletes set
	// deleted_at to the supplied moment.
	ApplyPlan(ctx context.Context, upserts []models.SocialAccount, softDeleteIDs []string, now time.Time) error
}

type socialAccountRepository struct {
	db *bun.DB
}

func NewSocialAccountRepository(db *bun.DB) SocialAccountRepository {
	return &socialAccountRepository{db: db}
}

func (r *socialAccountRepository) ListAll(ctx context.Context, profileID string) ([]models.SocialAccount, error) {
	var out []models.SocialAccount
	if err := r.db.NewSelect().Model(&out).
		Where("sa.profile_id = ?", profileID).
		OrderExpr("connected_at ASC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *socialAccountRepository) ListActive(ctx context.Context, profileID string) ([]models.SocialAccount, error) {
	var out []models.SocialAccount
	if err := r.db.NewSelect().Model(&out).
		Where("sa.profile_id = ?", profileID).
		Where("sa.deleted_at IS NULL").
		OrderExpr("connected_at ASC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *socialAccountRepository) ListActiveByPlatform(ctx context.Context, profileID, platform string) ([]models.SocialAccount, error) {
	var out []models.SocialAccount
	if err := r.db.NewSelect().Model(&out).
		Where("sa.profile_id = ?", profileID).
		Where("sa.platform = ?", platform).
		Where("sa.deleted_at IS NULL").
		OrderExpr("connected_at ASC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *socialAccountRepository) GetActive(ctx context.Context, profileID, id string) (*models.SocialAccount, error) {
	acc := new(models.SocialAccount)
	if err := r.db.NewSelect().Model(acc).
		Where("sa.id = ?", id).
		Where("sa.profile_id = ?", profileID).
		Where("sa.deleted_at IS NULL").
		Scan(ctx); err != nil {
		return nil, err
	}
	return acc, nil
}

func (r *socialAccountRepository) ApplyPlan(
	ctx context.Context,
	upserts []models.SocialAccount,
	softDeleteIDs []string,
	now time.Time,
) error {
	if len(upserts) == 0 && len(softDeleteIDs) == 0 {
		return nil
	}
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if len(upserts) > 0 {
			// ON CONFLICT covers both inserts and revives — a row
			// previously soft-deleted is brought back by clearing
			// deleted_at and refreshing the mutable fields.
			if _, err := tx.NewInsert().Model(&upserts).
				On("CONFLICT (id) DO UPDATE").
				Set("platform = EXCLUDED.platform").
				Set("profile_id = EXCLUDED.profile_id").
				Set("username = EXCLUDED.username").
				Set("display_name = EXCLUDED.display_name").
				Set("avatar_url = EXCLUDED.avatar_url").
				Set("is_active = EXCLUDED.is_active").
				Set("raw_json = EXCLUDED.raw_json").
				Set("last_synced_at = EXCLUDED.last_synced_at").
				Set("deleted_at = NULL").
				Exec(ctx); err != nil {
				return err
			}
		}
		if len(softDeleteIDs) > 0 {
			if _, err := tx.NewUpdate().Model((*models.SocialAccount)(nil)).
				Set("deleted_at = ?", now).
				Where("id IN (?)", bun.In(softDeleteIDs)).
				Where("deleted_at IS NULL").
				Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}
