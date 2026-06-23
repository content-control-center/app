package models

import (
	"time"

	"github.com/uptrace/bun"
)

type PostVersion struct {
	bun.BaseModel `bun:"table:post_versions,alias:pv" swaggerignore:"true"`
	TenantScoped  // CON-97: tenant_id column + central scoping hooks

	ID            string    `bun:"id,pk"                                        json:"id"`
	PostID        string    `bun:"post_id,notnull"                              json:"post_id"`
	VersionNumber int       `bun:"version_number,notnull"                       json:"version_number"`
	Content       string    `bun:"content,notnull"                              json:"content"`
	Note          string    `bun:"note,notnull"                                 json:"note"`
	Creator       string    `bun:"creator,notnull"                              json:"creator"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
}
