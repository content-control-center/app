package models

import (
	"time"

	"github.com/pgvector/pgvector-go"
	"github.com/uptrace/bun"
)

// AssetChunk stores one chunk of an Asset's text content together with its
// embedding vector. A short asset produces a single chunk (chunk_index = 0).
// Longer assets are split into multiple overlapping chunks.
type AssetChunk struct {
	bun.BaseModel `bun:"table:assets_chunks,alias:ac" swaggerignore:"true"`
	TenantScoped  // CON-97: tenant_id column + central scoping hooks

	ID         string `bun:"id,pk"                                        json:"id"`
	AssetID    string `bun:"asset_id,notnull"                             json:"asset_id"`
	ChunkIndex int    `bun:"chunk_index,notnull"                          json:"chunk_index"`
	PageStart  *int   `bun:"page_start"                                   json:"page_start"`
	PageEnd    *int   `bun:"page_end"                                     json:"page_end"`
	Content    string `bun:"content,notnull"                              json:"content"`
	TokenCount int    `bun:"token_count,notnull"                          json:"token_count"`
	// Embedding is the chunk's 3072-dim Gemini Embedding 2 vector (CON-101),
	// stored in a pgvector halfvec(3072) column. halfvec (16-bit floats) is
	// required because pgvector's full-precision vector HNSW index caps at 2000
	// dimensions. Similarity search runs in-database via the `<=>`
	// cosine-distance operator (see AssetChunksRepository.SearchSimilar).
	Embedding pgvector.HalfVector `bun:"embedding,type:halfvec(3072)" json:"-"`
	Model     string          `bun:"model"                     json:"model"`
	CreatedAt time.Time       `bun:"created_at,notnull,default:current_timestamp" json:"-"`
}
