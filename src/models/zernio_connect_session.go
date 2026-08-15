package models

import (
	"time"

	"github.com/uptrace/bun"
)

// Connect-session status values (CON-217). A row is created pending_auth when a
// connect link is issued; the OAuth callback either finalizes it (row deleted)
// or transitions it to awaiting_selection when 2+ targets need an in-Ogen pick.
const (
	ZernioConnectStatusPendingAuth       = "pending_auth"
	ZernioConnectStatusAwaitingSelection = "awaiting_selection"
)

// ZernioConnectSession is the short-lived, server-side state for a headless
// Zernio account connection (CON-217 — hide Zernio's hosted account selector).
// It is created when a connect link is issued and consumed by the OAuth
// callback / picker; completed rows are deleted, expired rows swept.
//
// Deliberately NOT TenantScoped: the OAuth callback lands WITHOUT a tenant in
// context (the browser round-trips through Zernio), and resolves the tenant
// FROM this row via the opaque id. It carries a plain tenant_id column and is
// scoped explicitly at the authenticated picker boundary — mirroring Session
// (CON-97 PR2), which is likewise looked up before a tenant is known.
//
// The short-lived Zernio secrets (connect_token, tempToken, userProfile) are
// stored encrypted in SealedSecrets (envelope-encrypted JSON); Options holds
// only non-secret display fields for the picker and is plain JSON.
type ZernioConnectSession struct {
	bun.BaseModel `bun:"table:zernio_connect_sessions,alias:zcs" swaggerignore:"true"`

	ID            string    `bun:"id,pk"                                        json:"id"`
	TenantID      string    `bun:"tenant_id,notnull"                            json:"-"`
	ProfileID     string    `bun:"profile_id,notnull"                           json:"profile_id"`
	Platform      string    `bun:"platform,notnull"                             json:"platform"`
	Status        string    `bun:"status,notnull"                               json:"status"`
	SealedSecrets []byte    `bun:"sealed_secrets,nullzero"                      json:"-"`
	Options       []byte    `bun:"options,type:jsonb,nullzero"                  json:"-"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
	ExpiresAt     time.Time `bun:"expires_at,notnull"                           json:"expires_at"`
}
