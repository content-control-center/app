package models

import (
	"time"

	"github.com/uptrace/bun"
)

// CampaignObjective is the marketing goal of a campaign.
type CampaignObjective string

const (
	ObjectiveAwareness  CampaignObjective = "awareness"
	ObjectiveEngagement CampaignObjective = "engagement"
	ObjectiveConversion CampaignObjective = "conversion"
	ObjectiveRetention  CampaignObjective = "retention"
)

// CampaignStatus represents the lifecycle state of a campaign.
type CampaignStatus string

const (
	StatusDraft     CampaignStatus = "draft"
	StatusScheduled CampaignStatus = "scheduled"
	StatusActive    CampaignStatus = "active"
	StatusPaused    CampaignStatus = "paused"
	StatusCompleted CampaignStatus = "completed"
	StatusArchived  CampaignStatus = "archived"
)

type Campaign struct {
	bun.BaseModel `bun:"table:campaigns,alias:c" swaggerignore:"true"`

	ID                string            `bun:"id,pk"                                        json:"id"`
	Name              string            `bun:"name,notnull"                                 json:"name"`
	Description       string            `bun:"description,notnull"                          json:"description"`
	TargetPersona     string            `bun:"target_persona,notnull"                       json:"target_persona"`
	KeyMessages       string            `bun:"key_messages,notnull"                         json:"key_messages"`
	ToneGuidelines    string            `bun:"tone_guidelines,notnull"                      json:"tone_guidelines"`
	UsePieces         bool              `bun:"use_pieces,notnull"                           json:"use_pieces"`
	PiecesIDs         StringSlice       `bun:"pieces_ids,notnull"                           json:"pieces_ids"`
	TargetPlatformIDs StringSlice       `bun:"target_platform_ids,notnull"                  json:"target_platform_ids"`
	Objective         CampaignObjective `bun:"objective,notnull"                            json:"objective"`
	Status            CampaignStatus    `bun:"status,notnull"                               json:"status"`
	StartDate         *time.Time        `bun:"start_date"                                   json:"start_date"`
	EndDate           *time.Time        `bun:"end_date"                                     json:"end_date"`
	Budget            *float64          `bun:"budget"                                       json:"budget"`
	Currency          string            `bun:"currency,notnull"                             json:"currency"`
	Tags              StringSlice       `bun:"tags,notnull"                                 json:"tags"`
	CreatedBy         string            `bun:"created_by,notnull"                           json:"created_by"`
	CreatedAt         time.Time         `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt         time.Time         `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}
