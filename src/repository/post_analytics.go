package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
)

// PostAnalyticsListOptions parameterises the list/overview query
// (CON-93 §5/§7). Publisher restricts the result to one publisher's
// posts (the analytics overview is "Zernio-published posts" today);
// empty means no publisher filter. Platform optionally narrows to one
// platform by name. SortBy/Order are validated/clamped by the caller and
// mapped to a whitelisted column here.
type PostAnalyticsListOptions struct {
	Publisher string
	Platform  string
	Page      int
	Limit     int
	SortBy    string
	Order     string
}

// PostAnalyticsListItem is one row of the overview, joined to the owning
// post for title/platform/published_at and shaped to the §7 response
// contract (aggregate metrics nested under `analytics`).
type PostAnalyticsListItem struct {
	PostID             string                      `json:"post_id"`
	PublisherPostID    string                      `json:"publisher_post_id"`
	Title              string                      `json:"title"`
	Publisher          string                      `json:"publisher"`
	Platform           string                      `json:"platform"`
	PublishedAt        *time.Time                  `json:"published_at"`
	SyncStatus         string                      `json:"sync_status"`
	MetricsLastUpdated *time.Time                  `json:"metrics_last_updated"`
	LastRefreshedAt    time.Time                   `json:"last_refreshed_at"`
	Analytics          models.PostAnalyticsMetrics `json:"analytics"`
}

// PostAnalyticsOverview is the summed/averaged headline block over the
// full filtered set (not just the current page), per §7.
type PostAnalyticsOverview struct {
	PostCount         int     `json:"post_count"`
	Impressions       int     `json:"impressions"`
	Reach             int     `json:"reach"`
	Likes             int     `json:"likes"`
	Comments          int     `json:"comments"`
	Shares            int     `json:"shares"`
	Saves             int     `json:"saves"`
	Clicks            int     `json:"clicks"`
	Views             int     `json:"views"`
	EngagementRateAvg float64 `json:"engagement_rate_avg"`
}

// PostAnalyticsRepository persists the latest analytics snapshot per Post
// (CON-93 §6 FR2). The table is 1:1 with Post via the post_id PK, so
// Upsert overwrites the prior snapshot rather than appending.
type PostAnalyticsRepository interface {
	// Upsert inserts the snapshot, or overwrites the existing row for the
	// same post_id. updated_at is stamped here.
	Upsert(ctx context.Context, a *models.PostAnalytics) error
	// GetByPostID returns the snapshot for a post, or (nil, nil) when the
	// post has no snapshot yet (refresh hasn't covered it).
	GetByPostID(ctx context.Context, postID string) (*models.PostAnalytics, error)
	// List returns one page of snapshots joined to their owning posts,
	// plus the overview computed over the full filtered set and the total
	// row count for pagination.
	List(ctx context.Context, opts PostAnalyticsListOptions) ([]PostAnalyticsListItem, PostAnalyticsOverview, error)
}

type postAnalyticsRepository struct {
	db *bun.DB
}

func NewPostAnalyticsRepository(db *bun.DB) PostAnalyticsRepository {
	return &postAnalyticsRepository{db: db}
}

