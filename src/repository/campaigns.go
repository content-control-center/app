package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
)

// CampaignRepository defines all persistence operations for the Campaign domain.
//
// Lifecycle (CON-156 BE 6): List and GetByID exclude soft-deleted campaigns;
// List additionally excludes archived ones (reachable via ListArchived). Delete
// soft-deletes; Archive/Unarchive toggle archived_at. All are tenant-scoped by
// the model's Before* hooks.
type CampaignRepository interface {
	List(ctx context.Context) ([]models.Campaign, error)
	ListArchived(ctx context.Context) ([]models.Campaign, error)
	Create(ctx context.Context, campaign *models.Campaign) error
	GetByID(ctx context.Context, id string) (*models.Campaign, error)
	Update(ctx context.Context, campaign *models.Campaign) error
	Delete(ctx context.Context, id string) (bool, error)
	Archive(ctx context.Context, id string) (bool, error)
	Unarchive(ctx context.Context, id string) (bool, error)
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

// List returns the active set: campaigns that are neither soft-deleted nor
// archived, oldest first.
func (r *campaignRepository) List(ctx context.Context) ([]models.Campaign, error) {
	return r.listFiltered(ctx, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("c.archived_at IS NULL")
	})
}

// ListArchived returns archived (but not soft-deleted) campaigns, oldest first.
func (r *campaignRepository) ListArchived(ctx context.Context) ([]models.Campaign, error) {
	return r.listFiltered(ctx, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("c.archived_at IS NOT NULL")
	})
}

// listFiltered runs the shared list query + hydration, always excluding
// soft-deleted rows and applying the caller's archived-state predicate.
func (r *campaignRepository) listFiltered(ctx context.Context, extra func(*bun.SelectQuery) *bun.SelectQuery) ([]models.Campaign, error) {
	var campaigns []models.Campaign
	q := extra(r.db.NewSelect().Model(&campaigns).Where("c.deleted_at IS NULL"))
	if err := q.OrderExpr("created_at ASC").Scan(ctx); err != nil {
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

// GetByID returns a single campaign by id. Soft-deleted campaigns are treated
// as gone (sql.ErrNoRows); archived campaigns are still returned so they can be
// viewed and unarchived.
func (r *campaignRepository) GetByID(ctx context.Context, id string) (*models.Campaign, error) {
	campaign := new(models.Campaign)
	err := r.db.NewSelect().Model(campaign).Where("c.id = ?", id).Where("c.deleted_at IS NULL").Scan(ctx)
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

// Delete soft-deletes a campaign by stamping deleted_at. Returns false when the
// campaign does not exist (in this tenant) or was already deleted. The row is
// retained as an operational safety net; there is no self-serve restore.
func (r *campaignRepository) Delete(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewUpdate().Model((*models.Campaign)(nil)).
		Set("deleted_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Archive stamps archived_at, removing the campaign from the default active
// list. Idempotent: re-archiving a live campaign re-stamps the time. Returns
// false only when the campaign is missing or soft-deleted.
func (r *campaignRepository) Archive(ctx context.Context, id string) (bool, error) {
	now := time.Now().UTC()
	return r.setArchivedAt(ctx, id, &now)
}

// Unarchive clears archived_at, returning the campaign to the active list.
// Idempotent. Returns false only when the campaign is missing or soft-deleted.
func (r *campaignRepository) Unarchive(ctx context.Context, id string) (bool, error) {
	return r.setArchivedAt(ctx, id, nil)
}

// setArchivedAt sets archived_at to at (nil clears it) on a live (not deleted)
// campaign, returning whether a row matched.
func (r *campaignRepository) setArchivedAt(ctx context.Context, id string, at *time.Time) (bool, error) {
	res, err := r.db.NewUpdate().Model((*models.Campaign)(nil)).
		Set("archived_at = ?", at).
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		Exec(ctx)
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
