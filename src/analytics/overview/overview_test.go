package overview

import (
	"testing"
	"time"

	"github.com/ogen-app/ogen/src/analytics/timeframe"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func cardByMetric(cards []Card, metric string) (Card, bool) {
	for _, c := range cards {
		if c.Metric == metric {
			return c, true
		}
	}
	return Card{}, false
}

// A 7-day window with two posts published on day 2, plus a growing follower
// level and two published timestamps. Exercises every metric's value + series.
func TestBuildEndToEnd(t *testing.T) {
	now := date("2026-08-27")
	rng, err := timeframe.Resolve("2026-08-01", "2026-08-07", "", "", now)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	pubDay := date("2026-08-02").Add(9 * time.Hour)

	in := Inputs{
		Rng:  rng,
		Prev: rng.Previous(),
		CurPosts: []PostPoint{
			{At: pubDay, Reach: 100, Interactions: 10},
			{At: pubDay, Reach: 200, Interactions: 20},
		},
		PrevPosts:     nil,
		CurPublished:  []time.Time{pubDay, pubDay},
		PrevPublished: nil,
		FollowerTotals: []FollowerDayTotal{
			{Date: date("2026-08-01"), Total: 1000},
			{Date: date("2026-08-04"), Total: 1050},
			{Date: date("2026-08-07"), Total: 1100},
		},
		FollowersNow: 1100,
		UpdatedAt:    now,
	}
	resp := Build(in)

	if resp.Window.Days != 7 {
		t.Fatalf("window days = %d, want 7", resp.Window.Days)
	}
	if len(resp.Cards) != 5 {
		t.Fatalf("cards = %d, want 5", len(resp.Cards))
	}

	// reach: cumulative endpoint = total = 300, baseline insufficient, prev empty → delta 0.
	reach, _ := cardByMetric(resp.Cards, "reach")
	if reach.Value != 300 {
		t.Fatalf("reach value = %v, want 300", reach.Value)
	}
	if reach.Baseline != baselineInsufficient {
		t.Fatalf("reach baseline = %s, want %s", reach.Baseline, baselineInsufficient)
	}
	rs := resp.Series["reach"]
	if len(rs.Current) != 7 || rs.Current[len(rs.Current)-1] != 300 {
		t.Fatalf("reach series tail = %v, want 300 at len 7 (%v)", rs.Current, rs.Current)
	}
	if len(rs.Previous) != 7 {
		t.Fatalf("reach previous len = %d, want 7 (aligned)", len(rs.Previous))
	}
	if rs.Band != nil {
		t.Fatalf("reach band should be nil until baselines exist")
	}

	// interactions total = 30.
	inter, _ := cardByMetric(resp.Cards, "interactions")
	if inter.Value != 30 {
		t.Fatalf("interactions value = %v, want 30", inter.Value)
	}

	// engagement rate = 30/300 = 0.1.
	eng, _ := cardByMetric(resp.Cards, "engagement_rate")
	if eng.Value != 0.1 {
		t.Fatalf("engagement rate = %v, want 0.1", eng.Value)
	}

	// posts published = 2.
	posts, _ := cardByMetric(resp.Cards, "posts_published")
	if posts.Value != 2 {
		t.Fatalf("posts value = %v, want 2", posts.Value)
	}

	// followers current = 1100; growth over window from 1000 → +10%.
	fol, _ := cardByMetric(resp.Cards, "followers")
	if fol.Value != 1100 {
		t.Fatalf("followers value = %v, want 1100", fol.Value)
	}
	if fol.DeltaPct != 10 {
		t.Fatalf("followers delta = %v, want 10", fol.DeltaPct)
	}
	if fol.Direction != "up" {
		t.Fatalf("followers direction = %s, want up", fol.Direction)
	}
	fs := resp.Series["followers"]
	// carry-forward: day0=1000, day3=1050, day6=1100.
	if fs.Current[0] != 1000 || fs.Current[6] != 1100 {
		t.Fatalf("follower level series = %v, want 1000..1100", fs.Current)
	}

	if resp.Insights == nil {
		t.Fatalf("insights must be non-nil (empty slice ok)")
	}
}

func TestBuildEmptyWindow(t *testing.T) {
	now := date("2026-08-27")
	rng, _ := timeframe.Resolve("2026-08-01", "2026-08-07", "", "", now)
	resp := Build(Inputs{Rng: rng, Prev: rng.Previous()})
	reach, _ := cardByMetric(resp.Cards, "reach")
	if reach.Value != 0 || reach.DeltaPct != 0 {
		t.Fatalf("empty reach card = %+v, want zeros", reach)
	}
	if len(resp.Series["reach"].Current) != 7 {
		t.Fatalf("empty series should still be 7 zero buckets")
	}
}
