package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
)

// PostRepository defines all persistence operations for the Post domain.
type PostRepository interface {
	List(ctx context.Context) ([]models.Post, error)
	ListByCampaign(ctx context.Context, campaignID string) ([]models.Post, error)
	Create(ctx context.Context, post *models.Post) error
	CreateBatch(ctx context.Context, posts []*models.Post) error
	GetByID(ctx context.Context, id string) (*models.Post, error)
	Update(ctx context.Context, post *models.Post) error
	// UpdateScheduledAtBatch updates only the scheduled_at (+ updated_at)
	// column of several posts atomically (CON-115 redistribution).
	UpdateScheduledAtBatch(ctx context.Context, posts []*models.Post) error
	Delete(ctx context.Context, id string) (bool, error)
	// ListScheduledByPlatform returns every post in status='scheduled'
	// for the given platform id (Sqid) — the posts that still carry a
	// live auto-publish Zernio job. CON-130 uses it to convert all of a
	// platform's upcoming auto-publish posts to manual publishing when its
	// allowlist entry is turned off. No scheduled_at filter: a scheduled
	// post that is overdue but not yet published would still auto-publish,
	// so it must be converted too.
	ListScheduledByPlatform(ctx context.Context, platformID string) ([]models.Post, error)
	// CON-69 §8: reconciliation sweeper helpers.
	ListStuckScheduled(ctx context.Context, cutoff time.Time, limit int) ([]models.Post, error)
	UpdateStatusAndReason(ctx context.Context, postID string, status models.PostStatus, reason string) error
	// CON-93 §6 FR2: build the publisher_post_id → post_id match map the
	// analytics refresh keys off. Returns id + publisher_post_id only,
	// for Zernio-published posts that actually carry a publisher post id.
	ListWithPublisherPostID(ctx context.Context) ([]models.Post, error)
}

type postRepository struct {
	db *bun.DB
}

// NewPostRepository returns a Bun-backed PostRepository.
func NewPostRepository(db *bun.DB) PostRepository {
	return &postRepository{db: db}
}

func (r *postRepository) List(ctx context.Context) ([]models.Post, error) {
	var posts []models.Post
	if err := r.db.NewSelect().Model(&posts).OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	if err := r.hydrateRelations(ctx, posts); err != nil {
		return nil, err
	}
	return posts, nil
}

func (r *postRepository) ListByCampaign(ctx context.Context, campaignID string) ([]models.Post, error) {
	var posts []models.Post
	if err := r.db.NewSelect().Model(&posts).Where("po.campaign_id = ?", campaignID).OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	if err := r.hydrateRelations(ctx, posts); err != nil {
		return nil, err
	}
	return posts, nil
}

func (r *postRepository) Create(ctx context.Context, post *models.Post) error {
	_, err := r.db.NewInsert().Model(post).Exec(ctx)
	return err
}

func (r *postRepository) CreateBatch(ctx context.Context, posts []*models.Post) error {
	if len(posts) == 0 {
		return nil
	}
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewInsert().Model(&posts).Exec(ctx)
		return err
	})
}

func (r *postRepository) GetByID(ctx context.Context, id string) (*models.Post, error) {
	post := new(models.Post)
	err := r.db.NewSelect().Model(post).Where("po.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	posts := []models.Post{*post}
	if err := r.hydrateRelations(ctx, posts); err != nil {
		return nil, err
	}
	return &posts[0], nil
}

func (r *postRepository) Update(ctx context.Context, post *models.Post) error {
	_, err := r.db.NewUpdate().Model(post).WherePK().Exec(ctx)
	return err
}

func (r *postRepository) UpdateScheduledAtBatch(ctx context.Context, posts []*models.Post) error {
	if len(posts) == 0 {
		return nil
	}
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, p := range posts {
			// Only the schedule columns are written; the TenantScoped hook adds
			// the tenant predicate, WherePK adds the id. The status guard limits
			// the write to still-eligible posts, so a post that was concurrently
			// published, scheduled, or already redistributed matches zero rows,
			// and the exactly-one-row check fails the whole transaction rather
			// than clobbering a schedule that moved out from under us.
			res, err := tx.NewUpdate().Model(p).
				Column("scheduled_at", "updated_at").
				Where("status IN (?)", bun.In([]models.PostStatus{
					models.PostStatusDraft, models.PostStatusReadyForPublish,
				})).
				WherePK().
				Exec(ctx)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return fmt.Errorf("post %s is no longer eligible for redistribution (status changed or removed)", p.ID)
			}
		}
		return nil
	})
}

