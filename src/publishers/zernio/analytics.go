package zernio

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ErrAnalyticsUnavailable maps Zernio's legacy 402 ("analytics add-on
// required") to a typed sentinel. Under Zernio's current bundled
// pricing — analytics is on every tier, re-verified 2026-06-15 — this
// is never expected; the refresh queue handles it defensively only
// (CON-93 §2/§9). Kept as a sentinel so that branch is explicit rather
// than a bare status check.
var ErrAnalyticsUnavailable = errors.New("zernio: analytics unavailable (legacy add-on required)")

// Analytics source filters (CON-93). Ogen only ever requests `late` —
// posts Zernio itself published, which is the entire CON-93 scope. The
// `external`/`all` values are listed for completeness; analytics for
// externally-synced/legacy posts is a separate, future issue.
const (
	AnalyticsSourceLate     = "late"
	AnalyticsSourceExternal = "external"
	AnalyticsSourceAll      = "all"
)

// AnalyticsQuery holds the documented GET /analytics query params. A
// zero-value field is omitted from the request so Zernio's own defaults
// apply (source=all, last 90d, limit=50, page=1, sortBy=date,
// order=desc). The refresh queue always sets Source=late.
type AnalyticsQuery struct {
	Source    string // late | external | all
	Platform  string
	ProfileID string
	AccountID string
	PostID    string // single-post filter (?postId=)
	FromDate  string // YYYY-MM-DD
	ToDate    string // YYYY-MM-DD
	SortBy    string // date|engagement|impressions|reach|likes|comments|shares|saves|clicks|views
	Order     string // asc|desc
	Limit     int    // 1–100
	Page      int    // 1-based
}

func (q AnalyticsQuery) values() url.Values {
	v := url.Values{}
	set := func(k, val string) {
		if val != "" {
			v.Set(k, val)
		}
	}
	set("source", q.Source)
	set("platform", q.Platform)
	set("profileId", q.ProfileID)
	set("accountId", q.AccountID)
	set("postId", q.PostID)
	set("fromDate", q.FromDate)
	set("toDate", q.ToDate)
	set("sortBy", q.SortBy)
	set("order", q.Order)
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.Page > 0 {
		v.Set("page", strconv.Itoa(q.Page))
	}
	return v
}

// Metrics is Zernio's aggregate engagement block (`analytics{}`), shared
// by the post-level rollup and each per-platform breakdown. Field names
// mirror Zernio's exactly. IGReels* are Instagram-only and zero
// elsewhere. LastUpdated is when Zernio last refreshed these numbers.
type Metrics struct {
	Impressions               int        `json:"impressions"`
	Reach                     int        `json:"reach"`
	Likes                     int        `json:"likes"`
	Comments                  int        `json:"comments"`
	Shares                    int        `json:"shares"`
	Saves                     int        `json:"saves"`
	Clicks                    int        `json:"clicks"`
	Views                     int        `json:"views"`
	IGReelsAvgWatchTime       float64    `json:"igReelsAvgWatchTime,omitempty"`
	IGReelsVideoViewTotalTime float64    `json:"igReelsVideoViewTotalTime,omitempty"`
	EngagementRate            float64    `json:"engagementRate"`
	LastUpdated               *time.Time `json:"lastUpdated,omitempty"`
}

// PlatformAnalytics is one per-platform row inside an AnalyticsItem.
// SyncStatus / ErrorMessage / ReauthorizeURL carry the per-platform
// scope-gap nuance (412/reauthorizeUrl, CON-93 §9/§10): a platform
// connected before analytics scopes were granted reports an error here
// without failing the whole item; platforms that do report still carry
// metrics.
type PlatformAnalytics struct {
	Platform        string  `json:"platform"`
	Status          string  `json:"status,omitempty"`
	PlatformPostID  string  `json:"platformPostId,omitempty"`
	AccountID       string  `json:"accountId,omitempty"`
	AccountUsername string  `json:"accountUsername,omitempty"`
	PlatformPostURL string  `json:"platformPostUrl,omitempty"`
	SyncStatus      string  `json:"syncStatus,omitempty"`
	ErrorMessage    string  `json:"errorMessage,omitempty"`
	ReauthorizeURL  string  `json:"reauthorizeUrl,omitempty"`
	Analytics       Metrics `json:"analytics"`
}

