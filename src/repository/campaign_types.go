package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
)

// CampaignTypeRepository defines persistence operations for the CampaignType domain.
// System types (is_system = true) are seeded by migration; only user-created types
// support write operations.
type CampaignTypeRepository interface {
	List(ctx context.Context) ([]models.CampaignType, error)
	GetByID(ctx context.Context, id string) (*models.CampaignType, error)
	GetByIDs(ctx context.Context, ids []string) (map[string]*models.CampaignType, error)
	Create(ctx context.Context, ct *models.CampaignType) error
	Update(ctx context.Context, ct *models.CampaignType) error
	Delete(ctx context.Context, id string) (bool, error)

	AddPhase(ctx context.Context, phase *models.CampaignTypePhase) error
	GetPhaseByID(ctx context.Context, id string) (*models.CampaignTypePhase, error)
	UpdatePhase(ctx context.Context, phase *models.CampaignTypePhase) error
	DeletePhase(ctx context.Context, id string) (bool, error)
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

func (r *campaignTypeRepository) GetByIDs(ctx context.Context, ids []string) (map[string]*models.CampaignType, error) {
	if len(ids) == 0 {
		return map[string]*models.CampaignType{}, nil
	}
	var types []models.CampaignType
	if err := r.db.NewSelect().Model(&types).Where("ct.id IN (?)", bun.In(ids)).Scan(ctx); err != nil {
		return nil, err
	}
	if err := r.hydratePhases(ctx, types); err != nil {
		return nil, err
	}
	byID := make(map[string]*models.CampaignType, len(types))
	for i := range types {
		byID[types[i].ID] = &types[i]
	}
	return byID, nil
}

func (r *campaignTypeRepository) Create(ctx context.Context, ct *models.CampaignType) error {
	_, err := r.db.NewInsert().Model(ct).Exec(ctx)
	return err
}

func (r *campaignTypeRepository) Update(ctx context.Context, ct *models.CampaignType) error {
	_, err := r.db.NewUpdate().Model(ct).WherePK().Exec(ctx)
	return err
}

func (r *campaignTypeRepository) Delete(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewDelete().TableExpr("campaigns_types").Where("id = ?", id).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *campaignTypeRepository) AddPhase(ctx context.Context, phase *models.CampaignTypePhase) error {
	_, err := r.db.NewInsert().Model(phase).Exec(ctx)
	return err
}

func (r *campaignTypeRepository) GetPhaseByID(ctx context.Context, id string) (*models.CampaignTypePhase, error) {
	phase := new(models.CampaignTypePhase)
	err := r.db.NewSelect().Model(phase).Where("ctp.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return phase, nil
}

func (r *campaignTypeRepository) UpdatePhase(ctx context.Context, phase *models.CampaignTypePhase) error {
	_, err := r.db.NewUpdate().Model(phase).WherePK().Exec(ctx)
	return err
}

func (r *campaignTypeRepository) DeletePhase(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewDelete().TableExpr("campaigns_types_phases").Where("id = ?", id).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
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
