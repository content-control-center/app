package models

import (
	"time"

	"github.com/uptrace/bun"
)

// AssetFile stores metadata for file-backed Assets (currently PDF uploads).
// One-to-one with Asset via UNIQUE(asset_id).
type AssetFile struct {
	bun.BaseModel `bun:"table:asset_files,alias:af" swaggerignore:"true"`

	ID             string    `bun:"id,pk"                                        json:"id"`
	AssetID        string    `bun:"asset_id,notnull"                             json:"asset_id"`
	OriginalName   string    `bun:"original_name,notnull"                        json:"original_name"`
	MimeType       string    `bun:"mime_type,notnull"                            json:"mime_type"`
	SizeBytes      int64     `bun:"size_bytes,notnull"                           json:"size_bytes"`
	S3Key          string    `bun:"s3_key,notnull"                               json:"s3_key"`
	ThumbnailS3Key *string   `bun:"thumbnail_s3_key"                             json:"thumbnail_s3_key"`
	PageCount      *int      `bun:"page_count"                                   json:"page_count"`
	CreatedAt      time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt      time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	// ThumbnailURL is a transient public URL rendered from ThumbnailS3Key by
	// the handler layer before serialization. Not persisted.
	ThumbnailURL *string `bun:"-" json:"thumbnail_url,omitempty"`
}
