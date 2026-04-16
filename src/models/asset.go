package models

import (
	"time"

	"github.com/uptrace/bun"
)

const (
	AssetStatusPending    = "pending"
	AssetStatusProcessing = "processing"
	AssetStatusReady      = "ready"
	AssetStatusFailed     = "failed"
)

const (
	// AssetTypeMarkdown marks assets imported from a `.md` upload.
	AssetTypeMarkdown = "MD"
)

type Asset struct {
	bun.BaseModel `bun:"table:assets,alias:a" swaggerignore:"true"`

	ID        string      `bun:"id,pk"                                        json:"id"`
	Title     string      `bun:"title,notnull"                                json:"title"`
	Content   string      `bun:"content,notnull"                              json:"content"`
	Status    string      `bun:"status,notnull,default:'ready'"               json:"status"`
	Type      *string     `bun:"type"                                         json:"type"`
	TagIDs    StringSlice `bun:"tag_ids,notnull"                              json:"tag_ids"`
	Tags      []Tag       `bun:"-"                                            json:"tags"`
	CreatedBy string      `bun:"created_by,notnull"                           json:"created_by"`
	CreatedAt time.Time   `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time   `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}
