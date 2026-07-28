package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/tenantctx"
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

// PostAnalyticsRepository persists analytics snapshots for Posts published
// through a publisher (CON-93 §6 FR2), migrated to an append-only time series
// in the isolated analytics DB (CON-125). Insert appends a new row per refresh;
// reads select the latest snapshot per post_id.
type PostAnalyticsRepository interface {
	// Insert appends one snapshot. ID + OccurredAt are set by the caller;
	// tenant_id is stamped by the TenantScoped hook from the write context.
	Insert(ctx context.Context, a *models.PostAnalytics) error
	// GetByPostID returns the LATEST snapshot for a post, or (nil, nil) when
	// the post has no snapshot yet (refresh hasn't covered it).
	GetByPostID(ctx context.Context, postID string) (*models.PostAnalytics, error)
	// List returns one page of latest-per-post snapshots (post display fields
	// read from the denormalised columns — no cross-DB join), plus the
	// overview computed over the full filtered latest set and the total count.
	List(ctx context.Context, opts PostAnalyticsListOptions) ([]PostAnalyticsListItem, PostAnalyticsOverview, error)
}

type postAnalyticsRepository struct {
	db *bun.DB
}

// NewPostAnalyticsRepository builds a PostAnalyticsRepository. Pass the
// analytics *bun.DB (the post_analytics_snapshots table lives there).
func NewPostAnalyticsRepository(db *bun.DB) PostAnalyticsRepository {
	return &postAnalyticsRepository{db: db}
}

func (r *postAnalyticsRepository) Insert(ctx context.Context, a *models.PostAnalytics) error {
	_, err := r.db.NewInsert().Model(a).Exec(ctx)
	return err
}

