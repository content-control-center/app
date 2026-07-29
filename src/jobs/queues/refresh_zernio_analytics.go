package queues

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/riverqueue/river"

	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/jobs"
	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/publishers/zernio"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// RefreshZernioAnalyticsQueue is the recurring analytics-refresh queue
// (CON-93 §6 FR3). Each tick batch-pages Zernio's analytics list
// (source=late), matches returned items back to local posts by
// publisher_post_id, and upserts a snapshot per matched post. It mirrors
// ReconcileScheduledPostsQueue: a marker payload, self-rescheduling at a
// fixed cadence, seeded once at boot.
const RefreshZernioAnalyticsQueue = "refresh_zernio_analytics"

// Defaults applied when the corresponding processor tunable is zero.
const (
	defaultAnalyticsWindowDays = 90
	defaultAnalyticsPageLimit  = 100
	// maxAnalyticsPages hard-caps the pages fetched per tick so a large
	// back-catalog can't make one tick unbounded; the uncovered tail is
	// picked up on subsequent ticks (CON-93 §10).
	maxAnalyticsPages = 20
)

// RefreshZernioAnalyticsTask is a marker payload — like
// ReconcileScheduledPostsTask, this queue carries no per-tick data.
type RefreshZernioAnalyticsTask struct{}

// Kind implements river.JobArgs.
func (RefreshZernioAnalyticsTask) Kind() string { return RefreshZernioAnalyticsQueue }

// InsertOpts sets per-kind defaults. Cadence is owned by a River PeriodicJob
// (registered when the Zernio integration is configured). Process never
// returns an error — each tick records its own outcome — so a single attempt
// is correct; idempotent upserts keep a re-run safe (§10). UniqueOpts (active
// states only) prevents overlapping ticks from stacking.
func (RefreshZernioAnalyticsTask) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, UniqueOpts: periodicUniqueOpts()}
}

// RefreshZernioAnalyticsProcessor wires one refresh tick. Deps supplies
// the post repo (match map), the analytics repo (upsert target), and the
// Zernio client. Settings records the zernio.analytics.* health keys. Hub
// is optional (CON-93 §8, frontend-deferred) — a nil Hub publishes
// nothing.
type RefreshZernioAnalyticsProcessor struct {
	river.WorkerDefaults[RefreshZernioAnalyticsTask]
	Deps     ZernioDeps
	Settings zernio.SettingsStore
	Hub      eventhub.Hub

	WindowDays int // analytics lookback window (defaults to 90)
	PageLimit  int // page size, 1–100 (defaults to 100)
}

// Work is the River entrypoint; it delegates to Process.
func (p *RefreshZernioAnalyticsProcessor) Work(ctx context.Context, job *river.Job[RefreshZernioAnalyticsTask]) error {
	ctx = WithJobRequestID(ctx, job.JobRow)
	// CON-97: background jobs span tenants (interim until per-tenant, PR4).
	ctx = tenantctx.WithSystem(ctx)
	return p.Process(ctx, job.Args)
}

// Timeout is the per-attempt context deadline.
func (p *RefreshZernioAnalyticsProcessor) Timeout(*river.Job[RefreshZernioAnalyticsTask]) time.Duration {
	return 60 * time.Second
}

func init() {
	register(func(w *river.Workers, d Deps) {
		river.AddWorker(w, &RefreshZernioAnalyticsProcessor{
			Deps:       d.Zernio,
			Settings:   d.AnalyticsSettings,
			Hub:        d.AnalyticsHub,
			WindowDays: d.AnalyticsWindowDays,
		})
	})
}

// Process runs one refresh tick across every tenant that owns Zernio-published
// posts. Per-tenant health (zernio.analytics.last_refresh_*) is recorded inside
// refresh under each tenant's context; the aggregate error here only drives the
// tick-level metric and log line.
func (p *RefreshZernioAnalyticsProcessor) Process(ctx context.Context, _ RefreshZernioAnalyticsTask) error {
	tickStart := time.Now()
	upserts, err := p.refresh(ctx, tickStart.UTC())

	if err != nil {
		// Any failure (transient 429/5xx/network, 401, legacy 402, terminal
		// 4xx) is recorded and logged; the tick still reschedules below and
		// returns nil. Transient failures recover on the next cadence tick
		// rather than via a River retry — see InsertOpts for why returning an
		// error here would risk killing or fanning out the recurring chain.
		jobs.ZernioAnalyticsRefreshFailed.Add(1)
		slog.ErrorContext(ctx, "analytics refresh failed", logging.AttrComponent, "jobs.refresh_analytics", "upserts", upserts, "tick_ms", time.Since(tickStart).Milliseconds(), logging.AttrError, err)
	} else {
		jobs.ZernioAnalyticsRefreshSucceeded.Add(1)
		jobs.ZernioAnalyticsPostsUpserted.Add(int64(upserts))
		slog.InfoContext(ctx, "analytics refresh ok", logging.AttrComponent, "jobs.refresh_analytics", "upserts", upserts, "tick_ms", time.Since(tickStart).Milliseconds())
	}
	return nil
}

