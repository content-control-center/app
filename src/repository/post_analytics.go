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

// PostAnalyticsListItem is one row of the overview, read from the current-state
// table (CON-236: one row per post, no latest-per-post subquery) and shaped to
// the §7 response contract (aggregate metrics nested under `analytics`).
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

// PostAnalyticsRepository persists analytics for Posts published through a
// publisher (CON-93; storage reworked in CON-236, isolated analytics DB). The
// current state is ONE upserted row per post (post_analytics_current); the
// append-only trend history is a separate table written only on change
// (post_analytics_snapshots). Reads serve entirely from the current-state row.
type PostAnalyticsRepository interface {
	// Upsert writes the current state for a post, keyed on (tenant_id, post_id):
	// the per-check write refreshes the metrics/display fields in place rather
	// than appending. tenant_id is stamped by the TenantScoped hook. Used on the
	// unchanged path (bump last_checked_at only).
	Upsert(ctx context.Context, a *models.PostAnalytics) error
	// AppendSnapshot appends one point to the trend history (CON-236: only when
	// the metrics changed). ID + OccurredAt are set by the caller.
	AppendSnapshot(ctx context.Context, s *models.PostAnalyticsSnapshot) error
	// UpsertWithSnapshot writes the current-state row and appends the trend point
	// in ONE transaction (CON-236): on a changed refresh both commit together, so
	// a snapshot failure can't leave the current row advanced without its history
	// point (which dedup would then hide forever). Use on the changed path.
	UpsertWithSnapshot(ctx context.Context, a *models.PostAnalytics, s *models.PostAnalyticsSnapshot) error
	// GetByPostID returns the current-state row for a post, or (nil, nil) when
	// the post has no snapshot yet (refresh hasn't covered it).
	GetByPostID(ctx context.Context, postID string) (*models.PostAnalytics, error)
	// CurrentByPostID returns the current-state rows for the ctx tenant keyed by
	// post_id, so the refresh job can dedup (compare metric keys) and apply the
	// decay schedule (last_checked_at) without a per-post query.
	CurrentByPostID(ctx context.Context) (map[string]*models.PostAnalytics, error)
	// List returns one page of current-state rows plus the overview computed over
	// the full filtered set and the total count.
	List(ctx context.Context, opts PostAnalyticsListOptions) ([]PostAnalyticsListItem, PostAnalyticsOverview, error)
}

type postAnalyticsRepository struct {
	db *bun.DB
}

// NewPostAnalyticsRepository builds a PostAnalyticsRepository. Pass the
// analytics *bun.DB (post_analytics_current / post_analytics_snapshots live there).
func NewPostAnalyticsRepository(db *bun.DB) PostAnalyticsRepository {
	return &postAnalyticsRepository{db: db}
}

func (r *postAnalyticsRepository) Upsert(ctx context.Context, a *models.PostAnalytics) error {
	_, err := upsertCurrentQuery(r.db, a).Exec(ctx)
	return err
}

// upsertCurrentQuery builds the (tenant_id, post_id) current-state upsert on the
// given db/tx. tenant_id is stamped per row by the TenantScoped BeforeAppendModel
// hook, so the conflict target always has a value.
func upsertCurrentQuery(idb bun.IDB, a *models.PostAnalytics) *bun.InsertQuery {
	return idb.NewInsert().Model(a).
		On("CONFLICT (tenant_id, post_id) DO UPDATE").
		Set("publisher = EXCLUDED.publisher").
		Set("publisher_post_id = EXCLUDED.publisher_post_id").
		Set("platform = EXCLUDED.platform").
		Set("title = EXCLUDED.title").
		Set("published_at = EXCLUDED.published_at").
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
		Set("last_changed_at = EXCLUDED.last_changed_at").
		Set("last_checked_at = EXCLUDED.last_checked_at")
}

func (r *postAnalyticsRepository) AppendSnapshot(ctx context.Context, s *models.PostAnalyticsSnapshot) error {
	_, err := r.db.NewInsert().Model(s).Exec(ctx)
	return err
}

func (r *postAnalyticsRepository) UpsertWithSnapshot(ctx context.Context, a *models.PostAnalytics, s *models.PostAnalyticsSnapshot) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := upsertCurrentQuery(tx, a).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewInsert().Model(s).Exec(ctx); err != nil {
			return err
		}
		return nil
	})
}

func (r *postAnalyticsRepository) GetByPostID(ctx context.Context, postID string) (*models.PostAnalytics, error) {
	a := new(models.PostAnalytics)
	err := r.db.NewSelect().
		Model(a).
		Where("pa.post_id = ?", postID).
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

func (r *postAnalyticsRepository) CurrentByPostID(ctx context.Context) (map[string]*models.PostAnalytics, error) {
	var rows []models.PostAnalytics
	// TenantScoped hook restricts to the ctx tenant.
	if err := r.db.NewSelect().Model(&rows).Scan(ctx); err != nil {
		return nil, err
	}
	out := make(map[string]*models.PostAnalytics, len(rows))
	for i := range rows {
		out[rows[i].PostID] = &rows[i]
	}
	return out, nil
}

// postAnalyticsSortColumns whitelists the §5 sort_by values, mapping each to a
// fully-qualified column on the current-state table. Keeping the map closed
// prevents arbitrary expressions reaching ORDER BY.
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

// postAnalyticsListRow is the flat scan target for the current-state read.
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

	// filtered builds a fresh Model query over the current-state table with the
	// publisher / platform filters. It stays a Model query so the TenantScoped
	// hook adds `pa.tenant_id = <ctx tenant>`. One row per post already, so no
	// latest-per-post subquery is needed (CON-236).
	filtered := func() *bun.SelectQuery {
		q := r.db.NewSelect().Model((*models.PostAnalytics)(nil))
		if opts.Publisher != "" {
			q = q.Where("pa.publisher = ?", opts.Publisher)
		}
		if opts.Platform != "" {
			q = q.Where("pa.platform = ?", opts.Platform)
		}
		return q
	}

	// Overview + total over the full filtered set.
	var overview PostAnalyticsOverview
	err := filtered().
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
	err = filtered().
		ColumnExpr("pa.post_id, pa.publisher_post_id, pa.sync_status, pa.metrics_last_updated, pa.last_checked_at AS last_refreshed_at").
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
