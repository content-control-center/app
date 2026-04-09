package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Tag struct {
	bun.BaseModel `bun:"table:tags,alias:t" swaggerignore:"true"`

	ID        string    `bun:"id,pk"                                        json:"id"`
	Name      string    `bun:"name,notnull"                                 json:"name"`
	Color     string    `bun:"color,notnull"                                json:"color"`
	CreatedBy string    `bun:"created_by,notnull"                           json:"created_by"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}
