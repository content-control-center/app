package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/database"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/pgtest"
	"github.com/ogen-app/ogen/src/repository"
)

// openMigratedDB returns a fresh, fully-migrated Postgres database with FK
// enforcement bypassed for the session, so a test can insert a post with a
// synthetic campaign_id / created_by without seeding the whole campaign
// graph — the repo logic under test doesn't depend on FK enforcement.
//
// It also applies the analytics migration set on the SAME database, so the
// post_analytics_snapshots table (CON-125 Track B) exists here. In production
// that table lives in the isolated analytics DB; in tests one physical DB holds
// both sets (the table names don't collide), so the repo can be pointed at the
// same pool.
//
// Postgres has no per-statement FK pragma, so the pool is capped at 1 and
// the session uses session_replication_role=replica (which skips FK-trigger
// checks; the test database user is a superuser, as required). With a single
// pooled connection the setting sticks for the whole test.
func openMigratedDB(t *testing.T) *bun.DB {
	t.Helper()
	db := pgtest.MustDB()
	db.DB.SetMaxOpenConns(1)
	db.DB.SetMaxIdleConns(1)
	if _, err := db.Exec("SET session_replication_role = replica"); err != nil {
		t.Fatalf("disable fks: %v", err)
	}
	if err := database.MigrateAnalytics(context.Background(), db); err != nil {
		t.Fatalf("migrate analytics: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedPost(t *testing.T, db *bun.DB, id, publisher, publisherPostID string, publishedAt time.Time) {
	t.Helper()
	post := &models.Post{
		ID:              id,
		CampaignID:      "camp-1",
		Title:           "Post " + id,
		Content:         "content " + id,
		MediaURLs:       models.StringSlice{},
		UsedAssetIDs:    models.StringSlice{},
		Status:          models.PostStatusPublished,
		Publisher:       publisher,
		PublisherPostID: publisherPostID,
		CTAType:         models.CTATypeNone,
		CreatedBy:       "user-1",
		PublishedAt:     &publishedAt,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if _, err := db.NewInsert().Model(post).Exec(tenantCtx()); err != nil {
		t.Fatalf("seed post %s: %v", id, err)
	}
}

func TestPostAnalyticsInsertAppendsAndGetLatest(t *testing.T) {
	db := openMigratedDB(t)
	ctx := tenantCtx()
	repo := repository.NewPostAnalyticsRepository(db)

	// No snapshot yet → (nil, nil).
	got, err := repo.GetByPostID(ctx, "p1")
	if err != nil {
		t.Fatalf("get (empty): %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil snapshot, got %+v", got)
	}

	lu := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	t0 := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	first := &models.PostAnalytics{
		ID:              mustID(t),
		PostID:          "p1",
		PublisherPostID: "z-1",
		Publisher:       models.PublisherZernio,
		Platform:        "linkedin",
		Impressions:     100,
		Likes:           10,
		EngagementRate:  0.1,
		PlatformAnalytics: models.PlatformAnalyticsList{
			{Platform: "linkedin", SyncStatus: "synced", Analytics: models.PostAnalyticsMetrics{Impressions: 100, Likes: 10}},
		},
		SyncStatus:         "synced",
		MetricsLastUpdated: &lu,
		RawJSON:            `{"postId":"z-1"}`,
		OccurredAt:         t0,
	}
	if err := repo.Insert(ctx, first); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err = repo.GetByPostID(ctx, "p1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.Impressions != 100 || got.Likes != 10 {
		t.Fatalf("snapshot mismatch: %+v", got)
	}
	if len(got.PlatformAnalytics) != 1 || got.PlatformAnalytics[0].Platform != "linkedin" {
		t.Fatalf("platform breakdown not persisted: %+v", got.PlatformAnalytics)
	}

	// A later refresh APPENDS a new row; GetByPostID returns the latest by
	// occurred_at, and both rows remain (append-only, not overwrite).
	second := *first
	second.ID = mustID(t)
	second.Impressions = 250
	second.OccurredAt = t0.Add(30 * time.Minute)
	if err := repo.Insert(ctx, &second); err != nil {
		t.Fatalf("insert second: %v", err)
	}
	got, _ = repo.GetByPostID(ctx, "p1")
	if got.Impressions != 250 {
		t.Fatalf("latest-per-post failed: impressions=%d want 250", got.Impressions)
	}
	count, err := db.NewSelect().Model((*models.PostAnalytics)(nil)).Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows after append, got %d", count)
	}
}

func TestPostAnalyticsListAndOverview(t *testing.T) {
	db := openMigratedDB(t)
	ctx := tenantCtx()
	repo := repository.NewPostAnalyticsRepository(db)

	base := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	mustInsert(t, repo, "p1", "z-1", 100, 10, 0.10, base)
	mustInsert(t, repo, "p2", "z-2", 300, 30, 0.20, base)
	// A stale earlier snapshot for p1 must be ignored by the latest-per-post read.
	mustInsert(t, repo, "p1", "z-1", 999, 999, 0.99, base.Add(-2*time.Hour))

	items, overview, err := repo.List(ctx, repository.PostAnalyticsListOptions{
		Publisher: models.PublisherZernio,
		SortBy:    "impressions",
		Order:     "desc",
		Page:      1,
		Limit:     50,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items: got %d want 2", len(items))
	}
	// Sorted by impressions desc → p2 first; p1 shows its LATEST (100, not 999).
	if items[0].PostID != "p2" || items[1].PostID != "p1" {
		t.Fatalf("sort order wrong: %s, %s", items[0].PostID, items[1].PostID)
	}
	if items[0].Analytics.Impressions != 300 || items[1].Analytics.Impressions != 100 {
		t.Fatalf("latest-per-post nested analytics wrong: %+v / %+v", items[0].Analytics, items[1].Analytics)
	}
	if overview.PostCount != 2 || overview.Impressions != 400 || overview.Likes != 40 {
		t.Fatalf("overview sums wrong: %+v", overview)
	}
	if overview.EngagementRateAvg < 0.149 || overview.EngagementRateAvg > 0.151 {
		t.Fatalf("overview avg wrong: %v", overview.EngagementRateAvg)
	}

	// Pagination: limit 1 returns the top item, overview still over full set.
	page1, ov, err := repo.List(ctx, repository.PostAnalyticsListOptions{
		Publisher: models.PublisherZernio, SortBy: "impressions", Order: "desc", Page: 1, Limit: 1,
	})
	if err != nil {
		t.Fatalf("list page1: %v", err)
	}
	if len(page1) != 1 || page1[0].PostID != "p2" {
		t.Fatalf("page1 wrong: %+v", page1)
	}
	if ov.PostCount != 2 {
		t.Fatalf("overview should cover full set: %+v", ov)
	}
}

func TestListWithPublisherPostID(t *testing.T) {
	db := openMigratedDB(t)
	ctx := tenantCtx()
	repo := repository.NewPostRepository(db)

	seedPost(t, db, "p1", models.PublisherZernio, "z-1", time.Now().UTC())
	seedPost(t, db, "p2", models.PublisherZernio, "z-2", time.Now().UTC())
	seedPost(t, db, "p3", "", "", time.Now().UTC()) // never published via a publisher

	// A Zernio post that ended up `failed` (a partial publish maps to failed)
	// must STILL be included — it has real per-platform analytics Zernio can
	// return (CON-93 §10). Status is intentionally not a filter.
	if _, err := db.NewInsert().Model(&models.Post{
		ID:              "p4",
		CampaignID:      "camp-1",
		Content:         "content p4",
		MediaURLs:       models.StringSlice{},
		UsedAssetIDs:    models.StringSlice{},
		Status:          models.PostStatusFailed,
		Publisher:       models.PublisherZernio,
		PublisherPostID: "z-4",
		CTAType:         models.CTATypeNone,
		CreatedBy:       "user-1",
	}).Exec(ctx); err != nil {
		t.Fatalf("seed failed post: %v", err)
	}

	posts, err := repo.ListWithPublisherPostID(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(posts) != 3 {
		t.Fatalf("got %d want 3", len(posts))
	}
	byID := map[string]string{}
	for _, p := range posts {
		byID[p.ID] = p.PublisherPostID
	}
	if byID["p1"] != "z-1" || byID["p2"] != "z-2" {
		t.Fatalf("match map wrong: %+v", byID)
	}
	if byID["p4"] != "z-4" {
		t.Fatalf("failed (non-published) Zernio post must be included: %+v", byID)
	}
	if _, ok := byID["p3"]; ok {
		t.Fatalf("p3 should be excluded")
	}
}

func TestBackfillPostAnalytics(t *testing.T) {
	db := openMigratedDB(t)
	ctx := tenantCtx()
	repo := repository.NewPostAnalyticsRepository(db)

	const linkedinSqid = "AXqWG7U2qnpt" // seeded platform → name "LinkedIn"
	published := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	post := &models.Post{
		ID:              "p-bf",
		CampaignID:      "camp-1",
		PlatformID:      linkedinSqid,
		Title:           "Backfilled Post",
		Content:         "content",
		MediaURLs:       models.StringSlice{},
		UsedAssetIDs:    models.StringSlice{},
		Status:          models.PostStatusPublished,
		Publisher:       models.PublisherZernio,
		PublisherPostID: "z-bf",
		CTAType:         models.CTATypeNone,
		CreatedBy:       "user-1",
		PublishedAt:     &published,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if _, err := db.NewInsert().Model(post).Exec(ctx); err != nil {
		t.Fatalf("seed post: %v", err)
	}

	// A legacy main-DB post_analytics row (the pre-migration shape). Inserted
	// with raw SQL because models.PostAnalytics now maps to the new table.
	refreshedAt := time.Date(2026, 6, 15, 9, 30, 0, 0, time.UTC)
	if _, err := db.Exec(`
		INSERT INTO post_analytics
			(post_id, tenant_id, publisher_post_id, impressions, reach, likes, comments,
			 shares, saves, clicks, views, engagement_rate, platform_analytics, sync_status,
			 metrics_last_updated, last_refreshed_at, raw_json, created_at, updated_at)
		VALUES (?, ?, ?, 500, 400, 50, 5, 3, 2, 1, 600, 0.25, '[]', 'synced',
			 NULL, ?, '{}', now(), now())`,
		"p-bf", models.DefaultTenantID, "z-bf", refreshedAt); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	// First backfill copies the legacy row into the snapshot table, denormalising
	// publisher/platform/title/published_at via the main-DB join.
	n, err := repository.BackfillPostAnalytics(ctx, db, db)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if n != 1 {
		t.Fatalf("backfilled %d want 1", n)
	}

	got, err := repo.GetByPostID(ctx, "p-bf")
	if err != nil || got == nil {
		t.Fatalf("get after backfill: %v (%+v)", err, got)
	}
	if got.Impressions != 500 || got.Views != 600 {
		t.Errorf("metrics not copied: %+v", got)
	}
	if got.Publisher != models.PublisherZernio || got.Platform != "LinkedIn" || got.Title != "Backfilled Post" {
		t.Errorf("denormalised fields wrong: publisher=%q platform=%q title=%q", got.Publisher, got.Platform, got.Title)
	}
	if !got.OccurredAt.Equal(refreshedAt) {
		t.Errorf("occurred_at = %v want %v (from last_refreshed_at)", got.OccurredAt, refreshedAt)
	}

	// Idempotent: a second run inserts nothing (the post already has a snapshot).
	n2, err := repository.BackfillPostAnalytics(ctx, db, db)
	if err != nil {
		t.Fatalf("backfill re-run: %v", err)
	}
	if n2 != 0 {
		t.Errorf("second backfill inserted %d, want 0 (idempotent)", n2)
	}
	count, _ := db.NewSelect().Model((*models.PostAnalytics)(nil)).Where("pa.post_id = ?", "p-bf").Count(ctx)
	if count != 1 {
		t.Errorf("snapshot rows for p-bf = %d, want 1", count)
	}
}

func mustID(t *testing.T) string {
	t.Helper()
	id, err := models.NewID()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	return id
}

func mustInsert(t *testing.T, repo repository.PostAnalyticsRepository, postID, pubPostID string, impressions, likes int, rate float64, occurredAt time.Time) {
	t.Helper()
	err := repo.Insert(tenantCtx(), &models.PostAnalytics{
		ID:                mustID(t),
		PostID:            postID,
		PublisherPostID:   pubPostID,
		Publisher:         models.PublisherZernio,
		Platform:          "linkedin",
		Impressions:       impressions,
		Likes:             likes,
		EngagementRate:    rate,
		PlatformAnalytics: models.PlatformAnalyticsList{},
		SyncStatus:        "synced",
		RawJSON:           "{}",
		OccurredAt:        occurredAt,
	})
	if err != nil {
		t.Fatalf("insert %s: %v", postID, err)
	}
}
