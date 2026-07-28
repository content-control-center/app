package models

import (
	"time"

	"github.com/uptrace/bun"
)

type PostAttachment struct {
	bun.BaseModel `bun:"table:post_attachments,alias:pa" swaggerignore:"true"`
	TenantScoped  // CON-97: tenant_id column + central scoping hooks

	ID       string `bun:"id,pk"                                        json:"id"`
	PostID   string `bun:"post_id,notnull"                              json:"post_id"`
	Position int    `bun:"position,notnull"                             json:"position"`
	// AltText is the accessibility description for the media (CON-122). Empty
	// means "no alt text". Editable on upload and via PATCH; sent to Zernio as
	// mediaItems[].altText.
	AltText        string    `bun:"alt_text,notnull,default:''"                  json:"alt_text"`
	MimeType       string    `bun:"mime_type,notnull"                            json:"mime_type"`
	SizeBytes      int64     `bun:"size_bytes,notnull"                           json:"size_bytes"`
	Width          int       `bun:"width,notnull"                                json:"width"`
	Height         int       `bun:"height,notnull"                                json:"height"`
	IsAnimated     bool      `bun:"is_animated,notnull"                          json:"is_animated"`
	PageCount      int       `bun:"page_count,notnull"                           json:"page_count"`
	ChecksumSHA256 string    `bun:"checksum_sha256,notnull"                      json:"checksum_sha256"`
	S3Key          string    `bun:"s3_key,notnull"                               json:"s3_key"`
	ThumbnailS3Key string    `bun:"thumbnail_s3_key"                             json:"thumbnail_s3_key,omitempty"`
	CreatedBy      string    `bun:"created_by,notnull"                           json:"created_by"`
	CreatedAt      time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`

	// PresignedURL and ThumbnailURL are hydrated by the handler at
	// response time; they are not stored in the database.
	PresignedURL string `bun:"-" json:"presigned_url,omitempty"`
	ThumbnailURL string `bun:"-" json:"thumbnail_url,omitempty"`
}
