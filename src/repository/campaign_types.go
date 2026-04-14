package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/models"
)

// CampaignTypeRepository defines read operations for the CampaignType domain.
// Types and phases are seeded by migration and are never created or deleted via the API.
type CampaignTypeRepository interface {
	List(ctx context.Context) ([]models.CampaignType, error)
	GetByID(ctx context.Context, id string) (*models.CampaignType, error)
}

type campaignTypeRepository struct {
	db *bun.DB
}

func NewCampaignTypeRepository(db *bun.DB) CampaignTypeRepository {
	return &campaignTypeRepository{db: db}
}

func (r *campaignTypeRepository) List(ctx context.Context) ([]models.CampaignType, error) {
	var types []models.CampaignType
	if err := r.db.NewSelect().Model(&types).OrderExpr("name ASC").Scan(ctx); err != nil {
		return nil, err
	}
	if err := r.hydratePhases(ctx, types); err != nil {
		return nil, err
	}
	return types, nil
}

func (r *campaignTypeRepository) GetByID(ctx context.Context, id string) (*models.CampaignType, error) {
	ct := new(models.CampaignType)
	err := r.db.NewSelect().Model(ct).Where("ct.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	types := []models.CampaignType{*ct}
	if err := r.hydratePhases(ctx, types); err != nil {
		return nil, err
	}
	return &types[0], nil
}

func (r *campaignTypeRepository) hydratePhases(ctx context.Context, types []models.CampaignType) error {
	for i := range types {
		types[i].Phases = []models.CampaignTypePhase{}
	}
	if len(types) == 0 {
		return nil
	}

	ids := make([]string, len(types))
	for i, ct := range types {
		ids[i] = ct.ID
	}

	var phases []models.CampaignTypePhase
	if err := r.db.NewSelect().
		Model(&phases).
		Where("ctp.campaign_type_id IN (?)", bun.In(ids)).
		OrderExpr("ctp.sequence ASC").
		Scan(ctx); err != nil {
		return err
	}

	index := make(map[string]int, len(types))
	for i, ct := range types {
		index[ct.ID] = i
	}
	for _, p := range phases {
		if i, ok := index[p.CampaignTypeID]; ok {
			types[i].Phases = append(types[i].Phases, p)
		}
	}
	return nil
}