// refresh sweeps analytics once per tenant that owns Zernio-published posts,
// scoping each fetch to that tenant's Zernio profile (CON-93 follow-up:
// per-profile collection). A single cross-tenant read of the published-post
// projection yields both the set of tenants to sweep and each tenant's match
// map. Per-tenant health is recorded under that tenant's context; the returned
// error is the first tenant-level failure — Process uses it only for the
// tick-level metric, never propagating to River.
func (p *RefreshZernioAnalyticsProcessor) refresh(ctx context.Context, now time.Time) (int, error) {
	if p.Deps.AnalyticsRepo == nil || p.Deps.PostRepo == nil {
		return 0, errors.New("refresh: analytics dependencies not configured")
	}

	// Enumerate under a system context so the scoping hook doesn't restrict the
	// projection to one tenant — this single read spans every tenant.
	posts, err := p.Deps.PostRepo.ListWithPublisherPostID(tenantctx.WithSystem(ctx))
	if err != nil {
		return 0, fmt.Errorf("refresh: list posts: %w", err)
	}
	if len(posts) == 0 {
		return 0, nil
	}
	platformName := p.platformNames(tenantctx.WithSystem(ctx))

	byTenant := make(map[string][]models.Post)
	for _, post := range posts {
		byTenant[post.TenantID] = append(byTenant[post.TenantID], post)
	}

	total := 0
	var firstErr error
	for _, tenantID := range sortedTenantIDs(byTenant) {
		tctx := tenantctx.With(ctx, tenantID)
		n, terr := p.refreshTenant(tctx, byTenant[tenantID], platformName, now)
		total += n
		p.recordStatus(tctx, terr)
		if terr != nil {
			if firstErr == nil {
				firstErr = terr
			}
			slog.ErrorContext(tctx, "analytics refresh: tenant sweep failed", logging.AttrComponent, "jobs.refresh_analytics", "tenant_id", tenantID, logging.AttrError, terr)
		}
	}
	return total, firstErr
}

// refreshTenant fetches and upserts one tenant's analytics. ctx is already
// tenant-scoped, so the Zernio fetch is filtered to this tenant's profile and
// every write/event lands in the right tenant. A tenant with Zernio-published
// posts but no profile id recorded is skipped (0, nil) rather than falling back
// to an unscoped cross-profile fetch.
func (p *RefreshZernioAnalyticsProcessor) refreshTenant(ctx context.Context, posts []models.Post, platformName map[string]string, now time.Time) (int, error) {
	profileID := ""
	if p.Deps.ProfileID != nil {
		id, err := p.Deps.ProfileID(ctx)
		if err != nil {
			return 0, fmt.Errorf("resolve profile id: %w", err)
		}
		profileID = id
	}
	if profileID == "" {
		return 0, nil
	}

	// publisher_post_id → post_id match map for this tenant, plus the per-post
	// fields denormalised onto the snapshot. Zernio returns analytics for every
	// late post under the profile; we write only the ones Ogen owns.
	byPublisherID := make(map[string]string, len(posts))
	postByID := make(map[string]models.Post, len(posts))
	for _, post := range posts {
		byPublisherID[post.PublisherPostID] = post.ID
		postByID[post.ID] = post
	}

	from := now.AddDate(0, 0, -p.windowDays()).Format("2006-01-02")
	limit := p.pageLimit()

	upserts := 0
	for page := 1; page <= maxAnalyticsPages; page++ {
		apiStart := time.Now()
		items, pagination, lerr := p.Deps.Client.ListAnalytics(ctx, zernio.AnalyticsQuery{
			Source:    zernio.AnalyticsSourceLate,
			ProfileID: profileID,
			FromDate:  from,
			Limit:     limit,
			Page:      page,
		})
		jobs.ObserveZernioCall(time.Since(apiStart))
		if lerr != nil {
			return upserts, lerr
		}

		for i := range items {
			postID, publisherPostID := p.matchPostID(byPublisherID, &items[i])
			if postID == "" {
				continue
			}
			post := postByID[postID]
			snapshot, berr := buildSnapshot(post, publisherPostID, platformName[post.PlatformID], &items[i], now)
			if berr != nil {
				slog.ErrorContext(ctx, "analytics snapshot build failed", logging.AttrComponent, "jobs.refresh_analytics", "post_id", postID, logging.AttrError, berr)
				continue
			}
			if uerr := p.Deps.AnalyticsRepo.Insert(ctx, snapshot); uerr != nil {
				slog.ErrorContext(ctx, "analytics insert failed", logging.AttrComponent, "jobs.refresh_analytics", "post_id", postID, logging.AttrError, uerr)
				continue
			}
			upserts++
			p.publishUpdated(ctx, snapshot)
		}

		// Stop paging on the last page, robust to whichever pagination shape the
		// endpoint returns: an explicit last-page number, the cursor-style
		// hasMore flag, or — absent both — a short/empty page.
		if len(items) == 0 {
			break
		}
		if last := pagination.LastPage(); last > 0 {
			if page >= last {
				break
			}
			continue
		}
		if !pagination.HasMore && len(items) < limit {
			break
		}
	}
	return upserts, nil
}

