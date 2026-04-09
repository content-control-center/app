package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/models"
)

// CampaignRepository defines all persistence operations for the Campaign domain.
type CampaignRepository interface {
	List(ctx context.Context) ([]models.Campaign, error)
	Create(ctx context.Context, campaign *models.Campaign) error
	GetByID(ctx context.Context, id string) (*models.Campaign, error)
	Update(ctx context.Context, campaign *models.Campaign) error
	Delete(ctx context.Context, id string) (bool, error)
}

type campaignRepository struct {
	db      *bun.DB
	tagRepo TagRepository
}

// NewCampaignRepository returns a Bun-backed CampaignRepository.
func NewCampaignRepository(db *bun.DB, tagRepo TagRepository) CampaignRepository {
	return &campaignRepository{db: db, tagRepo: tagRepo}
}

func (r *campaignRepository) List(ctx context.Context) ([]models.Campaign, error) {
	var campaigns []models.Campaign
	if err := r.db.NewSelect().Model(&campaigns).OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	if err := r.hydrateTags(ctx, campaigns); err != nil {
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
	return &campaigns[0], nil
}

func (r *campaignRepository) Update(ctx context.Context, campaign *models.Campaign) error {
	_, err := r.db.NewUpdate().Model(campaign).WherePK().Exec(ctx)
	return err
}

func (r *campaignRepository) Delete(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewDelete().Model((*models.Campaign)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *campaignRepository) hydrateTags(ctx context.Context, campaigns []models.Campaign) error {
	return hydrateTags(ctx, campaigns, r.tagRepo,
		func(c models.Campaign) []string { return c.TagIDs },
		func(c *models.Campaign, tags []models.Tag) { c.Tags = tags },
	)
}