func (r *postAnalyticsRepository) Upsert(ctx context.Context, a *models.PostAnalytics) error {
	a.UpdatedAt = time.Now().UTC()
	_, err := r.db.NewInsert().
		Model(a).
		On("CONFLICT (post_id) DO UPDATE").
		Set("publisher_post_id = EXCLUDED.publisher_post_id").
		Set("impressions = EXCLUDED.impressions").
		Set("reach = EXCLUDED.reach").
		Set("likes = EXCLUDED.likes").
		Set("comments = EXCLUDED.comments").
		Set("shares = EXCLUDED.shares").
		Set("saves = EXCLUDED.saves").
		Set("clicks = EXCLUDED.clicks").
		Set("views = EXCLUDED.views").
		Set("engagement_rate = EXCLUDED.engagement_rate").
		Set("platform_analytics = EXCLUDED.platform_analytics").
		Set("sync_status = EXCLUDED.sync_status").
		Set("metrics_last_updated = EXCLUDED.metrics_last_updated").
		Set("last_refreshed_at = EXCLUDED.last_refreshed_at").
		Set("raw_json = EXCLUDED.raw_json").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (r *postAnalyticsRepository) GetByPostID(ctx context.Context, postID string) (*models.PostAnalytics, error) {
	a := new(models.PostAnalytics)
	err := r.db.NewSelect().
		Model(a).
		Where("pa.post_id = ?", postID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

// postAnalyticsSortColumns whitelists the §5 sort_by values, mapping each
// to a fully-qualified column. Keeping the map closed prevents arbitrary
// expressions reaching ORDER BY.
var postAnalyticsSortColumns = map[string]string{
	"engagement":   "pa.engagement_rate",
	"impressions":  "pa.impressions",
	"reach":        "pa.reach",
	"likes":        "pa.likes",
	"comments":     "pa.comments",
	"shares":       "pa.shares",
	"saves":        "pa.saves",
	"clicks":       "pa.clicks",
	"views":        "pa.views",
	"published_at": "po.published_at",
}

// postAnalyticsListRow is the flat scan target for the join.
type postAnalyticsListRow struct {
	PostID             string     `bun:"post_id"`
	PublisherPostID    string     `bun:"publisher_post_id"`
	Title              string     `bun:"title"`
	Publisher          string     `bun:"publisher"`
	Platform           string     `bun:"platform"`
	PublishedAt        *time.Time `bun:"published_at"`
	SyncStatus         string     `bun:"sync_status"`
	MetricsLastUpdated *time.Time `bun:"metrics_last_updated"`
	LastRefreshedAt    time.Time  `bun:"last_refreshed_at"`
	Impressions        int        `bun:"impressions"`
	Reach              int        `bun:"reach"`
	Likes              int        `bun:"likes"`
	Comments           int        `bun:"comments"`
	Shares             int        `bun:"shares"`
	Saves              int        `bun:"saves"`
	Clicks             int        `bun:"clicks"`
	Views              int        `bun:"views"`
	EngagementRate     float64    `bun:"engagement_rate"`
}

func (r *postAnalyticsRepository) List(ctx context.Context, opts PostAnalyticsListOptions) ([]PostAnalyticsListItem, PostAnalyticsOverview, error) {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	sortCol, ok := postAnalyticsSortColumns[opts.SortBy]
	if !ok {
		sortCol = postAnalyticsSortColumns["engagement"]
	}
	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	// applyFilters narrows both the page query and the overview query to
	// the same set: snapshots whose owning post is published through the
	// requested publisher, optionally on one platform (by name).
	applyFilters := func(q *bun.SelectQuery) *bun.SelectQuery {
		q = q.
			Join("JOIN posts AS po ON po.id = pa.post_id").
			Join("LEFT JOIN platforms AS pl ON pl.id = po.platform_id")
		if opts.Publisher != "" {
			q = q.Where("po.publisher = ?", opts.Publisher)
		}
		if opts.Platform != "" {
			q = q.Where("pl.name = ?", opts.Platform)
		}
		return q
	}

	// Overview + total over the full filtered set.
	var overview PostAnalyticsOverview
	err := applyFilters(r.db.NewSelect().Model((*models.PostAnalytics)(nil))).
		ColumnExpr("COUNT(*) AS post_count").
		ColumnExpr("COALESCE(SUM(pa.impressions), 0) AS impressions").
		ColumnExpr("COALESCE(SUM(pa.reach), 0) AS reach").
		ColumnExpr("COALESCE(SUM(pa.likes), 0) AS likes").
		ColumnExpr("COALESCE(SUM(pa.comments), 0) AS comments").
		ColumnExpr("COALESCE(SUM(pa.shares), 0) AS shares").
		ColumnExpr("COALESCE(SUM(pa.saves), 0) AS saves").
		ColumnExpr("COALESCE(SUM(pa.clicks), 0) AS clicks").
		ColumnExpr("COALESCE(SUM(pa.views), 0) AS views").
		ColumnExpr("COALESCE(AVG(pa.engagement_rate), 0) AS engagement_rate_avg").
		Scan(ctx, &overview)
	if err != nil {
		return nil, PostAnalyticsOverview{}, err
	}

	if overview.PostCount == 0 {
		return []PostAnalyticsListItem{}, overview, nil
	}

	var rows []postAnalyticsListRow
	err = applyFilters(r.db.NewSelect().Model((*models.PostAnalytics)(nil))).
		ColumnExpr("pa.post_id, pa.publisher_post_id, pa.sync_status, pa.metrics_last_updated, pa.last_refreshed_at").
		ColumnExpr("pa.impressions, pa.reach, pa.likes, pa.comments, pa.shares, pa.saves, pa.clicks, pa.views, pa.engagement_rate").
		ColumnExpr("po.title AS title, po.publisher AS publisher, po.published_at AS published_at").
		ColumnExpr("COALESCE(pl.name, '') AS platform").
		OrderExpr(sortCol+" "+order).
		// Stable tiebreaker so paging is deterministic when the sort key ties.
		OrderExpr("pa.post_id ASC").
		Limit(limit).
		Offset((page-1)*limit).
		Scan(ctx, &rows)
	if err != nil {
		return nil, PostAnalyticsOverview{}, err
	}

	items := make([]PostAnalyticsListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, PostAnalyticsListItem{
			PostID:             row.PostID,
			PublisherPostID:    row.PublisherPostID,
			Title:              row.Title,
			Publisher:          row.Publisher,
			Platform:           row.Platform,
			PublishedAt:        row.PublishedAt,
			SyncStatus:         row.SyncStatus,
			MetricsLastUpdated: row.MetricsLastUpdated,
			LastRefreshedAt:    row.LastRefreshedAt,
			Analytics: models.PostAnalyticsMetrics{
				Impressions:    row.Impressions,
				Reach:          row.Reach,
				Likes:          row.Likes,
				Comments:       row.Comments,
				Shares:         row.Shares,
				Saves:          row.Saves,
				Clicks:         row.Clicks,
				Views:          row.Views,
				EngagementRate: row.EngagementRate,
			},
		})
	}
	return items, overview, nil
}
