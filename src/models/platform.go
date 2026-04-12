package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Platform struct {
	bun.BaseModel `bun:"table:platforms,alias:pl" swaggerignore:"true"`

	ID          string      `bun:"id,pk"                                        json:"id"`
	Name        string      `bun:"name,notnull"                                 json:"name"`
	PostTypes   PostTypeMap `bun:"post_types,notnull"                           json:"post_types"`
	Cadence     string      `bun:"cadence,notnull"                              json:"cadence"`
	Constraints string      `bun:"constraints,notnull"                          json:"constraints"`
	CreatedAt   time.Time   `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt   time.Time   `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}
