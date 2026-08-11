package models

import (
	"time"

	"github.com/uptrace/bun"
)

// Tenant is the top-level isolation boundary (CON-97). Every tenant-owned row
// carries a tenant_id referencing this table, and every user belongs to
// exactly one tenant. The slug is a human-friendly, globally-unique label; it
// is NOT used to resolve tenancy — isolation is derived from the session.
//
// Third-party dependency credentials are deliberately NOT modelled per tenant:
// one Ogen-wide, KEK-encrypted set of keys serves every tenant (CON-97 §10.3).
type Tenant struct {
	bun.BaseModel `bun:"table:tenants,alias:tn" swaggerignore:"true"`

	ID        string    `bun:"id,pk"                                        json:"id"`
	Name      string    `bun:"name,notnull"                                 json:"name"`
	Slug      string    `bun:"slug,notnull,unique"                          json:"slug"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
	// DeletedAt marks a soft-deleted workspace (CON-147 PR4). Membership
	// resolution filters it out, so a deleted workspace can't be listed, switched
	// to, or entered — but the row survives for support-side recovery. Plain
	// timestamp, NOT a bun `,soft_delete` column: see the migration comment.
	DeletedAt *time.Time `bun:"deleted_at" json:"deleted_at,omitempty"`
}

// DefaultTenantID is the id of the tenant created by the multi-tenancy
// foundation migration. All data that predates CON-97 is backfilled onto it.
const DefaultTenantID = "default"
