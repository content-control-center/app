package repository_test

import (
	"testing"
	"time"

	"github.com/ogen-app/ogen/src/domain/models"
	"github.com/ogen-app/ogen/src/infra/repository"
)

// TestPublishedBetween covers the CON-237 overview read: current-state rows
// filtered to a publish-time window, ordered ascending, tenant-scoped.
func TestPublishedBetween(t *testing.T) {
	db := openMigratedDB(t)
	ctx := tenantCtx()
	repo := repository.NewPostAnalyticsRepository(db)

	d := func(s string) time.Time {
		tt, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatalf("parse %s: %v", s, err)
		}
		return tt
	}
	seed := func(postID string, publishedAt time.Time, reach int) {
		t.Helper()
		pa := publishedAt
		if err := repo.Upsert(ctx, &models.PostAnalytics{
			PostID:            postID,
			PublisherPostID:   "z-" + postID,
			Publisher:         models.PublisherZernio,
			Platform:          "linkedin",
			Reach:             reach,
			PlatformAnalytics: models.PlatformAnalyticsList{},
			PublishedAt:       &pa,
			FirstSeenAt:       publishedAt,
			LastChangedAt:     publishedAt,
			LastCheckedAt:     publishedAt,
		}); err != nil {
			t.Fatalf("seed %s: %v", postID, err)
		}
	}

	seed("p1", d("2026-08-02"), 100)
	seed("p2", d("2026-08-05"), 200)
	seed("p3", d("2026-07-20"), 999) // before the window → excluded

	rows, err := repo.PublishedBetween(ctx, d("2026-08-01"), d("2026-08-08"))
	if err != nil {
		t.Fatalf("PublishedBetween: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].PostID != "p1" || rows[1].PostID != "p2" {
		t.Fatalf("order = %s,%s, want p1,p2 (published_at ASC)", rows[0].PostID, rows[1].PostID)
	}
	if rows[0].Reach != 100 {
		t.Fatalf("p1 reach = %d, want 100", rows[0].Reach)
	}
}
