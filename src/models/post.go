package models

import (
	"time"

	"github.com/uptrace/bun"
)

// PostStatus represents the lifecycle state of a post.
type PostStatus string

const (
	PostStatusDraft                      PostStatus = "draft"
	PostStatusReadyForPublish            PostStatus = "ready_for_publish"
	PostStatusScheduled                  PostStatus = "scheduled"
	PostStatusScheduledForManualPublish  PostStatus = "scheduled_for_manual_publishing"
	PostStatusFailed                     PostStatus = "failed"
	PostStatusPublished                  PostStatus = "published"
	PostStatusNotPublished               PostStatus = "not_published"
)

// PostCTAType represents the call-to-action type attached to a post.
type PostCTAType string

const (
	CTATypeLink   PostCTAType = "link"
	CTATypeButton PostCTAType = "button"
	CTATypeNone   PostCTAType = "none"
)

type Post struct {
	bun.BaseModel `bun:"table:posts,alias:po" swaggerignore:"true"`

	ID                   string      `bun:"id,pk"                                        json:"id"`
	CampaignID           string      `bun:"campaign_id,notnull"                          json:"campaign_id"`
	PlatformID           string      `bun:"platform_id,notnull"                          json:"platform_id"`
	PlatformPostType     string      `bun:"platform_post_type,notnull"                   json:"platform_post_type"`
	Title                string      `bun:"title,notnull"                                json:"title"`
	Content              string      `bun:"content,notnull"                              json:"content"`
	MediaURLs            StringSlice `bun:"media_urls,notnull"                           json:"media_urls"`
	ScheduledAt          *time.Time  `bun:"scheduled_at"                                 json:"scheduled_at"`
	PublishedAt          *time.Time  `bun:"published_at"                                 json:"published_at"`
	Status               PostStatus  `bun:"status,notnull"                               json:"status"`
	CTAType              PostCTAType `bun:"cta_type,notnull"                             json:"cta_type"`
	CTAUrl               string      `bun:"cta_url,notnull"                              json:"cta_url"`
	TargetAudienceNotes  string      `bun:"target_audience_notes,notnull"                json:"target_audience_notes"`
	UsedAssetIDs        StringSlice `bun:"used_asset_ids,notnull"                      json:"used_asset_ids"`
	CampaignTypePhaseID  *string     `bun:"campaign_type_phase_id"                       json:"campaign_type_phase_id"`
	CreatedBy            string      `bun:"created_by,notnull"                           json:"created_by"`
	CreatedAt            time.Time   `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt            time.Time   `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	// Hydrated relations — not stored in the database.
	Campaign           *Campaign           `bun:"-" json:"campaign"`
	Platform           *Platform           `bun:"-" json:"platform"`
	UsedAssets         []Asset             `bun:"-" json:"used_assets"`
	CampaignTypePhase  *CampaignTypePhase  `bun:"-" json:"campaign_type_phase,omitempty"`
}
