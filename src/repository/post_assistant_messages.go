package repository

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/models"
)

// PostAssistantMessageRepository handles persistence of per-post conversation
// history between the user and the AI assistant.
type PostAssistantMessageRepository interface {
	Create(ctx context.Context, msg *models.PostAssistantMessage) error
	ListRecentByPostID(ctx context.Context, postID string, limit int) ([]models.PostAssistantMessage, error)
}

type postAssistantMessageRepository struct {
	db *bun.DB
}

func NewPostAssistantMessageRepository(db *bun.DB) PostAssistantMessageRepository {
	return &postAssistantMessageRepository{db: db}
}

func (r *postAssistantMessageRepository) Create(ctx context.Context, msg *models.PostAssistantMessage) error {
	_, err := r.db.NewInsert().Model(msg).Exec(ctx)
	return err
}

func (r *postAssistantMessageRepository) ListRecentByPostID(ctx context.Context, postID string, limit int) ([]models.PostAssistantMessage, error) {
	var msgs []models.PostAssistantMessage
	err := r.db.NewSelect().
		Model(&msgs).
		Where("pam.post_id = ?", postID).
		OrderExpr("pam.created_at DESC").
		Limit(limit).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	// Reverse to chronological order.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}
