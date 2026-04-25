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

// ValidPostTransitions defines the allowed state-machine edges.
// The key is the current status; the value lists statuses it may move to.
var ValidPostTransitions = map[PostStatus][]PostStatus{
	PostStatusDraft:                     {PostStatusReadyForPublish},
	PostStatusReadyForPublish:           {PostStatusScheduled, PostStatusScheduledForManualPublish, PostStatusDraft},
	PostStatusScheduled:                 {PostStatusFailed, PostStatusPublished},
	PostStatusScheduledForManualPublish: {PostStatusPublished, PostStatusNotPublished},
	PostStatusFailed:                    {PostStatusReadyForPublish},
	PostStatusNotPublished:              {PostStatusReadyForPublish, PostStatusScheduledForManualPublish},
}

// CanTransition reports whether moving from the current status to next is allowed.
func (s PostStatus) CanTransition(next PostStatus) bool {
	if s == next {
		return true
	}
	for _, allowed := range ValidPostTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

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
	// platform_id and platform_post_type are nullable so draft posts can
	// exist without a platform chosen up front (CON-60). They become
	// required when the post moves out of "draft" — enforced by the
	// posts handler, not by the schema.
	//
	// `nullzero` makes bun send NULL (not "") for empty values, which is
	// required for platform_id because the FK to platforms can't match
	// an empty string. We use it on platform_post_type too for symmetry.
	PlatformID           string      `bun:"platform_id,nullzero"                         json:"platform_id"`
	PlatformPostType     string      `bun:"platform_post_type,nullzero"                  json:"platform_post_type"`
	Title                string      `bun:"title"                                        json:"title"`
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
