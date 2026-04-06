package models

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

type Session struct {
	bun.BaseModel `bun:"table:sessions,alias:s" swaggerignore:"true"`

	ID        string    `bun:"id,pk"                                        json:"id"`
	UserID    string    `bun:"user_id,notnull"                              json:"user_id"`
	ExpiresAt time.Time `bun:"expires_at,notnull"                           json:"expires_at"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
}

// NewSessionToken generates a cryptographically random 32-byte URL-safe token.
func NewSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
