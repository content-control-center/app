package repository

import (
	"context"
	"database/sql"
	"encoding/json"
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

// Delete removes the asset and, in the same transaction, scrubs its id from
// every campaign.asset_ids and post.used_asset_ids (CON-214). Without the
// scrub a deleted asset lingers as a dangling reference that later hard-fails
// content generation ("asset %q not found").
//
// In a tenant context the TenantScoped hooks scope every statement to the
// caller's tenant. Under a system context those hooks add no predicate, so we
// capture the deleted asset's tenant up front and scope the jsonb-scrub updates
// to it explicitly — a system-context delete can never touch another tenant's
// campaigns or posts.
func (r *assetRepository) Delete(ctx context.Context, id string) (bool, error) {
	var deleted bool
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Read the owning tenant before the delete so the cleanup updates below
		// can be scoped to it even when no tenant is in context (system context).
		var tenantID string
		err := tx.NewSelect().
			Model((*models.Asset)(nil)).
			Column("tenant_id").
			Where("id = ?", id).
			Scan(ctx, &tenantID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil // asset not found → nothing to delete or scrub
		}
		if err != nil {
			return err
		}

		res, err := tx.NewDelete().Model((*models.Asset)(nil)).Where("id = ?", id).Exec(ctx)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		deleted = n > 0
		if !deleted {
			return nil // nothing removed → no dangling references to scrub
		}

		// jsonb array element removal: `col - <text>` drops matching elements.
		// The @> guard limits each update to rows that actually reference the id;
		// the needle is the id encoded as a JSON scalar so containment matches an
		// array element (e.g. asset_ids @> '"abc"'). The tenant_id predicate keeps
		// the scrub within the deleted asset's tenant under a system context.
		needle, err := json.Marshal(id)
		if err != nil {
			return err
		}
		if _, err := tx.NewUpdate().
			Model((*models.Campaign)(nil)).
			Set("asset_ids = asset_ids - ?", id).
			Where("tenant_id = ?", tenantID).
			Where("asset_ids @> ?::jsonb", string(needle)).
			Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewUpdate().
			Model((*models.Post)(nil)).
			Set("used_asset_ids = used_asset_ids - ?", id).
			Where("tenant_id = ?", tenantID).
			Where("used_asset_ids @> ?::jsonb", string(needle)).
			Exec(ctx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return deleted, nil
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
