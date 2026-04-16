package repository

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/models"
)

// AssetChunksRepository handles persistence of asset text chunks and their
// embedding vectors.
type AssetChunksRepository interface {
	// UpsertChunks atomically replaces all chunks for the given asset.
	UpsertChunks(ctx context.Context, assetID string, chunks []models.AssetChunk) error
	// GetByAssetID returns all chunks for an asset, ordered by chunk_index.
	GetByAssetID(ctx context.Context, assetID string) ([]models.AssetChunk, error)
	// GetAllEmbedded returns all chunks that have a non-nil embedding vector.
	GetAllEmbedded(ctx context.Context) ([]models.AssetChunk, error)
	// DeleteByAssetID removes all chunks for an asset.
	DeleteByAssetID(ctx context.Context, assetID string) error
	// GetByIDs returns chunks matching the given primary-key IDs, ordered by
	// asset_id then chunk_index.
	GetByIDs(ctx context.Context, ids []string) ([]models.AssetChunk, error)
}

type assetChunksRepository struct {
	db *bun.DB
}

func NewAssetChunksRepository(db *bun.DB) AssetChunksRepository {
	return &assetChunksRepository{db: db}
}

func (r *assetChunksRepository) UpsertChunks(ctx context.Context, assetID string, chunks []models.AssetChunk) error {
	return withRetry(ctx, func() error {
		// Rebuild the pointer slice on every attempt — bun's Model() may
		// mutate the passed slice on a failed INSERT, so reusing the same
		// slice across retries would silently lose data.
		ptrs := make([]*models.AssetChunk, len(chunks))
		for i := range chunks {
			ptrs[i] = &chunks[i]
		}

		return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			delRes, err := tx.NewDelete().
				Model((*models.AssetChunk)(nil)).
				Where("asset_id = ?", assetID).
				Exec(ctx)
			if err != nil {
				return fmt.Errorf("delete existing chunks: %w", err)
			}
			delN, _ := delRes.RowsAffected()

			if len(ptrs) == 0 {
				log.Printf("UpsertChunks %s: deleted %d, nothing to insert", assetID, delN)
				return nil
			}

			insRes, err := tx.NewInsert().Model(&ptrs).Exec(ctx)
			if err != nil {
				return fmt.Errorf("insert chunks: %w", err)
			}
			insN, _ := insRes.RowsAffected()
			log.Printf("UpsertChunks %s: deleted %d, inserted %d (wanted %d)", assetID, delN, insN, len(ptrs))
			return nil
		})
	})
}

// withRetry retries a SQLite operation up to 3 times on transient I/O errors.
// Backoff is exponential: 100ms, 400ms, 1600ms.
func withRetry(ctx context.Context, op func() error) error {
	const maxAttempts = 3
	delay := 100 * time.Millisecond

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := op()
		if err == nil {
			return nil
		}
		lastErr = err
		if !isTransientSQLiteError(err) {
			return err
		}
		if attempt == maxAttempts {
			break
		}
		log.Printf("sqlite: transient error on attempt %d/%d: %v — retrying in %s", attempt, maxAttempts, err, delay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 4
	}
	return lastErr
}

// isTransientSQLiteError returns true for errors that may succeed on retry,
// typically disk I/O glitches in virtualised filesystems (Docker Desktop).
func isTransientSQLiteError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "disk I/O error") ||
		strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked")
}

func (r *assetChunksRepository) GetByAssetID(ctx context.Context, assetID string) ([]models.AssetChunk, error) {
	var chunks []models.AssetChunk
	err := r.db.NewSelect().
		Model(&chunks).
		Where("ac.asset_id = ?", assetID).
		OrderExpr("ac.chunk_index ASC").
		Scan(ctx)
	return chunks, err
}

func (r *assetChunksRepository) GetAllEmbedded(ctx context.Context) ([]models.AssetChunk, error) {
	var chunks []models.AssetChunk
	err := r.db.NewSelect().
		Model(&chunks).
		Where("ac.embedding IS NOT NULL").
		OrderExpr("ac.asset_id ASC, ac.chunk_index ASC").
		Scan(ctx)
	return chunks, err
}

func (r *assetChunksRepository) DeleteByAssetID(ctx context.Context, assetID string) error {
	_, err := r.db.NewDelete().
		Model((*models.AssetChunk)(nil)).
		Where("asset_id = ?", assetID).
		Exec(ctx)
	return err
}

func (r *assetChunksRepository) GetByIDs(ctx context.Context, ids []string) ([]models.AssetChunk, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var chunks []models.AssetChunk
	err := r.db.NewSelect().
		Model(&chunks).
		Where("ac.id IN (?)", bun.In(ids)).
		OrderExpr("ac.asset_id ASC, ac.chunk_index ASC").
		Scan(ctx)
	return chunks, err
}
