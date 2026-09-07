package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/domain/models"
)

// PlatformRepository defines all persistence operations for the Platform domain.
type PlatformRepository interface {
	List(ctx context.Context) ([]models.Platform, error)
	Create(ctx context.Context, platform *models.Platform) error
	GetByID(ctx context.Context, id string) (*models.Platform, error)
	Update(ctx context.Context, platform *models.Platform) error
	Delete(ctx context.Context, id string) (bool, error)
}

type platformRepository struct {
	db *bun.DB
}

// NewPlatformRepository returns a Bun-backed PlatformRepository.
func NewPlatformRepository(db *bun.DB) PlatformRepository {
	return &platformRepository{db: db}
}

func (r *platformRepository) List(ctx context.Context) ([]models.Platform, error) {
	var platforms []models.Platform
	if err := r.db.NewSelect().Model(&platforms).OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	return platforms, nil
}

func (r *platformRepository) Create(ctx context.Context, platform *models.Platform) error {
	_, err := r.db.NewInsert().Model(platform).Exec(ctx)
	return err
}

func (r *platformRepository) GetByID(ctx context.Context, id string) (*models.Platform, error) {
	platform := new(models.Platform)
	err := r.db.NewSelect().Model(platform).Where("pl.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return platform, nil
}

func (r *platformRepository) Update(ctx context.Context, platform *models.Platform) error {
	_, err := r.db.NewUpdate().Model(platform).WherePK().Exec(ctx)
	return err
}

func (r *platformRepository) Delete(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewDelete().Model((*models.Platform)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
