package poststat

import (
	"math"
	"testing"
	"time"
)

// sample builds one age-day-0 baseline observation for a platform post.
func sample(platform, id string, reach, interactions, impressions, saves, clicks int) AgeSample {
	return AgeSample{
		Platform: platform, PostID: id, AgeDay: 0,
		Reach: reach, Interactions: interactions,
		Impressions: impressions, Saves: saves, Clicks: clicks,
	}
}

func cardByMetric(cards []Card, metric string) (Card, bool) {
	for _, c := range cards {
		if c.Metric == metric {
			return c, true
		}
	}
	return Card{}, false
}

func approx(got, want float64) bool { return math.Abs(got-want) < 1e-9 }

// TestBuild_ScreenshotBands is the core acceptance check: a high multiplier on a
// tight-spread metric (reach 2.8×) reads "above", while a similar-or-larger
// multiplier on a wide-spread metric (saves 1.62×, clicks 1.70×) still reads
// "typical" — because the band is p25/p75-at-age, not a fixed threshold. A single
// fixed cutoff would wrongly flag saves/clicks as "above".
func TestBuild_ScreenshotBands(t *testing.T) {
	const pf = "instagram"
	// Five baseline posts, each with an age-day-0 observation. Reach/impressions/
	// interactions are tightly clustered; saves/clicks are widely dispersed.
	samples := []AgeSample{
		sample(pf, "p1", 600, 28, 850, 3, 5),
		sample(pf, "p2", 630, 31, 900, 8, 12),
		sample(pf, "p3", 657, 33, 932, 13, 20),
		sample(pf, "p4", 680, 35, 980, 25, 40),
		sample(pf, "p5", 700, 38, 1020, 60, 80),
	}

	resp := Build(Inputs{
		Now:       time.Unix(1_700_000_000, 0).UTC(),
		UpdatedAt: time.Unix(1_700_000_000, 0).UTC(),
		Header:    PostHeader{PostID: "target", Title: "Why we photograph every implant"},
		Age:       4 * time.Hour,
		Platform:  pf,
		Samples:   samples,
		Current: &Metrics{
			Reach: 1840, Impressions: 2610, Interactions: 96,
			Saves: 21, Clicks: 34, EngagementRate: 0.052,
		},
		Snapshots: []Snapshot{
			{AgeHours: 1, Metrics: Metrics{Reach: 640, Impressions: 900, Interactions: 30, Saves: 6, Clicks: 10}},
			{AgeHours: 2, Metrics: Metrics{Reach: 1120, Impressions: 1600, Interactions: 55, Saves: 12, Clicks: 20}},
			{AgeHours: 4, Metrics: Metrics{Reach: 1840, Impressions: 2610, Interactions: 96, Saves: 21, Clicks: 34}},
		},
		Lifespan: &Lifespan{T50Hours: 19, T95Hours: 82, SettledPosts: 74},
	})

	if resp.Overview.State != stateReady {
		t.Fatalf("state = %q, want ready", resp.Overview.State)
	}
	if !resp.StillCounting {
		t.Fatalf("still_counting = false, want true (age 4h < t95 82h)")
	}

	cases := []struct {
		metric string
		band   string
		mult   float64
	}{
		{MetricReach, "above", 2.8},
		{MetricImpressions, "above", 2.8},
		{MetricInteractions, "above", 2.91},
		{MetricSaves, "typical", 1.62},
		{MetricClicks, "typical", 1.7},
	}
	for _, tc := range cases {
		card, ok := cardByMetric(resp.Overview.Cards, tc.metric)
		if !ok {
			t.Fatalf("no card for %s", tc.metric)
		}
		if card.Baseline != "" {
			t.Fatalf("%s: baseline = %q, want a computed band", tc.metric, card.Baseline)
		}
		if card.Band != tc.band {
			t.Errorf("%s: band = %q, want %q", tc.metric, card.Band, tc.band)
		}
		if card.AgainstTypical == nil || !approx(*card.AgainstTypical, tc.mult) {
			t.Errorf("%s: against_typical = %v, want %v", tc.metric, card.AgainstTypical, tc.mult)
		}
	}

	// Narrative: pace (reach above + half-life) then the settled-post note.
	if len(resp.Overview.Narrative) != 2 {
		t.Fatalf("narrative = %d insights, want 2: %+v", len(resp.Overview.Narrative), resp.Overview.Narrative)
	}
	pace := resp.Overview.Narrative[0]
	if pace.ID != "pace" || !contains(pace.Text, "Well ahead") || !contains(pace.Text, "19 hours") {
		t.Errorf("pace insight = %+v", pace)
	}
	if half := resp.Overview.Narrative[1]; half.ID != "half_life" || !contains(half.Text, "74 finished posts") {
		t.Errorf("half_life insight = %+v", half)
	}

	// Series: running-total, anchored at publish, ending on the card figure.
	reach := resp.Series[MetricReach]
	if len(reach) == 0 {
		t.Fatalf("empty reach series")
	}
	if reach[0].AgeHours != 0 || reach[0].Value != 0 {
		t.Errorf("reach series should anchor at (0,0), got %+v", reach[0])
	}
	if last := reach[len(reach)-1]; last.Value != 1840 {
		t.Errorf("reach series should end on 1840, got %v", last.Value)
	}
}

