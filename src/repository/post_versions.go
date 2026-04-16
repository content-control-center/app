package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/models"
)

// PostVersionRepository handles persistence of post description version snapshots.
type PostVersionRepository interface {
	Create(ctx context.Context, version *models.PostVersion) error
	ListByPostID(ctx context.Context, postID string) ([]models.PostVersion, error)
	GetLatestByPostID(ctx context.Context, postID string) (*models.PostVersion, error)
	CountByPostID(ctx context.Context, postID string) (int, error)
}

type postVersionRepository struct {
	db *bun.DB
}

func NewPostVersionRepository(db *bun.DB) PostVersionRepository {
	return &postVersionRepository{db: db}
}

func (r *postVersionRepository) Create(ctx context.Context, version *models.PostVersion) error {
	_, err := r.db.NewInsert().Model(version).Exec(ctx)
	return err
}

func (r *postVersionRepository) ListByPostID(ctx context.Context, postID string) ([]models.PostVersion, error) {
	var versions []models.PostVersion
	err := r.db.NewSelect().
		Model(&versions).
		Where("pv.post_id = ?", postID).
		OrderExpr("pv.version_number ASC").
		Scan(ctx)
	return versions, err
}

func (r *postVersionRepository) GetLatestByPostID(ctx context.Context, postID string) (*models.PostVersion, error) {
	version := new(models.PostVersion)
	err := r.db.NewSelect().
		Model(version).
		Where("pv.post_id = ?", postID).
		OrderExpr("pv.version_number DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return version, nil
}

func (r *postVersionRepository) CountByPostID(ctx context.Context, postID string) (int, error) {
	return r.db.NewSelect().
		Model((*models.PostVersion)(nil)).
		Where("post_id = ?", postID).
		Count(ctx)
}
