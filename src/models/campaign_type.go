package models

import (
	"time"

	"github.com/uptrace/bun"
)

type CampaignType struct {
	bun.BaseModel `bun:"table:campaigns_types,alias:ct" swaggerignore:"true"`

	ID          string    `bun:"id,pk"                                        json:"id"`
	Name        string    `bun:"name,notnull"                                 json:"name"`
	Label       string    `bun:"label,notnull"                                json:"label"`
	Description string    `bun:"description,notnull"                          json:"description"`
	IsSystem    bool      `bun:"is_system,notnull"                            json:"is_system"`
	CreatedAt   time.Time `bun:"created_at,notnull,default:current_timestamp" json:"-"`
	UpdatedAt   time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"-"`

	Phases []CampaignTypePhase `bun:"-" json:"phases,omitempty"`
}

type CampaignTypePhase struct {
	bun.BaseModel `bun:"table:campaigns_types_phases,alias:ctp" swaggerignore:"true"`

	ID             string    `bun:"id,pk"                                        json:"id"`
	CampaignTypeID string    `bun:"campaign_type_id,notnull"                     json:"campaign_type_id"`
	Name           string    `bun:"name,notnull"                                 json:"name"`
	Purpose        string    `bun:"purpose,notnull"                              json:"purpose"`
	Sequence       int       `bun:"sequence,notnull"                             json:"sequence"`
	CreatedAt      time.Time `bun:"created_at,notnull,default:current_timestamp" json:"-"`
	UpdatedAt      time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"-"`
}