func (r *postRepository) Delete(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewDelete().Model((*models.Post)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ListStuckScheduled returns Posts in status='scheduled' whose
// scheduled_at is older than cutoff, capped at limit. Used by the
// reconciliation sweeper (CON-69 §8). NULL scheduled_at rows are
// skipped — a Scheduled post without a scheduled_at is a data
// integrity bug, not a reconciliation candidate.
func (r *postRepository) ListStuckScheduled(ctx context.Context, cutoff time.Time, limit int) ([]models.Post, error) {
	if limit <= 0 {
		limit = 100
	}
	var posts []models.Post
	err := r.db.NewSelect().
		Model(&posts).
		Where("po.status = ?", models.PostStatusScheduled).
		Where("po.scheduled_at IS NOT NULL").
		Where("po.scheduled_at < ?", cutoff).
		OrderExpr("po.scheduled_at ASC").
		Limit(limit).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return posts, nil
}

// ListScheduledByPlatform returns the tenant's posts in
// status='scheduled' for platformID (a platform Sqid), oldest
// scheduled_at first. See the interface doc for why scheduled_at is not
// filtered (CON-130).
func (r *postRepository) ListScheduledByPlatform(ctx context.Context, platformID string) ([]models.Post, error) {
	var posts []models.Post
	err := r.db.NewSelect().
		Model(&posts).
		Where("po.status = ?", models.PostStatusScheduled).
		Where("po.platform_id = ?", platformID).
		OrderExpr("po.scheduled_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return posts, nil
}

// UpdateStatusAndReason atomically sets status + failure_reason on
// one Post. Used by the reconciliation sweeper to avoid loading the
// full row just to flip two fields.
func (r *postRepository) UpdateStatusAndReason(ctx context.Context, postID string, status models.PostStatus, reason string) error {
	_, err := r.db.NewUpdate().
		Model((*models.Post)(nil)).
		Set("status = ?", status).
		Set("failure_reason = ?", reason).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", postID).
		Exec(ctx)
	return err
}

// ListWithPublisherPostID returns the (id, publisher_post_id) projection
// for every post that carries the Zernio publisher marker and a non-empty
// publisher_post_id — i.e. that was submitted through Zernio, whatever its
// current Ogen status. Status is deliberately NOT filtered: a partial
// publish maps to Ogen `failed` (see poll_zernio_status) yet still has real
// per-platform analytics that CON-93 §10 requires us to surface, and
// Zernio's own source=late filter is what gates which posts actually have
// analytics. The analytics refresh queue turns this into a
// publisher_post_id → post_id map to match the batch Zernio returns back to
// local posts (CON-93 §6 FR2). No relation hydration — only the two columns
// are needed.
func (r *postRepository) ListWithPublisherPostID(ctx context.Context) ([]models.Post, error) {
	var posts []models.Post
	err := r.db.NewSelect().
		Model(&posts).
		// title / published_at / platform_id / publisher are denormalised onto
		// the analytics snapshot at refresh time (CON-125 Track B), so the
		// overview read needs no cross-DB join back to posts/platforms.
		Column("id", "tenant_id", "publisher_post_id", "title", "published_at", "platform_id", "publisher").
		Where("po.publisher = ?", models.PublisherZernio).
		Where("po.publisher_post_id IS NOT NULL").
		Where("po.publisher_post_id <> ''").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return posts, nil
}

func (r *postRepository) hydrateRelations(ctx context.Context, posts []models.Post) error {
	for i := range posts {
		posts[i].UsedAssets = []models.Asset{}
	}

	campaignIDs := collectIDs(posts, func(p models.Post) string { return p.CampaignID })
	platformIDs := collectIDs(posts, func(p models.Post) string { return p.PlatformID })
	assetIDs := collectIDsFlat(posts, func(p models.Post) []string { return p.UsedAssetIDs })
	phaseIDs := collectIDsPtr(posts, func(p models.Post) *string { return p.CampaignTypePhaseID })

	campaignByID, err := fetchByIDs[models.Campaign](ctx, r.db, campaignIDs, func(c *models.Campaign) string { return c.ID })
	if err != nil {
		return err
	}
	for _, c := range campaignByID {
		c.Tags = []models.Tag{}
	}

	platformByID, err := fetchByIDs[models.Platform](ctx, r.db, platformIDs, func(p *models.Platform) string { return p.ID })
	if err != nil {
		return err
	}

	assetByID, err := fetchByIDs[models.Asset](ctx, r.db, assetIDs, func(p *models.Asset) string { return p.ID })
	if err != nil {
		return err
	}
	for _, p := range assetByID {
		p.Tags = []models.Tag{}
	}

	phaseByID, err := fetchByIDs[models.CampaignTypePhase](ctx, r.db, phaseIDs, func(p *models.CampaignTypePhase) string { return p.ID })
	if err != nil {
		return err
	}

	for i, p := range posts {
		posts[i].Campaign = campaignByID[p.CampaignID]
		posts[i].Platform = platformByID[p.PlatformID]
		for _, id := range p.UsedAssetIDs {
			if asset, ok := assetByID[id]; ok {
				posts[i].UsedAssets = append(posts[i].UsedAssets, *asset)
			}
		}
		if p.CampaignTypePhaseID != nil {
			posts[i].CampaignTypePhase = phaseByID[*p.CampaignTypePhaseID]
		}
	}
	return nil
}
