package models

import (
	"time"

	"github.com/uptrace/bun"
)

// NotificationLevel is the severity of a notification (CON-242): a stable,
// closed set enforced by a DB CHECK and validated in Go, so the UI can map each
// level to an icon/colour. Distinct from Type, which is the open-ended machine
// key describing WHAT happened.
type NotificationLevel string

const (
	NotificationLevelInfo    NotificationLevel = "info"
	NotificationLevelSuccess NotificationLevel = "success"
	NotificationLevelWarning NotificationLevel = "warning"
	NotificationLevelError   NotificationLevel = "error"
)

// Valid reports whether l is a known level.
func (l NotificationLevel) Valid() bool {
	switch l {
	case NotificationLevelInfo, NotificationLevelSuccess, NotificationLevelWarning, NotificationLevelError:
		return true
	default:
		return false
	}
}

// Notification is one persistent, per-user inbox item (CON-242). Every event
// that concerns several people (e.g. all workspace owners) writes one row per
// recipient — fan-out on write — so read/unread is a plain column and reads are
// a trivial `WHERE user_id = me`.
//
// Seq is a monotonic BIGSERIAL used as the SSE stream cursor (Last-Event-ID /
// ?since=): the sqids ID is cryptographically random, so it cannot order the
// stream. It is DB-assigned (scanonly — never written by the app) and read back
// via RETURNING on insert.
type Notification struct {
	bun.BaseModel `bun:"table:notifications,alias:n" swaggerignore:"true"`
	TenantScoped  // tenant_id column + central scoping hooks (CON-97)

	ID          string            `bun:"id,pk"                                        json:"id"`
	Seq         int64             `bun:"seq,scanonly"                                 json:"seq"`
	UserID      string            `bun:"user_id,notnull"                              json:"user_id"`
	Level       NotificationLevel `bun:"level,notnull,default:'info'"                 json:"level"`
	Type        string            `bun:"type,notnull"                                 json:"type"`
	Title       string            `bun:"title,notnull"                                json:"title"`
	Body        string            `bun:"body,notnull,default:''"                      json:"body"`
	EntityType  string            `bun:"entity_type,notnull,default:''"               json:"entity_type,omitempty"`
	EntityID    string            `bun:"entity_id,notnull,default:''"                 json:"entity_id,omitempty"`
	ActionURL   string            `bun:"action_url,notnull,default:''"                json:"action_url,omitempty"`
	Data        JSONMap           `bun:"data,nullzero,type:jsonb"                     json:"data,omitempty"`
	DedupeKey   string            `bun:"dedupe_key,nullzero"                          json:"-"`
	ReadAt      *time.Time        `bun:"read_at,nullzero"                             json:"read_at"`
	DismissedAt *time.Time        `bun:"dismissed_at,nullzero"                        json:"-"`
	ExpiresAt   *time.Time        `bun:"expires_at,nullzero"                          json:"expires_at,omitempty"`
	CreatedAt   time.Time         `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
}
