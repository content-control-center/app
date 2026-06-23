package queues

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/riverqueue/river"

	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/jobs"
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

// Process runs one refresh tick.
func (p *RefreshZernioAnalyticsProcessor) Process(ctx context.Context, _ RefreshZernioAnalyticsTask) error {
	tickStart := time.Now()
	upserts, err := p.refresh(ctx, tickStart.UTC())
	p.recordStatus(ctx, err)

	if err != nil {
		// Any failure (transient 429/5xx/network, 401, legacy 402, terminal
		// 4xx) is recorded and logged; the tick still reschedules below and
		// returns nil. Transient failures recover on the next cadence tick
		// rather than via a River retry — see InsertOpts for why returning an
		// error here would risk killing or fanning out the recurring chain.
		jobs.ZernioAnalyticsRefreshFailed.Add(1)
		log.Printf("zernio.analytics failed error=%q upserts=%d tick_ms=%d",
			err.Error(), upserts, time.Since(tickStart).Milliseconds())
	} else {
		jobs.ZernioAnalyticsRefreshSucceeded.Add(1)
		jobs.ZernioAnalyticsPostsUpserted.Add(int64(upserts))
		log.Printf("zernio.analytics ok upserts=%d tick_ms=%d",
			upserts, time.Since(tickStart).Milliseconds())
	}
	return nil
}

// refresh performs the batch fetch + match + upsert and returns the
// number of snapshots written. The returned error is the first
// page-level failure encountered; Process records it and reschedules
// regardless (it never propagates the error to River).
func (p *RefreshZernioAnalyticsProcessor) refresh(ctx context.Context, now time.Time) (int, error) {
	if p.Deps.AnalyticsRepo == nil || p.Deps.PostRepo == nil {
		return 0, errors.New("refresh: analytics dependencies not configured")
	}

	posts, err := p.Deps.PostRepo.ListWithPublisherPostID(ctx)
	if err != nil {
		return 0, fmt.Errorf("refresh: list posts: %w", err)
	}
	if len(posts) == 0 {
		return 0, nil
	}
	// publisher_post_id → post_id match map. Zernio returns analytics for
	// every late post on the account; we upsert only the ones Ogen owns.
	byPublisherID := make(map[string]string, len(posts))
	for _, post := range posts {
		byPublisherID[post.PublisherPostID] = post.ID
	}

	from := now.AddDate(0, 0, -p.windowDays()).Format("2006-01-02")
	limit := p.pageLimit()

	upserts := 0
	for page := 1; page <= maxAnalyticsPages; page++ {
		apiStart := time.Now()
		items, pagination, err := p.Deps.Client.ListAnalytics(ctx, zernio.AnalyticsQuery{
			Source:   zernio.AnalyticsSourceLate,
			FromDate: from,
			Limit:    limit,
			Page:     page,
		})
		jobs.ObserveZernioCall(time.Since(apiStart))
		if err != nil {
			return upserts, err
		}

		for i := range items {
			postID, publisherPostID := p.matchPostID(byPublisherID, &items[i])
			if postID == "" {
				continue
			}
			snapshot := buildSnapshot(postID, publisherPostID, &items[i], now)
			if uerr := p.Deps.AnalyticsRepo.Upsert(ctx, snapshot); uerr != nil {
				log.Printf("zernio.analytics upsert post_id=%s error=%q", postID, uerr.Error())
				continue
			}
			upserts++
			p.publishUpdated(ctx, snapshot)
		}

		if pagination.Pages == 0 || page >= pagination.Pages {
			break
		}
	}
	return upserts, nil
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

// buildSnapshot maps a Zernio analytics item onto the persisted model.
// publisherPostID is the matched key from the local post (the join id),
// kept consistent with posts.publisher_post_id.
func buildSnapshot(postID, publisherPostID string, item *zernio.AnalyticsItem, now time.Time) *models.PostAnalytics {
	a := &models.PostAnalytics{
		PostID:             postID,
		PublisherPostID:    publisherPostID,
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
		LastRefreshedAt:    now,
		RawJSON:            rawOrEmpty(item.Raw),
	}
	return a
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
		log.Printf("zernio.analytics publish event: %v", err)
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
		log.Printf("zernio.analytics write %s: %v", zernio.SettingAnalyticsLastRefreshAt, err)
	}
	status := zernio.SyncStatusOK
	if refreshErr != nil {
		status = "error: " + truncateAnalyticsErr(refreshErr.Error())
	}
	if err := p.Settings.Set(ctx, zernio.SettingAnalyticsLastRefreshStatus, status); err != nil {
		log.Printf("zernio.analytics write %s: %v", zernio.SettingAnalyticsLastRefreshStatus, err)
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
