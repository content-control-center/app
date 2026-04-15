package repository

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"math"
	"time"

	"github.com/uptrace/bun"
)

// AssetsEmbeddingsRepository handles persistence of asset vector embeddings.
type AssetsEmbeddingsRepository interface {
	Upsert(ctx context.Context, assetID string, vector []float32, model string) error
	GetByAssetID(ctx context.Context, assetID string) (vector []float32, model string, err error)
}

type embeddingRepository struct {
	db *bun.DB
}

// NewAssetsEmbeddingRepository returns a Bun-backed AssetsEmbeddingsRepository.
func NewAssetsEmbeddingRepository(db *bun.DB) AssetsEmbeddingsRepository {
	return &embeddingRepository{db: db}
}

type assetEmbedding struct {
	bun.BaseModel `bun:"table:assets_embeddings,alias:ae"`

	AssetID   string    `bun:"asset_id,pk"`
	Embedding []byte    `bun:"embedding,notnull"`
	Model     string    `bun:"model,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp"`
}

func encodeVector(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func decodeVector(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

func (r *embeddingRepository) Upsert(ctx context.Context, assetID string, vector []float32, model string) error {
	now := time.Now().UTC()
	row := &assetEmbedding{
		AssetID:   assetID,
		Embedding: encodeVector(vector),
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := r.db.NewInsert().Model(row).
		On("CONFLICT (asset_id) DO UPDATE").
		Set("embedding = EXCLUDED.embedding, model = EXCLUDED.model, updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (r *embeddingRepository) GetByAssetID(ctx context.Context, assetID string) ([]float32, string, error) {
	var row assetEmbedding
	err := r.db.NewSelect().Model(&row).Where("ae.asset_id = ?", assetID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", sql.ErrNoRows
		}
		return nil, "", err
	}
	return decodeVector(row.Embedding), row.Model, nil
}
