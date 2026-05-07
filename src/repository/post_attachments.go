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
	ListS3KeysByPostID(ctx context.Context, postID string) ([]string, error)
	GetByID(ctx context.Context, id string) (*models.PostAttachment, error)
	// CreateAtNextPosition inserts att and assigns it the next free
	// position for its post in a single atomic statement, so concurrent
	// uploads to the same post never trip the (post_id, position)
	// unique constraint. att.Position is overwritten with the assigned
	// value on success.
	CreateAtNextPosition(ctx context.Context, att *models.PostAttachment) error
	UpdatePosition(ctx context.Context, id string, position int) error
	Delete(ctx context.Context, id string) (bool, error)
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

func (r *postAttachmentRepository) ListS3KeysByPostID(ctx context.Context, postID string) ([]string, error) {
	var keys []string
	err := r.db.NewSelect().
		Model((*models.PostAttachment)(nil)).
		Column("s3_key").
		Where("post_id = ?", postID).
		Scan(ctx, &keys)
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func (r *postAttachmentRepository) CreateAtNextPosition(ctx context.Context, att *models.PostAttachment) error {
	// Single-statement INSERT computes the next position from the same
	// SELECT, so two concurrent uploads to the same post can't both
	// resolve the same value and collide on the unique constraint.
	// SQLite serialises writers, so the SELECT subquery sees the
	// committed state at the moment the INSERT starts.
	const q = `INSERT INTO post_attachments
		(id, post_id, position, mime_type, size_bytes, width, height,
		 is_animated, checksum_sha256, s3_key, created_by)
		VALUES (?, ?, COALESCE((SELECT MAX(position)+1 FROM post_attachments WHERE post_id=?), 0),
		        ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING position`
	row := r.db.QueryRowContext(ctx, q,
		att.ID, att.PostID, att.PostID,
		att.MimeType, att.SizeBytes, att.Width, att.Height,
		att.IsAnimated, att.ChecksumSHA256, att.S3Key, att.CreatedBy,
	)
	if err := row.Scan(&att.Position); err != nil {
		return err
	}
	return nil
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

