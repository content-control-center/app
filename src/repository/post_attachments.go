package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/models"
)

// PostAttachmentRepository persists image attachments bound to a Post (CON-73).
type PostAttachmentRepository interface {
	ListByPostID(ctx context.Context, postID string) ([]models.PostAttachment, error)
	GetByID(ctx context.Context, id string) (*models.PostAttachment, error)
	Create(ctx context.Context, att *models.PostAttachment) error
	UpdatePosition(ctx context.Context, id string, position int) error
	Delete(ctx context.Context, id string) (bool, error)
	NextPosition(ctx context.Context, postID string) (int, error)
}

type postAttachmentRepository struct {
	db *bun.DB
}

func NewPostAttachmentRepository(db *bun.DB) PostAttachmentRepository {
	return &postAttachmentRepository{db: db}
}

func (r *postAttachmentRepository) ListByPostID(ctx context.Context, postID string) ([]models.PostAttachment, error) {
	var atts []models.PostAttachment
	err := r.db.NewSelect().
		Model(&atts).
		Where("pa.post_id = ?", postID).
		OrderExpr("position ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return atts, nil
}

func (r *postAttachmentRepository) GetByID(ctx context.Context, id string) (*models.PostAttachment, error) {
	att := new(models.PostAttachment)
	err := r.db.NewSelect().Model(att).Where("pa.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return att, nil
}

func (r *postAttachmentRepository) Create(ctx context.Context, att *models.PostAttachment) error {
	_, err := r.db.NewInsert().Model(att).Exec(ctx)
	return err
}

func (r *postAttachmentRepository) UpdatePosition(ctx context.Context, id string, position int) error {
	_, err := r.db.NewUpdate().
		Model((*models.PostAttachment)(nil)).
		Set("position = ?", position).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (r *postAttachmentRepository) Delete(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewDelete().
		Model((*models.PostAttachment)(nil)).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// NextPosition returns max(position)+1 for the post, or 0 when no
// attachments exist yet. Last-write-wins for the MVP — no optimistic
// concurrency token (per CON-73 §6 decision).
func (r *postAttachmentRepository) NextPosition(ctx context.Context, postID string) (int, error) {
	var max sql.NullInt64
	err := r.db.NewSelect().
		Model((*models.PostAttachment)(nil)).
		ColumnExpr("MAX(position)").
		Where("post_id = ?", postID).
		Scan(ctx, &max)
	if err != nil {
		return 0, err
	}
	if !max.Valid {
		return 0, nil
	}
	return int(max.Int64) + 1, nil
}