// platformNames resolves platform_id → display name once per tick (platforms is
// a small seeded reference table) so each snapshot can carry the denormalised
// platform name (CON-125 Track B). Best-effort: a lookup failure leaves names
// empty rather than failing the tick.
func (p *RefreshZernioAnalyticsProcessor) platformNames(ctx context.Context) map[string]string {
	names := map[string]string{}
	if p.Deps.PlatformRepo == nil {
		return names
	}
	plats, err := p.Deps.PlatformRepo.List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "analytics refresh: platform list failed; platform names left empty", logging.AttrComponent, "jobs.refresh_analytics", logging.AttrError, err)
		return names
	}
	for _, pl := range plats {
		names[pl.ID] = pl.Name
	}
	return names
}

// sortedTenantIDs returns the map keys in deterministic order so per-tenant
// sweeps (and their logs/tests) run in a stable sequence.
func sortedTenantIDs(byTenant map[string][]models.Post) []string {
	ids := make([]string, 0, len(byTenant))
	for id := range byTenant {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// matchPostID resolves a returned item to a local post by either of the
// two ids Zernio exposes (CON-93 §6 step 2). It returns the local post
// id plus the publisher_post_id that matched — that matched key (not
// necessarily item.PostID) is what gets denormalised onto the snapshot,
// so it stays consistent with posts.publisher_post_id.
func (p *RefreshZernioAnalyticsProcessor) matchPostID(byPublisherID map[string]string, item *zernio.AnalyticsItem) (postID, publisherPostID string) {
	if item.PostID != "" {
		if id, ok := byPublisherID[item.PostID]; ok {
			return id, item.PostID
		}
	}
	if item.LatePostID != "" {
		if id, ok := byPublisherID[item.LatePostID]; ok {
			return id, item.LatePostID
		}
	}
	return "", ""
}

// buildSnapshot maps a Zernio analytics item onto a new append-only snapshot
// row (CON-125 Track B). publisherPostID is the matched key from the local post
// (kept consistent with posts.publisher_post_id); platformName is the resolved
// display name for the post's platform. The post's publisher/title/published_at
// are denormalised so the overview read needs no cross-DB join. A fresh id and
// occurred_at (the refresh time) make each tick a distinct row.
func buildSnapshot(post models.Post, publisherPostID, platformName string, item *zernio.AnalyticsItem, now time.Time) (*models.PostAnalytics, error) {
	id, err := models.NewID()
	if err != nil {
		return nil, err
	}
	a := &models.PostAnalytics{
		ID:                 id,
		PostID:             post.ID,
		PublisherPostID:    publisherPostID,
		Publisher:          post.Publisher,
		Platform:           platformName,
		Title:              post.Title,
		PublishedAt:        post.PublishedAt,
		Impressions:        item.Analytics.Impressions,
		Reach:              item.Analytics.Reach,
		Likes:              item.Analytics.Likes,
		Comments:           item.Analytics.Comments,
		Shares:             item.Analytics.Shares,
		Saves:              item.Analytics.Saves,
		Clicks:             item.Analytics.Clicks,
		Views:              item.Analytics.Views,
		EngagementRate:     item.Analytics.EngagementRate,
		PlatformAnalytics:  mapPlatformAnalytics(item.PlatformAnalytics),
		SyncStatus:         item.SyncStatus,
		MetricsLastUpdated: item.Analytics.LastUpdated,
		RawJSON:            rawOrEmpty(item.Raw),
		OccurredAt:         now,
	}
	return a, nil
}

func mapPlatformAnalytics(in []zernio.PlatformAnalytics) models.PlatformAnalyticsList {
	out := make(models.PlatformAnalyticsList, 0, len(in))
	for _, pa := range in {
		out = append(out, models.PostPlatformAnalytics{
			Platform:        pa.Platform,
			Status:          pa.Status,
			PlatformPostID:  pa.PlatformPostID,
			AccountID:       pa.AccountID,
			AccountUsername: pa.AccountUsername,
			PlatformPostURL: pa.PlatformPostURL,
			SyncStatus:      pa.SyncStatus,
			ErrorMessage:    pa.ErrorMessage,
			ReauthorizeURL:  pa.ReauthorizeURL,
			Analytics: models.PostAnalyticsMetrics{
				Impressions:    pa.Analytics.Impressions,
				Reach:          pa.Analytics.Reach,
				Likes:          pa.Analytics.Likes,
				Comments:       pa.Analytics.Comments,
				Shares:         pa.Analytics.Shares,
				Saves:          pa.Analytics.Saves,
				Clicks:         pa.Analytics.Clicks,
				Views:          pa.Analytics.Views,
				EngagementRate: pa.Analytics.EngagementRate,
			},
		})
	}
	return out
}

func rawOrEmpty(raw []byte) string {
	if len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}

// publishUpdated emits the optional post.analytics.updated event
// (CON-93 §8). No-op when no Hub is wired.
func (p *RefreshZernioAnalyticsProcessor) publishUpdated(ctx context.Context, a *models.PostAnalytics) {
	if p.Hub == nil {
		return
	}
	if err := p.Hub.Publish(ctx, eventhub.Event{
		Topic:    fmt.Sprintf("entity:post:%s", a.PostID),
		TenantID: a.TenantID,
		Type:     "post.analytics.updated",
		Payload: map[string]any{
			"post_id":     a.PostID,
			"sync_status": a.SyncStatus,
			"analytics":   a.Metrics(),
		},
	}); err != nil {
		slog.ErrorContext(ctx, "analytics publish event failed", logging.AttrComponent, "jobs.refresh_analytics", logging.AttrError, err)
	}
}

// recordStatus writes the documented zernio.analytics.last_refresh_at /
// last_refresh_status settings. Best-effort: write failures are logged,
// not propagated.
func (p *RefreshZernioAnalyticsProcessor) recordStatus(ctx context.Context, refreshErr error) {
	if p.Settings == nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := p.Settings.Set(ctx, zernio.SettingAnalyticsLastRefreshAt, now); err != nil {
		slog.ErrorContext(ctx, "analytics settings write failed", logging.AttrComponent, "jobs.refresh_analytics", "setting", zernio.SettingAnalyticsLastRefreshAt, logging.AttrError, err)
	}
	status := zernio.SyncStatusOK
	if refreshErr != nil {
		status = "error: " + truncateAnalyticsErr(refreshErr.Error())
	}
	if err := p.Settings.Set(ctx, zernio.SettingAnalyticsLastRefreshStatus, status); err != nil {
		slog.ErrorContext(ctx, "analytics settings write failed", logging.AttrComponent, "jobs.refresh_analytics", "setting", zernio.SettingAnalyticsLastRefreshStatus, logging.AttrError, err)
	}
}

func (p *RefreshZernioAnalyticsProcessor) windowDays() int {
	if p.WindowDays > 0 {
		return p.WindowDays
	}
	return defaultAnalyticsWindowDays
}

func (p *RefreshZernioAnalyticsProcessor) pageLimit() int {
	if p.PageLimit > 0 && p.PageLimit <= 100 {
		return p.PageLimit
	}
	return defaultAnalyticsPageLimit
}

func truncateAnalyticsErr(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
