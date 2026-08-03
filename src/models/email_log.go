package models

import (
	"time"

	"github.com/uptrace/bun"
)

// EmailLogStatus is the terminal (or terminal-ish) outcome recorded for one
// send attempt. Delivery events arriving later via the Resend webhook update
// the same row in place by provider_message_id (CON-154 FR8).
type EmailLogStatus string

const (
	EmailLogQueued            EmailLogStatus = "queued"
	EmailLogSent              EmailLogStatus = "sent"
	EmailLogFailed            EmailLogStatus = "failed"
	EmailLogSkippedSuppressed EmailLogStatus = "skipped_suppressed"
	EmailLogSkippedDisabled   EmailLogStatus = "skipped_disabled"
	// EmailLogBounced / EmailLogComplained are written by the webhook when
	// Resend reports a hard bounce or a spam complaint against a prior send.
	EmailLogBounced    EmailLogStatus = "bounced"
	EmailLogComplained EmailLogStatus = "complained"
)

// ProviderResend is the only mail provider today (CON-154). Recorded on every
// row so a future second provider stays distinguishable in the audit trail.
const ProviderResend = "resend"

// EmailLog is one entry in the append-only send audit (CON-154 §7), mirroring
// PostLog. tenant_id / user_id are nullable because some mail (future
// system/ops notices) has no owning tenant or user. idempotency_key is nullable
// so the partial unique index only constrains rows that carry one.
type EmailLog struct {
	bun.BaseModel `bun:"table:email_logs,alias:el" swaggerignore:"true"`

	ID                string         `bun:"id,pk"                                        json:"id"`
	TenantID          string         `bun:"tenant_id,nullzero"                           json:"tenant_id,omitempty"`
	UserID            string         `bun:"user_id,nullzero"                             json:"user_id,omitempty"`
	TemplateID        string         `bun:"template_id,notnull"                          json:"template_id"`
	Kind              EmailKind      `bun:"kind,notnull"                                 json:"kind"`
	ToEmail           string         `bun:"to_email,notnull"                             json:"to_email"`
	Status            EmailLogStatus `bun:"status,notnull"                               json:"status"`
	Provider          string         `bun:"provider,notnull,default:'resend'"            json:"provider"`
	ProviderMessageID string         `bun:"provider_message_id,nullzero"                 json:"provider_message_id,omitempty"`
	IdempotencyKey    string         `bun:"idempotency_key,nullzero"                     json:"idempotency_key,omitempty"`
	Error             string         `bun:"error,nullzero"                               json:"error,omitempty"`
	CreatedAt         time.Time      `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt         time.Time      `bun:"updated_at,nullzero,notnull,default:current_timestamp" json:"updated_at"`
}
