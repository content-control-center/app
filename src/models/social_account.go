package models

import (
	"time"

	"github.com/uptrace/bun"
)

// SocialAccount is the local view of a Zernio-connected account
// (CON-62). The primary key matches Zernio's accountId so reconciler
// upserts can key off it directly.
//
// Soft-delete is the default disconnect path: DeletedAt set to a
// non-nil time means the account was removed on Zernio's side, and
// the row is preserved so future Pieces / Posts that reference it
// keep their FK integrity.
type SocialAccount struct {
	bun.BaseModel `bun:"table:social_accounts,alias:sa" swaggerignore:"true"`
	TenantScoped  // CON-97: tenant_id column + central scoping hooks

	ID           string     `bun:"id,pk"                 json:"id"`
	Platform     string     `bun:"platform,notnull"      json:"platform"`
	ProfileID    string     `bun:"profile_id,notnull"    json:"profile_id"`
	Username     string     `bun:"username,notnull"      json:"username"`
	DisplayName  string     `bun:"display_name,notnull"  json:"display_name"`
	AvatarURL    string     `bun:"avatar_url,notnull"    json:"avatar_url"`
	IsActive     bool       `bun:"is_active,notnull"     json:"is_active"`
	RawJSON      string     `bun:"raw_json,notnull"      json:"-"`
	ConnectedAt  time.Time  `bun:"connected_at,notnull"  json:"connected_at"`
	LastSyncedAt time.Time  `bun:"last_synced_at,notnull" json:"last_synced_at"`
	DeletedAt    *time.Time `bun:"deleted_at,nullzero"   json:"deleted_at,omitempty"`
}