// AnalyticsItem is one post's analytics as returned by GET /analytics.
// PostID / LatePostID are the join keys back to a local
// posts.publisher_post_id. Raw retains the verbatim item for
// forward-compat, exactly as Account/Profile do.
type AnalyticsItem struct {
	PostID            string              `json:"postId"`
	LatePostID        string              `json:"latePostId,omitempty"`
	Status            string              `json:"status,omitempty"`
	Content           string              `json:"content,omitempty"`
	ScheduledFor      *time.Time          `json:"scheduledFor,omitempty"`
	PublishedAt       *time.Time          `json:"publishedAt,omitempty"`
	Platform          string              `json:"platform,omitempty"`
	PlatformPostURL   string              `json:"platformPostUrl,omitempty"`
	IsExternal        bool                `json:"isExternal"`
	SyncStatus        string              `json:"syncStatus,omitempty"`
	Message           string              `json:"message,omitempty"`
	ThumbnailURL      string              `json:"thumbnailUrl,omitempty"`
	MediaType         string              `json:"mediaType,omitempty"`
	Analytics         Metrics             `json:"analytics"`
	PlatformAnalytics []PlatformAnalytics `json:"platformAnalytics,omitempty"`

	// Raw is the verbatim Zernio item, preserved so a future field can
	// be read without a client change. Not part of the JSON contract.
	Raw json.RawMessage `json:"-"`
}

// AnalyticsPagination mirrors Zernio's standard pagination block (the
// same shape posts list uses).
type AnalyticsPagination struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
	Pages int `json:"pages"`
}

// listAnalyticsEnvelope mirrors Zernio's list-response shape:
// `{"analytics": [...], "pagination": {...}}`. The single-key envelope
// is consistent across every Zernio list endpoint (profiles, accounts,
// posts). Items are decoded individually so each retains its Raw bytes.
type listAnalyticsEnvelope struct {
	Analytics  []json.RawMessage   `json:"analytics"`
	Pagination AnalyticsPagination `json:"pagination"`
}

// ListAnalytics fetches one page of post analytics from GET /analytics,
// mirroring Status. The refresh queue pages through with Source=late.
//
// The legacy 402 is translated to ErrAnalyticsUnavailable; all other
// non-2xx responses surface as the usual *APIError so the worker can
// reuse IsStatus(err, 401|429).
func (c *Client) ListAnalytics(ctx context.Context, q AnalyticsQuery) ([]AnalyticsItem, AnalyticsPagination, error) {
	if c == nil {
		return nil, AnalyticsPagination{}, errors.New("zernio: client is disabled")
	}
	var env listAnalyticsEnvelope
	if err := c.do(ctx, http.MethodGet, "/analytics", q.values(), nil, &env); err != nil {
		if IsStatus(err, http.StatusPaymentRequired) {
			return nil, AnalyticsPagination{}, ErrAnalyticsUnavailable
		}
		return nil, AnalyticsPagination{}, err
	}
	out := make([]AnalyticsItem, 0, len(env.Analytics))
	for _, raw := range env.Analytics {
		item, err := decodeAnalyticsItem(raw)
		if err != nil {
			return nil, AnalyticsPagination{}, err
		}
		out = append(out, *item)
	}
	return out, env.Pagination, nil
}

func decodeAnalyticsItem(raw json.RawMessage) (*AnalyticsItem, error) {
	var item AnalyticsItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, err
	}
	item.Raw = append(json.RawMessage(nil), raw...)
	return &item, nil
}
