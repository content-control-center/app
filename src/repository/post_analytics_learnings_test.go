package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// TestLifespanSamples covers the CON-239 lifespan read: the slim history joined
// to the current row, reduced to one (post, age-hour) row with the max reach.
func TestLifespanSamples(t *testing.T) {
	db := openMigratedDB(t)
	ctx := tenantCtx()
	repo := repository.NewPostAnalyticsRepository(db)

	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	pub := base
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

	for _, s := range []struct {
		offset time.Duration
		reach  int
	}{
		{1 * time.Hour, 100},
		{20 * time.Hour, 500},
		{120 * time.Hour, 1000},
	} {
		if err := repo.AppendSnapshot(ctx, &models.PostAnalyticsSnapshot{
			ID:         uuid.NewString(),
			PostID:     "p1",
			Reach:      s.reach,
			OccurredAt: base.Add(s.offset),
		}); err != nil {
			t.Fatalf("append snapshot: %v", err)
		}
	}

	samples, err := repo.LifespanSamples(ctx)
	if err != nil {
		t.Fatalf("LifespanSamples: %v", err)
	}
	byAge := map[int]int{}
	for _, s := range samples {
		if s.PostID != "p1" {
			t.Fatalf("unexpected post %s", s.PostID)
		}
		byAge[s.AgeHours] = s.Reach
	}
	if len(byAge) != 3 {
		t.Fatalf("age-hours = %d, want 3 (1,20,120): %+v", len(byAge), samples)
	}
	if byAge[1] != 100 || byAge[20] != 500 || byAge[120] != 1000 {
		t.Fatalf("reach by age-hour = %+v, want 1->100, 20->500, 120->1000", byAge)
	}
}
