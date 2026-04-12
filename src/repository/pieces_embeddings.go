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

// PiecesEmbeddingsRepository handles persistence of piece vector embeddings.
type PiecesEmbeddingsRepository interface {
	Upsert(ctx context.Context, pieceID string, vector []float32, model string) error
	GetByPieceID(ctx context.Context, pieceID string) (vector []float32, model string, err error)
}

type embeddingRepository struct {
	db *bun.DB
}

// NewPiecesEmbeddingRepository returns a Bun-backed PiecesEmbeddingsRepository.
func NewPiecesEmbeddingRepository(db *bun.DB) PiecesEmbeddingsRepository {
	return &embeddingRepository{db: db}
}

type pieceEmbedding struct {
	bun.BaseModel `bun:"table:pieces_embeddings,alias:pe"`

	PieceID   string    `bun:"piece_id,pk"`
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

func (r *embeddingRepository) Upsert(ctx context.Context, pieceID string, vector []float32, model string) error {
	now := time.Now().UTC()
	row := &pieceEmbedding{
		PieceID:   pieceID,
		Embedding: encodeVector(vector),
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := r.db.NewInsert().Model(row).
		On("CONFLICT (piece_id) DO UPDATE").
		Set("embedding = EXCLUDED.embedding, model = EXCLUDED.model, updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (r *embeddingRepository) GetByPieceID(ctx context.Context, pieceID string) ([]float32, string, error) {
	var row pieceEmbedding
	err := r.db.NewSelect().Model(&row).Where("pe.piece_id = ?", pieceID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", sql.ErrNoRows
		}
		return nil, "", err
	}
	return decodeVector(row.Embedding), row.Model, nil
}
