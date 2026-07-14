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
}
