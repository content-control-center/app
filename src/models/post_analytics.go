package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// PostAnalyticsMetrics is the aggregate engagement block surfaced in
// analytics API responses (CON-93 §7). The same shape is embedded inside
// each per-platform breakdown row. On the stored PostAnalytics row these
// aggregates live in their own columns (for sortable list/overview
// queries); this struct is the assembled JSON representation.
type PostAnalyticsMetrics struct {
	Impressions    int     `json:"impressions"`
	Reach          int     `json:"reach"`
	Likes          int     `json:"likes"`
	Comments       int     `json:"comments"`
	Shares         int     `json:"shares"`
	Saves          int     `json:"saves"`
	Clicks         int     `json:"clicks"`
	Views          int     `json:"views"`
	EngagementRate float64 `json:"engagement_rate"`
}

// PostPlatformAnalytics is one per-platform row of the breakdown stored
// in post_analytics.platform_analytics (CON-93 §7). SyncStatus /
// ErrorMessage / ReauthorizeURL carry the per-platform scope-gap nuance
// (412/reauthorizeUrl): a platform connected before analytics scopes
// were granted surfaces its error here without failing the whole
// snapshot, while platforms that do report still carry metrics.
type PostPlatformAnalytics struct {
	Platform        string               `json:"platform"`
	Status          string               `json:"status,omitempty"`
	PlatformPostID  string               `json:"platform_post_id,omitempty"`
	AccountID       string               `json:"account_id,omitempty"`
	AccountUsername string               `json:"account_username,omitempty"`
	PlatformPostURL string               `json:"platform_post_url,omitempty"`
	SyncStatus      string               `json:"sync_status,omitempty"`
	ErrorMessage    string               `json:"error_message,omitempty"`
	ReauthorizeURL  string               `json:"reauthorize_url,omitempty"`
	Analytics       PostAnalyticsMetrics `json:"analytics"`
}

// PlatformAnalyticsList is a []PostPlatformAnalytics that serialises as a
// JSON array in a SQLite TEXT column (mirrors StringSlice / CampaignPlatforms).
type PlatformAnalyticsList []PostPlatformAnalytics

func (l PlatformAnalyticsList) Value() (driver.Value, error) {
	if l == nil {
		return "[]", nil
	}
	b, err := json.Marshal(l)
	return string(b), err
}

func (l *PlatformAnalyticsList) Scan(src any) error {
	switch v := src.(type) {
	case string:
		if v == "" {
			*l = PlatformAnalyticsList{}
			return nil
		}
		return json.Unmarshal([]byte(v), l)
	case []byte:
		if len(v) == 0 {
			*l = PlatformAnalyticsList{}
			return nil
		}
		return json.Unmarshal(v, l)
	case nil:
		*l = PlatformAnalyticsList{}
		return nil
	default:
		return fmt.Errorf("PlatformAnalyticsList: cannot scan %T", src)
	}
}

// PostAnalytics is the latest engagement snapshot for a Post published
// through a publisher (CON-93). Exactly one row per Post — the refresh
// queue UPSERTs on post_id, so this always reflects the most recent
// fetch.
//
// The aggregate metrics are denormalised columns (sortable in the
// list/overview); PlatformAnalytics holds the per-platform breakdown as
// JSON. MetricsLastUpdated is the publisher's own "numbers last computed"
// timestamp; LastRefreshedAt is when Ogen last fetched — together they
// expose staleness. RawJSON keeps the verbatim publisher item.
type PostAnalytics struct {
	bun.BaseModel `bun:"table:post_analytics,alias:pa" swaggerignore:"true"`

	PostID             string                `bun:"post_id,pk"                                   json:"post_id"`
	PublisherPostID    string                `bun:"publisher_post_id,notnull"                    json:"publisher_post_id"`
	Impressions        int                   `bun:"impressions,notnull"                          json:"impressions"`
	Reach              int                   `bun:"reach,notnull"                                json:"reach"`
	Likes              int                   `bun:"likes,notnull"                                json:"likes"`
	Comments           int                   `bun:"comments,notnull"                             json:"comments"`
	Shares             int                   `bun:"shares,notnull"                               json:"shares"`
	Saves              int                   `bun:"saves,notnull"                                json:"saves"`
	Clicks             int                   `bun:"clicks,notnull"                               json:"clicks"`
	Views              int                   `bun:"views,notnull"                                json:"views"`
	EngagementRate     float64               `bun:"engagement_rate,notnull"                      json:"engagement_rate"`
	PlatformAnalytics  PlatformAnalyticsList `bun:"platform_analytics,notnull"                   json:"platform_analytics"`
	SyncStatus         string                `bun:"sync_status,notnull"                          json:"sync_status"`
	MetricsLastUpdated *time.Time            `bun:"metrics_last_updated"                         json:"metrics_last_updated"`
	LastRefreshedAt    time.Time             `bun:"last_refreshed_at,notnull"                    json:"last_refreshed_at"`
	RawJSON            string                `bun:"raw_json,notnull"                             json:"-"`
	CreatedAt          time.Time             `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt          time.Time             `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}

// Metrics assembles the denormalised aggregate columns into the response
// block (CON-93 §7).
func (a *PostAnalytics) Metrics() PostAnalyticsMetrics {
	return PostAnalyticsMetrics{
		Impressions:    a.Impressions,
		Reach:          a.Reach,
		Likes:          a.Likes,
		Comments:       a.Comments,
		Shares:         a.Shares,
		Saves:          a.Saves,
		Clicks:         a.Clicks,
		Views:          a.Views,
		EngagementRate: a.EngagementRate,
	}
}