func (r *postAnalyticsRepository) GetByPostID(ctx context.Context, postID string) (*models.PostAnalytics, error) {
	a := new(models.PostAnalytics)
	err := r.db.NewSelect().
		Model(a).
		Where("pa.post_id = ?", postID).
		OrderExpr("pa.occurred_at DESC").
		Limit(1).
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
// expressions reaching ORDER BY. All columns are on the snapshot now
// (published_at is denormalised), so there is no join to sort against.
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
	"published_at": "pa.published_at",
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

	// latestForTenant builds a fresh Model query restricted to the LATEST
	// snapshot per post (append-only: many rows per post) plus the publisher /
	// platform filters. It stays a Model query so the TenantScoped hook adds
	// `pa.tenant_id = <ctx tenant>`; the correlated subquery re-scopes to
	// pa.tenant_id, so it never crosses tenants. Post display fields are the
	// denormalised columns on the snapshot — no join to posts/platforms (they
	// live in a different database now).
	//
	// The match is on the (occurred_at, id) tuple, not just MAX(occurred_at),
	// with an id tiebreaker: if two snapshots for a post share the maximum
	// occurred_at, a plain `= MAX(occurred_at)` would match BOTH and duplicate
	// the item / double-count the overview totals. Ordering the correlated pick
	// by (occurred_at DESC, id DESC) LIMIT 1 guarantees exactly one row per post.
	latestForTenant := func() *bun.SelectQuery {
		q := r.db.NewSelect().Model((*models.PostAnalytics)(nil)).
			Where("(pa.occurred_at, pa.id) = (SELECT s.occurred_at, s.id FROM post_analytics_snapshots AS s WHERE s.post_id = pa.post_id AND s.tenant_id = pa.tenant_id ORDER BY s.occurred_at DESC, s.id DESC LIMIT 1)")
		if opts.Publisher != "" {
			q = q.Where("pa.publisher = ?", opts.Publisher)
		}
		if opts.Platform != "" {
			q = q.Where("pa.platform = ?", opts.Platform)
		}
		return q
	}

	// Overview + total over the full filtered latest set.
	var overview PostAnalyticsOverview
	err := latestForTenant().
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
	err = latestForTenant().
		ColumnExpr("pa.post_id, pa.publisher_post_id, pa.sync_status, pa.metrics_last_updated, pa.occurred_at AS last_refreshed_at").
		ColumnExpr("pa.impressions, pa.reach, pa.likes, pa.comments, pa.shares, pa.saves, pa.clicks, pa.views, pa.engagement_rate").
		ColumnExpr("pa.title AS title, pa.publisher AS publisher, pa.published_at AS published_at").
		ColumnExpr("COALESCE(pa.platform, '') AS platform").
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

// BackfillPostAnalytics is a one-time, idempotent copy of the legacy main-DB
// post_analytics snapshots into the analytics-DB post_analytics_snapshots table
// (CON-125 Track B, migration §9 step 4). It denormalises each post's
// publisher/platform/title/published_at via a join in the main DB (where those
// tables live), stamps occurred_at from the legacy last_refreshed_at, and skips
// any post that already has a snapshot — so it is safe to re-run and races
// harmlessly with a live refresh. Returns the number of rows inserted.
//
// It reads the legacy table with raw SQL because models.PostAnalytics now maps
// to the NEW table. The read is cross-tenant (each row carries its tenant_id,
// preserved on insert under a system context). A missing legacy table (never
// created) surfaces as an error the caller treats as non-fatal.
func BackfillPostAnalytics(ctx context.Context, mainDB, analyticsDB *bun.DB) (int, error) {
	if mainDB == nil || analyticsDB == nil {
		return 0, nil
	}

	type legacyRow struct {
		TenantID           string                       `bun:"tenant_id"`
		PostID             string                       `bun:"post_id"`
		PublisherPostID    string                       `bun:"publisher_post_id"`
		Publisher          string                       `bun:"publisher"`
		Platform           string                       `bun:"platform"`
		Title              string                       `bun:"title"`
		PublishedAt        *time.Time                   `bun:"published_at"`
		Impressions        int                          `bun:"impressions"`
		Reach              int                          `bun:"reach"`
		Likes              int                          `bun:"likes"`
		Comments           int                          `bun:"comments"`
		Shares             int                          `bun:"shares"`
		Saves              int                          `bun:"saves"`
		Clicks             int                          `bun:"clicks"`
		Views              int                          `bun:"views"`
		EngagementRate     float64                      `bun:"engagement_rate"`
		PlatformAnalytics  models.PlatformAnalyticsList `bun:"platform_analytics"`
		SyncStatus         string                       `bun:"sync_status"`
		MetricsLastUpdated *time.Time                   `bun:"metrics_last_updated"`
		LastRefreshedAt    time.Time                    `bun:"last_refreshed_at"`
		RawJSON            string                       `bun:"raw_json"`
	}

	var legacy []legacyRow
	err := mainDB.NewRaw(`
		SELECT pa.tenant_id, pa.post_id, pa.publisher_post_id,
		       po.publisher AS publisher, COALESCE(pl.name, '') AS platform,
		       po.title AS title, po.published_at AS published_at,
		       pa.impressions, pa.reach, pa.likes, pa.comments, pa.shares,
		       pa.saves, pa.clicks, pa.views, pa.engagement_rate,
		       pa.platform_analytics, pa.sync_status, pa.metrics_last_updated,
		       pa.last_refreshed_at, pa.raw_json
		FROM post_analytics AS pa
		JOIN posts AS po ON po.id = pa.post_id
		LEFT JOIN platforms AS pl ON pl.id = po.platform_id
	`).Scan(ctx, &legacy)
	if err != nil {
		return 0, err
	}
	if len(legacy) == 0 {
		return 0, nil
	}

	// Idempotency: skip posts that already have a snapshot (already backfilled,
	// or a live refresh has since written one).
	var seenIDs []string
	if err := analyticsDB.NewRaw(`SELECT DISTINCT post_id FROM post_analytics_snapshots`).Scan(ctx, &seenIDs); err != nil {
		return 0, err
	}
	seen := make(map[string]bool, len(seenIDs))
	for _, id := range seenIDs {
		seen[id] = true
	}

	inserted := 0
	sysCtx := tenantctx.WithSystem(ctx)
	for i := range legacy {
		row := legacy[i]
		if seen[row.PostID] {
			continue
		}
		id, err := models.NewID()
		if err != nil {
			return inserted, err
		}
		snap := &models.PostAnalytics{
			ID:                 id,
			PostID:             row.PostID,
			PublisherPostID:    row.PublisherPostID,
			Publisher:          row.Publisher,
			Platform:           row.Platform,
			Title:              row.Title,
			PublishedAt:        row.PublishedAt,
			Impressions:        row.Impressions,
			Reach:              row.Reach,
			Likes:              row.Likes,
			Comments:           row.Comments,
			Shares:             row.Shares,
			Saves:              row.Saves,
			Clicks:             row.Clicks,
			Views:              row.Views,
			EngagementRate:     row.EngagementRate,
			PlatformAnalytics:  row.PlatformAnalytics,
			SyncStatus:         row.SyncStatus,
			MetricsLastUpdated: row.MetricsLastUpdated,
			RawJSON:            row.RawJSON,
			OccurredAt:         row.LastRefreshedAt,
		}
		snap.TenantID = row.TenantID // preserved by BeforeAppendModel in the system ctx
		if _, err := analyticsDB.NewInsert().Model(snap).Exec(sysCtx); err != nil {
			return inserted, err
		}
		seen[row.PostID] = true
		inserted++
	}
	return inserted, nil
}
