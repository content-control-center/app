package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
)

// CampaignRepository defines all persistence operations for the Campaign domain.
type CampaignRepository interface {
	List(ctx context.Context) ([]models.Campaign, error)
	Create(ctx context.Context, campaign *models.Campaign) error
	GetByID(ctx context.Context, id string) (*models.Campaign, error)
	Update(ctx context.Context, campaign *models.Campaign) error
	Delete(ctx context.Context, id string) (bool, error)
	// AddAssetIDs / RemoveAssetID are the CON-233 membership write path: they
	// mutate only campaigns.asset_ids, in one atomic UPDATE, so attaching or
	// detaching one document no longer round-trips the whole record and
	// concurrent adds of different ids don't clobber each other. Both return the
	// hydrated campaign, or sql.ErrNoRows if it doesn't exist (in this tenant).
	AddAssetIDs(ctx context.Context, id string, assetIDs []string) (*models.Campaign, error)
	RemoveAssetID(ctx context.Context, id, assetID string) (*models.Campaign, error)
}

type campaignRepository struct {
	db               *bun.DB
	tagRepo          TagRepository
	platformRepo     PlatformRepository
	campaignTypeRepo CampaignTypeRepository
}

// NewCampaignRepository returns a Bun-backed CampaignRepository.
func NewCampaignRepository(
	db *bun.DB,
	tagRepo TagRepository,
	platformRepo PlatformRepository,
	campaignTypeRepo CampaignTypeRepository,
) CampaignRepository {
	return &campaignRepository{
		db:               db,
		tagRepo:          tagRepo,
		platformRepo:     platformRepo,
		campaignTypeRepo: campaignTypeRepo,
	}
}

func (r *campaignRepository) List(ctx context.Context) ([]models.Campaign, error) {
	var campaigns []models.Campaign
	if err := r.db.NewSelect().Model(&campaigns).OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	if err := r.hydrateTags(ctx, campaigns); err != nil {
		return nil, err
	}
	if err := r.hydratePlatforms(ctx, campaigns); err != nil {
		return nil, err
	}
	if err := r.hydrateCampaignTypes(ctx, campaigns); err != nil {
		return nil, err
	}
	return campaigns, nil
}

func (r *campaignRepository) Create(ctx context.Context, campaign *models.Campaign) error {
	_, err := r.db.NewInsert().Model(campaign).Exec(ctx)
	return err
}

func (r *campaignRepository) GetByID(ctx context.Context, id string) (*models.Campaign, error) {
	campaign := new(models.Campaign)
	err := r.db.NewSelect().Model(campaign).Where("c.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	campaigns := []models.Campaign{*campaign}
	if err := r.hydrateTags(ctx, campaigns); err != nil {
		return nil, err
	}
	if err := r.hydratePlatforms(ctx, campaigns); err != nil {
		return nil, err
	}
	if err := r.hydrateCampaignTypes(ctx, campaigns); err != nil {
		return nil, err
	}
	return &campaigns[0], nil
}

func (r *campaignRepository) Update(ctx context.Context, campaign *models.Campaign) error {
	_, err := r.db.NewUpdate().Model(campaign).WherePK().Exec(ctx)
	return err
}

// AddAssetIDs unions assetIDs into campaigns.asset_ids atomically (CON-233).
// An empty input is a no-op read: it still validates existence and returns the
// current campaign, so the caller gets a 404 for a missing id without a
// pointless write. The TenantScoped hook scopes the UPDATE to the caller's
// tenant, so a foreign or unknown id matches zero rows and surfaces as
// sql.ErrNoRows.
func (r *campaignRepository) AddAssetIDs(ctx context.Context, id string, assetIDs []string) (*models.Campaign, error) {
	if len(assetIDs) == 0 {
		return r.GetByID(ctx, id)
	}
	payload, err := json.Marshal(assetIDs)
	if err != nil {
		return nil, err
	}
	res, err := r.db.NewUpdate().
		Model((*models.Campaign)(nil)).
		Set(jsonbIDUnionSet("asset_ids"), string(payload)).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, sql.ErrNoRows
	}
	return r.GetByID(ctx, id)
}

// RemoveAssetID drops assetID from campaigns.asset_ids atomically (CON-233).
// Removal is idempotent — an absent id still matches the row (so it is not a
// 404) and leaves the set unchanged.
func (r *campaignRepository) RemoveAssetID(ctx context.Context, id, assetID string) (*models.Campaign, error) {
	res, err := r.db.NewUpdate().
		Model((*models.Campaign)(nil)).
		Set(jsonbIDRemoveSet("asset_ids"), assetID).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, sql.ErrNoRows
	}
	return r.GetByID(ctx, id)
}

func (r *campaignRepository) Delete(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewDelete().Model((*models.Campaign)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *campaignRepository) hydratePlatforms(ctx context.Context, campaigns []models.Campaign) error {
	for i := range campaigns {
		campaigns[i].Platforms = []models.Platform{}
	}
	ids := collectIDsFlat(campaigns, func(c models.Campaign) []string {
		out := make([]string, len(c.TargetPlatforms))
		for i, tp := range c.TargetPlatforms {
			out[i] = tp.ID
		}
		return out
	})
	byID, err := fetchByIDs(ctx, r.db, ids, func(p *models.Platform) string { return p.ID })
	if err != nil {
		return err
	}
	for i, c := range campaigns {
		for _, tp := range c.TargetPlatforms {
			if p, ok := byID[tp.ID]; ok {
				campaigns[i].Platforms = append(campaigns[i].Platforms, *p)
			}
		}
	}
	return nil
}

func (r *campaignRepository) hydrateCampaignTypes(ctx context.Context, campaigns []models.Campaign) error {
	ids := make([]string, 0, len(campaigns))
	for _, c := range campaigns {
		ids = append(ids, c.CampaignTypeID)
	}
	byID, err := r.campaignTypeRepo.GetByIDs(ctx, ids)
	if err != nil {
		return err
	}
	for i, c := range campaigns {
		if ct, ok := byID[c.CampaignTypeID]; ok {
			campaigns[i].CampaignType = ct
		}
	}
	return nil
}

func (r *campaignRepository) hydrateTags(ctx context.Context, campaigns []models.Campaign) error {
	return HydrateTags(ctx, r.tagRepo, campaigns,
		func(c models.Campaign) []string { return c.TagIDs },
		func(c *models.Campaign) *[]models.Tag { return &c.Tags },
	)
}
