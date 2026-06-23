package models

import (
	"time"

	"github.com/uptrace/bun"
)

// Secret is the persisted, envelope-encrypted form of a third-party API
// key. Plaintext never appears in this struct — Ciphertext / Nonce /
// WrappedDEK / DEKNonce are the inputs to envelope.Cipher.Decrypt.
//
// Rows are keyed by Name (the human-readable allowlist entry, e.g.
// "anthropic_api_key"). The numeric ID is incidental and only exists
// to give each row a stable numeric identity.
type Secret struct {
	bun.BaseModel `bun:"table:secret,alias:s" swaggerignore:"true"`

	ID         int64     `bun:"id,pk,autoincrement" json:"id"`
	Name       string    `bun:"name,notnull,unique" json:"name"`
	Ciphertext []byte    `bun:"ciphertext,notnull"  json:"-"`
	Nonce      []byte    `bun:"nonce,notnull"       json:"-"`
	WrappedDEK []byte    `bun:"wrapped_dek,notnull" json:"-"`
	DEKNonce   []byte    `bun:"dek_nonce,notnull"   json:"-"`
	Algorithm  string    `bun:"algorithm,notnull"   json:"algorithm"`
	KEKVersion int       `bun:"kek_version,notnull" json:"kek_version"`
	CreatedAt  time.Time `bun:"created_at,notnull"  json:"created_at"`
	UpdatedAt  time.Time `bun:"updated_at,notnull"  json:"updated_at"`
}
