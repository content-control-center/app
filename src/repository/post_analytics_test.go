package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/pgtest"
	"github.com/ogen-app/ogen/src/repository"
)

// openMigratedDB returns a fresh, fully-migrated Postgres database with FK
// enforcement bypassed for the session, so a test can insert a post with a
// synthetic campaign_id / created_by without seeding the whole campaign
// graph — the repo logic under test doesn't depend on FK enforcement.
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
	if _, err := db.NewInsert().Model(post).Exec(context.Background()); err != nil {
		t.Fatalf("seed post %s: %v", id, err)
	}
}

func TestPostAnalyticsUpsertAndGet(t *testing.T) {
	db := openMigratedDB(t)
	ctx := context.Background()
	repo := repository.NewPostAnalyticsRepository(db)

	seedPost(t, db, "p1", models.PublisherZernio, "z-1", time.Now().UTC())

	// No snapshot yet → (nil, nil).
	got, err := repo.GetByPostID(ctx, "p1")
	if err != nil {
		t.Fatalf("get (empty): %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil snapshot, got %+v", got)
	}

	lu := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	a := &models.PostAnalytics{
		PostID:          "p1",
		PublisherPostID: "z-1",
		Impressions:     100,
		Likes:           10,
		EngagementRate:  0.1,
		PlatformAnalytics: models.PlatformAnalyticsList{
			{Platform: "linkedin", SyncStatus: "synced", Analytics: models.PostAnalyticsMetrics{Impressions: 100, Likes: 10}},
		},
		SyncStatus:         "synced",
		MetricsLastUpdated: &lu,
		LastRefreshedAt:    time.Now().UTC(),
		RawJSON:            `{"postId":"z-1"}`,
	}
	if err := repo.Upsert(ctx, a); err != nil {
		t.Fatalf("upsert: %v", err)
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

	// Upsert again overwrites in place (1:1 on post_id), not appends.
	a.Impressions = 250
	a.Likes = 20
	if err := repo.Upsert(ctx, a); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	got, _ = repo.GetByPostID(ctx, "p1")
	if got.Impressions != 250 {
		t.Fatalf("overwrite failed: impressions=%d want 250", got.Impressions)
	}
	var count int
	count, err = db.NewSelect().Model((*models.PostAnalytics)(nil)).Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row after re-upsert, got %d", count)
	}
}

func TestPostAnalyticsListAndOverview(t *testing.T) {
	db := openMigratedDB(t)
	ctx := context.Background()
	repo := repository.NewPostAnalyticsRepository(db)

	now := time.Now().UTC()
	seedPost(t, db, "p1", models.PublisherZernio, "z-1", now.Add(-2*time.Hour))
	seedPost(t, db, "p2", models.PublisherZernio, "z-2", now.Add(-1*time.Hour))
	// A non-Zernio post must be excluded from the overview.
	seedPost(t, db, "p3", "", "", now)

	mustUpsert(t, repo, "p1", "z-1", 100, 10, 0.10)
	mustUpsert(t, repo, "p2", "z-2", 300, 30, 0.20)

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
	// Sorted by impressions desc → p2 first.
	if items[0].PostID != "p2" || items[1].PostID != "p1" {
		t.Fatalf("sort order wrong: %s, %s", items[0].PostID, items[1].PostID)
	}
	if items[0].Analytics.Impressions != 300 {
		t.Fatalf("nested analytics wrong: %+v", items[0].Analytics)
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
	ctx := context.Background()
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

func mustUpsert(t *testing.T, repo repository.PostAnalyticsRepository, postID, pubPostID string, impressions, likes int, rate float64) {
	t.Helper()
	err := repo.Upsert(context.Background(), &models.PostAnalytics{
		PostID:            postID,
		PublisherPostID:   pubPostID,
		Impressions:       impressions,
		Likes:             likes,
		EngagementRate:    rate,
		PlatformAnalytics: models.PlatformAnalyticsList{},
		SyncStatus:        "synced",
		LastRefreshedAt:   time.Now().UTC(),
		RawJSON:           "{}",
	})
	if err != nil {
		t.Fatalf("upsert %s: %v", postID, err)
	}
}
