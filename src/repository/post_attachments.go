package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
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
	// Returns both the primary blob key and any non-empty thumbnail key
	// (CON-75) so the cascade-delete hook clears every object owned by
	// the post in one pass.
	var rows []struct {
		S3Key          string `bun:"s3_key"`
		ThumbnailS3Key string `bun:"thumbnail_s3_key"`
	}
	err := r.db.NewSelect().
		Model((*models.PostAttachment)(nil)).
		Column("s3_key", "thumbnail_s3_key").
		Where("post_id = ?", postID).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rows)*2)
	for _, row := range rows {
		if row.S3Key != "" {
			keys = append(keys, row.S3Key)
		}
		if row.ThumbnailS3Key != "" {
			keys = append(keys, row.ThumbnailS3Key)
		}
	}
	return keys, nil
}

func (r *postAttachmentRepository) CreateAtNextPosition(ctx context.Context, att *models.PostAttachment) error {
	var thumb any
	if att.ThumbnailS3Key != "" {
		thumb = att.ThumbnailS3Key
	}
	// Lock the parent post row for the duration of the insert so two
	// concurrent uploads to the same post serialise — otherwise both could
	// read the same MAX(position) and collide on the (post_id, position)
	// unique constraint. Postgres runs writers concurrently (SQLite did not),
	// so the serialisation must be explicit.
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw(`SELECT 1 FROM posts WHERE id = ? FOR UPDATE`, att.PostID).Exec(ctx); err != nil {
			return err
		}
		const q = `INSERT INTO post_attachments
			(id, post_id, position, mime_type, size_bytes, width, height,
			 is_animated, page_count, checksum_sha256, s3_key, thumbnail_s3_key, created_by)
			VALUES (?, ?, COALESCE((SELECT MAX(position)+1 FROM post_attachments WHERE post_id=?), 0),
			        ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING position`
		return tx.NewRaw(q,
			att.ID, att.PostID, att.PostID,
			att.MimeType, att.SizeBytes, att.Width, att.Height,
			att.IsAnimated, att.PageCount, att.ChecksumSHA256, att.S3Key, thumb, att.CreatedBy,
		).Scan(ctx, &att.Position)
	})
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
