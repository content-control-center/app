package performers

import (
	"testing"
	"time"
)

func has(ins []insightID, id string) bool {
	for _, i := range ins {
		if string(i) == id {
			return true
		}
	}
	return false
}

type insightID string

func insightIDs(r Result) []insightID {
	out := make([]insightID, len(r.Insights))
	for i, x := range r.Insights {
		out[i] = insightID(x.ID)
	}
	return out
}

// linkedin baseline: 3 posts with reach at age 2 ≈ 5000 and at age 8 ≈ 11700.
func liBaseline() []Sample {
	return []Sample{
		{Platform: "linkedin", PostID: "b1", AgeDay: 2, Reach: 4800, Interactions: 40},
		{Platform: "linkedin", PostID: "b1", AgeDay: 8, Reach: 11000, Interactions: 90},
		{Platform: "linkedin", PostID: "b2", AgeDay: 2, Reach: 5000, Interactions: 50},
		{Platform: "linkedin", PostID: "b2", AgeDay: 8, Reach: 11700, Interactions: 95},
		{Platform: "linkedin", PostID: "b3", AgeDay: 2, Reach: 5200, Interactions: 55},
		{Platform: "linkedin", PostID: "b3", AgeDay: 8, Reach: 12000, Interactions: 100},
	}
}

// The headline acceptance criterion: a younger, lower-reach post outranks an
// older, higher-reach one once scored against the per-platform age curve.
func TestAgeAdjustedRanking(t *testing.T) {
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	cands := []Candidate{
		{PostID: "young", Platform: "linkedin", Reach: 9600, PublishedAt: now.AddDate(0, 0, -2), AgeDays: 2},
		{PostID: "old", Platform: "linkedin", Reach: 12900, PublishedAt: now.AddDate(0, 0, -8), AgeDays: 8},
	}
	res := Build(cands, liBaseline(), Options{By: ByAgainstTypical})

	if res.Best[0].PostID != "young" {
		t.Fatalf("best[0] = %s, want young (higher multiplier despite lower reach)", res.Best[0].PostID)
	}
	y := res.Best[0]
	if y.AgainstTypical == nil || *y.AgainstTypical < 1.8 || *y.AgainstTypical > 2.0 {
		t.Fatalf("young multiplier = %v, want ~1.9", y.AgainstTypical)
	}
	if y.Direction != "above" {
		t.Fatalf("young direction = %s, want above", y.Direction)
	}
}

func TestClampedPartition(t *testing.T) {
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	var cands []Candidate
	for i := 0; i < 9; i++ {
		cands = append(cands, Candidate{
			PostID:      itoa(i),
			Platform:    "linkedin",
			Reach:       (i + 1) * 10, // 10..90
			PublishedAt: now.AddDate(0, 0, -i-1),
			AgeDays:     i + 1,
		})
	}
	// No baseline → multipliers nil → against_typical falls back to reach order.
	res := Build(cands, nil, Options{By: ByAgainstTypical})
	if res.TotalPosts != 9 {
		t.Fatalf("total = %d, want 9", res.TotalPosts)
	}
	if len(res.Best) != 5 || len(res.Worst) != 4 {
		t.Fatalf("best/worst = %d/%d, want 5/4", len(res.Best), len(res.Worst))
	}
	if res.Best[0].Reach != 90 {
		t.Fatalf("best[0] reach = %d, want 90", res.Best[0].Reach)
	}
	if res.Worst[0].Reach != 10 {
		t.Fatalf("worst[0] reach = %d, want 10 (weakest first)", res.Worst[0].Reach)
	}
	// non-overlapping
	seen := map[string]bool{}
	for _, r := range append(append([]Row{}, res.Best...), res.Worst...) {
		if seen[r.PostID] {
			t.Fatalf("post %s appears in both lists", r.PostID)
		}
		seen[r.PostID] = true
	}
	// every row without a baseline is marked insufficient_history
	if res.Best[0].AgainstTypical != nil || res.Best[0].Baseline != baselineInsufficient {
		t.Fatalf("expected insufficient_history with no baseline, got %+v", res.Best[0])
	}
}

func TestInsufficientHistoryThinPlatform(t *testing.T) {
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	// only 2 baseline posts (< baselineMinPosts) on facebook
	samples := []Sample{
		{Platform: "facebook", PostID: "b1", AgeDay: 3, Reach: 1000},
		{Platform: "facebook", PostID: "b2", AgeDay: 3, Reach: 1200},
	}
	cands := []Candidate{{PostID: "c", Platform: "facebook", Reach: 5000, PublishedAt: now.AddDate(0, 0, -3), AgeDays: 3}}
	res := Build(cands, samples, Options{By: ByAgainstTypical})
	if res.Best[0].AgainstTypical != nil {
		t.Fatalf("multiplier should be nil for thin platform, got %v", *res.Best[0].AgainstTypical)
	}
	if res.Best[0].Baseline != baselineInsufficient {
		t.Fatalf("baseline = %s, want insufficient_history", res.Best[0].Baseline)
	}
}

func TestInsightsRankDivergenceAndSampleSize(t *testing.T) {
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	cands := []Candidate{
		{PostID: "big", Platform: "linkedin", Reach: 1000, Likes: 10, PublishedAt: now.AddDate(0, 0, -5), AgeDays: 5}, // eng ~0.01
		{PostID: "eng", Platform: "linkedin", Reach: 200, Likes: 20, PublishedAt: now.AddDate(0, 0, -5), AgeDays: 5},  // eng ~0.10
		{PostID: "mid", Platform: "linkedin", Reach: 500, Likes: 25, PublishedAt: now.AddDate(0, 0, -5), AgeDays: 5},  // eng ~0.05
	}
	// EngagementRate is a stored field; set it explicitly for the divergence check.
	cands[0].EngagementRate = 0.01
	cands[1].EngagementRate = 0.10
	cands[2].EngagementRate = 0.05

	res := Build(cands, nil, Options{By: ByReach})
	ids := insightIDs(res)
	if !has(ids, "rank_divergence") {
		t.Fatalf("expected rank_divergence, got %v", ids)
	}
	if !has(ids, "sample_size") {
		t.Fatalf("expected sample_size (3 posts), got %v", ids)
	}
}

func TestValidBy(t *testing.T) {
	for _, b := range []string{ByAgainstTypical, ByReach, ByEngagementRate, ByInteractions} {
		if !ValidBy(b) {
			t.Fatalf("%s should be valid", b)
		}
	}
	if ValidBy("nonsense") {
		t.Fatalf("nonsense should be invalid")
	}
}

func TestPeriodShare(t *testing.T) {
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	cands := []Candidate{
		{PostID: "a", Platform: "linkedin", Reach: 300, PublishedAt: now, AgeDays: 1},
		{PostID: "b", Platform: "linkedin", Reach: 100, PublishedAt: now, AgeDays: 1},
	}
	res := Build(cands, nil, Options{By: ByReach})
	// a has 300 / 400 = 0.75
	if res.Best[0].PostID != "a" || res.Best[0].PeriodShare != 0.75 {
		t.Fatalf("period_share = %v for %s, want 0.75 for a", res.Best[0].PeriodShare, res.Best[0].PostID)
	}
}
