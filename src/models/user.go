package models

import (
	"time"

	"github.com/uptrace/bun"
)

// Role is a user's authority within their workspace (tenant). CON-26 introduces
// a minimal two-tier model — owner > member — forward-compatible with CON-147's
// eventual owner/admin/member split (an `admin` tier is inserted between them).
// The workspace creator is owner; a workspace always keeps at least one owner.
type Role = string

const (
	// RoleOwner may invite/remove teammates, change roles, and manage the
	// workspace; RoleMember is a full content collaborator that cannot manage
	// the team. See CON-26 §7 for the enforced capability matrix.
	RoleOwner  Role = "owner"
	RoleMember Role = "member"
)

// IsValidRole reports whether r is one of the roles CON-26 recognises.
func IsValidRole(r string) bool { return r == RoleOwner || r == RoleMember }

type User struct {
	bun.BaseModel `bun:"table:users,alias:u" swaggerignore:"true"`

	ID           string `bun:"id,pk"                json:"id"`
	TenantID     string `bun:"tenant_id,notnull"    json:"-"`
	Name         string `bun:"name,notnull"         json:"name"`
	Email        string `bun:"email,notnull,unique" json:"email"`
	PasswordHash string `bun:"password_hash,notnull" json:"-"`
	// Role is the user's authority in their tenant (CON-26). Backfilled to
	// 'owner' for every pre-CON-26 user (each was the sole member of its tenant).
	Role      string    `bun:"role,notnull,default:'member'" json:"role"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	// Tenant is loaded only on demand (e.g. GET /api/current_user via
	// UserRepository.GetByIDWithTenant) and omitted otherwise, so ordinary
	// user responses stay unchanged. CON-97.
	Tenant *Tenant `bun:"rel:belongs-to,join:tenant_id=id" json:"tenant,omitempty"`
}
