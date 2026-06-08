package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
)

// AssetRepository defines all persistence operations for the Asset domain.
type AssetRepository interface {
	List(ctx context.Context) ([]models.Asset, error)
	Create(ctx context.Context, asset *models.Asset) error
	GetByID(ctx context.Context, id string) (*models.Asset, error)
	Update(ctx context.Context, asset *models.Asset) error
	UpdateStatus(ctx context.Context, id, status string) error
	Delete(ctx context.Context, id string) (bool, error)
}

type assetRepository struct {
	db       *bun.DB
	tagRepo  TagRepository
	fileRepo AssetFileRepository
}

// NewAssetRepository returns a Bun-backed AssetRepository. fileRepo may be
// nil, in which case asset.File hydration is skipped.
func NewAssetRepository(db *bun.DB, tagRepo TagRepository, fileRepo AssetFileRepository) AssetRepository {
	return &assetRepository{db: db, tagRepo: tagRepo, fileRepo: fileRepo}
}

func (r *assetRepository) List(ctx context.Context) ([]models.Asset, error) {
	var assets []models.Asset
	if err := r.db.NewSelect().Model(&assets).OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	if err := r.hydrateTags(ctx, assets); err != nil {
		return nil, err
	}
	if err := r.hydrateFiles(ctx, assets); err != nil {
		return nil, err
	}
	return assets, nil
}

func (r *assetRepository) Create(ctx context.Context, asset *models.Asset) error {
	_, err := r.db.NewInsert().Model(asset).Exec(ctx)
	return err
}

func (r *assetRepository) GetByID(ctx context.Context, id string) (*models.Asset, error) {
	asset := new(models.Asset)
	err := r.db.NewSelect().Model(asset).Where("a.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	assets := []models.Asset{*asset}
	if err := r.hydrateTags(ctx, assets); err != nil {
		return nil, err
	}
	if err := r.hydrateFiles(ctx, assets); err != nil {
		return nil, err
	}
	return &assets[0], nil
}

func (r *assetRepository) Update(ctx context.Context, asset *models.Asset) error {
	_, err := r.db.NewUpdate().Model(asset).WherePK().Exec(ctx)
	return err
}

func (r *assetRepository) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := r.db.NewUpdate().
		Model((*models.Asset)(nil)).
		Set("status = ?", status).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (r *assetRepository) Delete(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewDelete().Model((*models.Asset)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *assetRepository) hydrateTags(ctx context.Context, assets []models.Asset) error {
	return HydrateTags(ctx, r.tagRepo, assets,
		func(p models.Asset) []string { return p.TagIDs },
		func(p *models.Asset) *[]models.Tag { return &p.Tags },
	)
}

func (r *assetRepository) hydrateFiles(ctx context.Context, assets []models.Asset) error {
	if r.fileRepo == nil || len(assets) == 0 {
		return nil
	}
	ids := make([]string, 0, len(assets))
	for _, a := range assets {
		ids = append(ids, a.ID)
	}
	byAsset, err := r.fileRepo.ListByAssetIDs(ctx, ids)
	if err != nil {
		return err
	}
	for i := range assets {
		if f, ok := byAsset[assets[i].ID]; ok {
			assets[i].File = f
		}
	}
	return nil
}
