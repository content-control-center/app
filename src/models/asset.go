package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Asset struct {
	bun.BaseModel `bun:"table:assets,alias:a" swaggerignore:"true"`

	ID        string      `bun:"id,pk"                                        json:"id"`
	Title     string      `bun:"title,notnull"                                json:"title"`
	Content   string      `bun:"content,notnull"                              json:"content"`
	TagIDs    StringSlice `bun:"tag_ids,notnull"                              json:"tag_ids"`
	Tags      []Tag       `bun:"-"                                            json:"tags"`
	CreatedBy string      `bun:"created_by,notnull"                           json:"created_by"`
	CreatedAt time.Time   `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time   `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}
