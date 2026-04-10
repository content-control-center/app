package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/models"
)

// PostRepository defines all persistence operations for the Post domain.
type PostRepository interface {
	List(ctx context.Context) ([]models.Post, error)
	ListByCampaign(ctx context.Context, campaignID string) ([]models.Post, error)
	Create(ctx context.Context, post *models.Post) error
	CreateBatch(ctx context.Context, posts []*models.Post) error
	GetByID(ctx context.Context, id string) (*models.Post, error)
	Update(ctx context.Context, post *models.Post) error
	Delete(ctx context.Context, id string) (bool, error)
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

func (r *postRepository) Delete(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewDelete().Model((*models.Post)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *postRepository) hydrateRelations(ctx context.Context, posts []models.Post) error {
	for i := range posts {
		posts[i].UsedPieces = []models.Piece{}
	}

	campaignIDs := collectIDs(posts, func(p models.Post) string { return p.CampaignID })
	platformIDs := collectIDs(posts, func(p models.Post) string { return p.PlatformID })
	pieceIDs := collectIDsFlat(posts, func(p models.Post) []string { return p.UsedPiecesIDs })

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

	pieceByID, err := fetchByIDs[models.Piece](ctx, r.db, pieceIDs, func(p *models.Piece) string { return p.ID })
	if err != nil {
		return err
	}
	for _, p := range pieceByID {
		p.Tags = []models.Tag{}
	}

	for i, p := range posts {
		posts[i].Campaign = campaignByID[p.CampaignID]
		posts[i].Platform = platformByID[p.PlatformID]
		for _, id := range p.UsedPiecesIDs {
			if piece, ok := pieceByID[id]; ok {
				posts[i].UsedPieces = append(posts[i].UsedPieces, *piece)
			}
		}
	}
	return nil
}
