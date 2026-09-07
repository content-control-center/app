package repository_test

import (
	"testing"
	"time"

	"github.com/ogen-app/ogen/src/domain/models"
	"github.com/ogen-app/ogen/src/repository"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func followerRow(t *testing.T, accountID, platform string, followers, growth int64, pct float64, date time.Time) models.FollowerSnapshot {
	t.Helper()
	return models.FollowerSnapshot{
		ID:               mustID(t),
		SocialAccountID:  accountID,
		Platform:         platform,
		Username:         "acme",
		Followers:        followers,
		Growth:           growth,
		GrowthPercentage: pct,
		PointDate:        date,
		RawJSON:          "{}",
		RecordedAt:       time.Now().UTC(),
	}
}

func TestFollowerStatsUpsertAndSummary(t *testing.T) {
	db := openMigratedDB(t)
	ctx := tenantCtx()
	repo := repository.NewFollowerStatsRepository(db)

	// Empty → no summary, zero latest date.
	if sum, err := repo.Summary(ctx, ""); err != nil || len(sum) != 0 {
		t.Fatalf("empty summary: got %v err %v", sum, err)
	}
	if latest, err := repo.LatestPointDate(ctx); err != nil || !latest.IsZero() {
		t.Fatalf("empty latest: got %v err %v", latest, err)
	}

	rows := []models.FollowerSnapshot{
		followerRow(t, "acc-1", "instagram", 100, 0, 0, day(2026, 7, 1)),
		followerRow(t, "acc-1", "instagram", 128, 28, 28.0, day(2026, 7, 2)),
		followerRow(t, "acc-2", "tiktok", 50, 5, 11.1, day(2026, 7, 2)),
	}
	n, err := repo.InsertMany(ctx, rows)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if n != 3 {
		t.Fatalf("rows affected: got %d want 3", n)
	}

	// Same (account, day) upsert refines in place, not appends.
	refine := []models.FollowerSnapshot{followerRow(t, "acc-1", "instagram", 130, 30, 30.0, day(2026, 7, 2))}
	if _, err := repo.InsertMany(ctx, refine); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	sum, err := repo.Summary(ctx, "")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(sum) != 2 {
		t.Fatalf("summary accounts: got %d want 2", len(sum))
	}
	byAcc := map[string]repository.FollowerAccountSummary{}
	for _, s := range sum {
		byAcc[s.SocialAccountID] = s
	}
	a1 := byAcc["acc-1"]
	if a1.CurrentFollowers != 130 { // latest (2026-07-02, upserted)
		t.Errorf("acc-1 current: got %d want 130", a1.CurrentFollowers)
	}
	if a1.DataPoints != 2 { // 07-01 and 07-02 (upsert did not add a 3rd)
		t.Errorf("acc-1 data_points: got %d want 2", a1.DataPoints)
	}
	if a1.GrowthPercentage != 30.0 {
		t.Errorf("acc-1 growth_pct: got %v want 30", a1.GrowthPercentage)
	}

	// Account filter.
	only, err := repo.Summary(ctx, "acc-2")
	if err != nil || len(only) != 1 || only[0].SocialAccountID != "acc-2" {
		t.Fatalf("filtered summary: got %v err %v", only, err)
	}

	latest, err := repo.LatestPointDate(ctx)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if !latest.Equal(day(2026, 7, 2)) {
		t.Errorf("latest point: got %v want 2026-07-02", latest)
	}
}

func TestFollowerStatsSeries(t *testing.T) {
	db := openMigratedDB(t)
	ctx := tenantCtx()
	repo := repository.NewFollowerStatsRepository(db)

	rows := []models.FollowerSnapshot{
		followerRow(t, "acc-1", "instagram", 100, 0, 0, day(2026, 7, 1)),
		followerRow(t, "acc-1", "instagram", 110, 10, 10, day(2026, 7, 2)),
		followerRow(t, "acc-1", "instagram", 120, 20, 20, day(2026, 7, 3)),
		followerRow(t, "acc-2", "tiktok", 50, 0, 0, day(2026, 7, 2)),
	}
	if _, err := repo.InsertMany(ctx, rows); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// One account, windowed.
	pts, err := repo.Series(ctx, repository.FollowerSeriesOptions{
		AccountID: "acc-1",
		From:      day(2026, 7, 2),
		To:        day(2026, 7, 3),
	})
	if err != nil {
		t.Fatalf("series: %v", err)
	}
	if len(pts) != 2 {
		t.Fatalf("series points: got %d want 2", len(pts))
	}
	if pts[0].Followers != 110 || pts[1].Followers != 120 {
		t.Errorf("series order/values: %+v", pts)
	}

	// All accounts, unbounded.
	all, err := repo.Series(ctx, repository.FollowerSeriesOptions{})
	if err != nil {
		t.Fatalf("series all: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("series all: got %d want 4", len(all))
	}
}