// TestBuild_AwaitingPlatform: a post with no reported metrics yet returns the
// header + awaiting_platform overview (no cards/series), never zeros.
func TestBuild_AwaitingPlatform(t *testing.T) {
	resp := Build(Inputs{
		Now:      time.Unix(1_700_000_000, 0).UTC(),
		Header:   PostHeader{PostID: "fresh", Title: "Just out"},
		Age:      40 * time.Minute,
		Platform: "instagram",
		Current:  nil,
	})
	if resp.Overview.State != stateAwaitingPlatform {
		t.Fatalf("state = %q, want awaiting_platform", resp.Overview.State)
	}
	if len(resp.Overview.Cards) != 0 || len(resp.Series) != 0 {
		t.Fatalf("awaiting should have no cards/series, got %d cards / %d series", len(resp.Overview.Cards), len(resp.Series))
	}
	if resp.Overview.WindowLabel != "first 40 minutes" {
		t.Errorf("window_label = %q, want %q", resp.Overview.WindowLabel, "first 40 minutes")
	}
	if !resp.StillCounting {
		t.Errorf("awaiting post should be still_counting")
	}
}

// TestBuild_ThinBaseline: below the per-platform post floor, cards degrade to
// insufficient_history (value shown, no multiplier/band), not a 500 or omission.
func TestBuild_ThinBaseline(t *testing.T) {
	resp := Build(Inputs{
		Now:      time.Unix(1_700_000_000, 0).UTC(),
		Header:   PostHeader{PostID: "target"},
		Age:      4 * time.Hour,
		Platform: "instagram",
		Current:  &Metrics{Reach: 1840, Impressions: 2610, Interactions: 96, Saves: 21, Clicks: 34, EngagementRate: 0.052},
		Samples:  []AgeSample{sample("instagram", "p1", 600, 28, 850, 3, 5)}, // 1 post < baselineMinPosts
	})
	if len(resp.Overview.Cards) != len(metricOrder) {
		t.Fatalf("want %d cards, got %d", len(metricOrder), len(resp.Overview.Cards))
	}
	reach, _ := cardByMetric(resp.Overview.Cards, MetricReach)
	if reach.Baseline != baselineInsufficient {
		t.Errorf("reach baseline = %q, want insufficient_history", reach.Baseline)
	}
	if reach.AgainstTypical != nil || reach.Band != "" {
		t.Errorf("thin-baseline card should carry no multiplier/band, got %+v", reach)
	}
	if reach.Value != 1840 {
		t.Errorf("value should still be shown, got %v", reach.Value)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
