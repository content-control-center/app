package models

import (
	"time"

	"github.com/uptrace/bun"
)

type User struct {
	bun.BaseModel `bun:"table:users,alias:u" swaggerignore:"true"`

	ID           string    `bun:"id,pk"                json:"id"`
	TenantID     string    `bun:"tenant_id,notnull"    json:"-"`
	Name         string    `bun:"name,notnull"         json:"name"`
	Email        string    `bun:"email,notnull,unique" json:"email"`
	PasswordHash string    `bun:"password_hash,notnull" json:"-"`
	CreatedAt    time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt    time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	// Tenant is loaded only on demand (e.g. GET /api/current_user via
	// UserRepository.GetByIDWithTenant) and omitted otherwise, so ordinary
	// user responses stay unchanged. CON-97.
	Tenant *Tenant `bun:"rel:belongs-to,join:tenant_id=id" json:"tenant,omitempty"`
}
