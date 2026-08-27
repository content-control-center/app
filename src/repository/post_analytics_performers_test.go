package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// TestReachByAgeSamples covers the CON-238 baseline read: the slim history joined
// to the current row, reduced to one (platform, post, age-day) row with the max
// metric in that age-day.
func TestReachByAgeSamples(t *testing.T) {
	db := openMigratedDB(t)
	ctx := tenantCtx()
	repo := repository.NewPostAnalyticsRepository(db)

	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	pub := base
	// current row carries platform + published_at (the join source).
	if err := repo.Upsert(ctx, &models.PostAnalytics{
		PostID:            "p1",
		PublisherPostID:   "z-1",
		Publisher:         models.PublisherZernio,
		Platform:          "linkedin",
		PublishedAt:       &pub,
		PlatformAnalytics: models.PlatformAnalyticsList{},
		FirstSeenAt:       base,
		LastChangedAt:     base,
		LastCheckedAt:     base,
	}); err != nil {
		t.Fatalf("upsert current: %v", err)
	}

	// three change points at ages 0, 2, 8 days.
	for _, s := range []struct {
		offset time.Duration
		reach  int
		likes  int
	}{
		{1 * time.Hour, 100, 5},
		{48 * time.Hour, 500, 20},
		{192 * time.Hour, 1200, 40},
	} {
		if err := repo.AppendSnapshot(ctx, &models.PostAnalyticsSnapshot{
			ID:         uuid.NewString(),
			PostID:     "p1",
			Reach:      s.reach,
			Likes:      s.likes,
			OccurredAt: base.Add(s.offset),
		}); err != nil {
			t.Fatalf("append snapshot: %v", err)
		}
	}

	samples, err := repo.ReachByAgeSamples(ctx)
	if err != nil {
		t.Fatalf("ReachByAgeSamples: %v", err)
	}
	byAge := map[int]repository.ReachAgeSample{}
	for _, s := range samples {
		if s.Platform != "linkedin" || s.PostID != "p1" {
			t.Fatalf("unexpected sample %+v", s)
		}
		byAge[s.AgeDay] = s
	}
	if len(byAge) != 3 {
		t.Fatalf("age-days = %d, want 3 (0,2,8): %+v", len(byAge), samples)
	}
	if byAge[0].Reach != 100 || byAge[2].Reach != 500 || byAge[8].Reach != 1200 {
		t.Fatalf("reach by age = %+v, want 0->100, 2->500, 8->1200", byAge)
	}
	if byAge[8].Interactions != 40 {
		t.Fatalf("age8 interactions = %d, want 40", byAge[8].Interactions)
	}
}
