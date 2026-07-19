package campaign_assistant

import (
	"testing"
	"time"

	"github.com/ogen-app/ogen/src/models"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptrTime(t time.Time) *time.Time { return &t }

func timelineCampaign() *models.Campaign {
	return &models.Campaign{
		StartDate: ptrTime(day("2026-01-01")),
		EndDate:   ptrTime(day("2026-01-30")), // 30 days, 3 phases → 10 days each
		CampaignType: &models.CampaignType{
			Phases: []models.CampaignTypePhase{
				{ID: "p1", Name: "Hook", Sequence: 1},
				{ID: "p2", Name: "Nurture", Sequence: 2},
				{ID: "p3", Name: "Convert", Sequence: 3},
			},
		},
		Platforms: []models.Platform{
			{ID: "pl1", Name: "LinkedIn"},
			{ID: "pl2", Name: "Threads"},
		},
	}
}

func TestCurrentPhase_Timeline(t *testing.T) {
	c := timelineCampaign()
	cases := []struct{ today, want string }{
		{"2026-01-05", "p1"}, // window 1: 01-01..01-10
		{"2026-01-15", "p2"}, // window 2: 01-11..01-20
		{"2026-01-28", "p3"}, // window 3: 01-21..01-30
		{"2025-12-20", "p1"}, // before start → first
		{"2026-03-01", "p3"}, // after end → last
	}
	for _, tc := range cases {
		if got := currentPhase(c, day(tc.today)); got.ID != tc.want {
			t.Errorf("today %s: currentPhase = %q, want %q", tc.today, got.ID, tc.want)
		}
	}
}

func TestCurrentPhase_NoDatesFallsBackToFirst(t *testing.T) {
	c := timelineCampaign()
	c.StartDate, c.EndDate = nil, nil
	if got := currentPhase(c, day("2026-01-15")); got.ID != "p1" {
		t.Fatalf("no-dates fallback = %q, want p1", got.ID)
	}
}

func TestResolvePhase(t *testing.T) {
	c := timelineCampaign()
	today := day("2026-01-15") // → p2 for "current"

	for _, tc := range []struct{ in, wantID string }{
		{"current", "p2"},
		{"", "p2"},
		{"Nurture", "p2"}, // by name
		{"p3", "p3"},      // by id
	} {
		id, _, err := resolvePhase(c, tc.in, today)
		if err != nil {
			t.Fatalf("resolvePhase(%q): %v", tc.in, err)
		}
		if id != tc.wantID {
			t.Errorf("resolvePhase(%q) = %q, want %q", tc.in, id, tc.wantID)
		}
	}

	if _, _, err := resolvePhase(c, "Nonexistent", today); err == nil {
		t.Fatal("expected error for unknown phase")
	}
}

func TestResolveTargetPlatforms(t *testing.T) {
	c := timelineCampaign()

	ids, names, err := resolveTargetPlatforms(c, []string{"Threads"})
	if err != nil {
		t.Fatalf("by name: %v", err)
	}
	if len(ids) != 1 || ids[0] != "pl2" || names[0] != "Threads" {
		t.Fatalf("by name = %v / %v", ids, names)
	}

	// Case-insensitive name + id both resolve; dupes collapse.
	ids, _, err = resolveTargetPlatforms(c, []string{"threads", "pl2", "pl1"})
	if err != nil {
		t.Fatalf("mixed: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("dupes should collapse, got %v", ids)
	}

	// A non-target platform is rejected (offer-to-add path).
	if _, _, err := resolveTargetPlatforms(c, []string{"TikTok"}); err == nil {
		t.Fatal("expected error for a non-target platform")
	}
}

func TestResolveWindow(t *testing.T) {
	today := day("2026-02-10")

	// Omitted → next 14 days.
	s, e, err := resolveWindow("", "", today)
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	if s != "2026-02-10" || e != "2026-02-24" {
		t.Fatalf("default window = %s..%s", s, e)
	}

	// Valid future window passes through.
	s, e, err = resolveWindow("2026-02-15", "2026-02-28", today)
	if err != nil || s != "2026-02-15" || e != "2026-02-28" {
		t.Fatalf("passthrough = %s..%s err=%v", s, e, err)
	}

	// Past start clamps to today.
	s, _, err = resolveWindow("2026-01-01", "2026-02-28", today)
	if err != nil || s != "2026-02-10" {
		t.Fatalf("past-start clamp = %s err=%v", s, err)
	}

	// End before start → error.
	if _, _, err := resolveWindow("2026-02-20", "2026-02-15", today); err == nil {
		t.Fatal("expected error when end precedes start")
	}
	// Bad format → error.
	if _, _, err := resolveWindow("nope", "2026-02-15", today); err == nil {
		t.Fatal("expected error for bad windowStart")
	}

	// Only a start → end derived as start + 14 days (the "upcoming weeks" bug:
	// the model resolves a start and omits the end). Must not error.
	s, e, err = resolveWindow("2026-02-10", "", today)
	if err != nil || s != "2026-02-10" || e != "2026-02-24" {
		t.Fatalf("start-only = %s..%s err=%v", s, e, err)
	}

	// Only an end → start defaults to today.
	s, e, err = resolveWindow("", "2026-02-28", today)
	if err != nil || s != "2026-02-10" || e != "2026-02-28" {
		t.Fatalf("end-only = %s..%s err=%v", s, e, err)
	}

	// A non-empty but malformed end is still rejected.
	if _, _, err := resolveWindow("2026-02-15", "nope", today); err == nil {
		t.Fatal("expected error for bad windowEnd")
	}
}

// CON-118: page citation rendering for asset Q&A excerpts.
func TestPageRef(t *testing.T) {
	p := func(i int) *int { return &i }
	cases := []struct {
		start, end *int
		want       string
	}{
		{nil, nil, ""},
		{p(4), nil, "p. 4"},
		{p(4), p(4), "p. 4"},
		{p(3), p(5), "pp. 3-5"},
	}
	for _, c := range cases {
		if got := pageRef(c.start, c.end); got != c.want {
			t.Errorf("pageRef(%v, %v) = %q, want %q", c.start, c.end, got, c.want)
		}
	}
}

// CON-114: a count the user names is honored exactly; an omitted/zero count
// defaults to 1 (the safe minimum) so an omitting planner can't over-produce.
// Guards the "generate 1 post" -> 3 regression.
func TestResolveGenerateCount(t *testing.T) {
	cases := []struct {
		name        string
		requested   int
		maxN        int
		wantCount   int
		wantReq     int
		wantClamped bool
	}{
		{"exact one is honored", 1, 10, 1, 1, false},
		{"exact five is honored", 5, 10, 5, 5, false},
		{"omitted defaults to 1 (safe minimum)", 0, 10, 1, 1, false},
		{"negative treated as omitted", -2, 10, 1, 1, false},
		{"above cap clamps down", 25, 10, 10, 25, true},
		{"unset cap falls back to 10", 25, 0, 10, 25, true},
	}
	for _, c := range cases {
		gotCount, gotReq, gotClamped := resolveGenerateCount(c.requested, c.maxN)
		if gotCount != c.wantCount || gotReq != c.wantReq || gotClamped != c.wantClamped {
			t.Errorf("%s: resolveGenerateCount(%d, %d) = (count=%d, requested=%d, clamped=%v); want (count=%d, requested=%d, clamped=%v)",
				c.name, c.requested, c.maxN, gotCount, gotReq, gotClamped, c.wantCount, c.wantReq, c.wantClamped)
		}
	}
}
