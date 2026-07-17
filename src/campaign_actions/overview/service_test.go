package overview

import (
	"testing"

	"github.com/ogen-app/ogen/src/models"
)

func ptr(s string) *string { return &s }

func sampleCampaign() *models.Campaign {
	return &models.Campaign{
		ID:             "camp-1",
		Name:           "Go for AI",
		Status:         models.StatusActive,
		Language:       "en",
		Description:    "d",
		TargetPersona:  "p",
		KeyMessages:    "k",
		ToneGuidelines: "t",
		CampaignTypeID: "type-1",
		CampaignType: &models.CampaignType{
			ID:   "type-1",
			Name: "Awareness",
			Phases: []models.CampaignTypePhase{
				{ID: "p1", Name: "Hook", Purpose: "grab attention", Sequence: 1},
				{ID: "p2", Name: "Nurture", Purpose: "build trust", Sequence: 2},
			},
		},
	}
}

// bucketMap flattens buckets to key→count for order-independent assertions.
func bucketMap(bs []Bucket) map[string]int {
	m := make(map[string]int, len(bs))
	for _, b := range bs {
		m[b.Key] = b.Count
	}
	return m
}

func TestBuildOverview_DistributionAndReconciliation(t *testing.T) {
	posts := []models.Post{
		{ID: "a", CampaignTypePhaseID: ptr("p1"), PlatformID: "pl1", PlatformPostType: "text-post", Status: models.PostStatusDraft},
		{ID: "b", CampaignTypePhaseID: ptr("p1"), PlatformID: "pl2", PlatformPostType: "article", Status: models.PostStatusPublished},
		{ID: "c", CampaignTypePhaseID: ptr("p2"), PlatformID: "pl1", PlatformPostType: "text-post", Status: models.PostStatusDraft},
		{ID: "d", CampaignTypePhaseID: nil, PlatformID: "", PlatformPostType: "", Status: models.PostStatusScheduled},
	}
	names := map[string]string{"pl1": "LinkedIn", "pl2": "X"}

	ov := buildOverview(sampleCampaign(), posts, names)

	if ov.Type != "Awareness" {
		t.Fatalf("Type = %q, want Awareness", ov.Type)
	}
	if ov.Brief.Description != "d" || ov.Brief.ToneGuidelines != "t" {
		t.Fatalf("brief not carried through: %+v", ov.Brief)
	}
	if ov.TotalPosts != 4 {
		t.Fatalf("TotalPosts = %d, want 4", ov.TotalPosts)
	}

	// Phases ordered by sequence, with per-phase counts.
	if len(ov.Phases) != 2 || ov.Phases[0].ID != "p1" || ov.Phases[1].ID != "p2" {
		t.Fatalf("phases not in sequence order: %+v", ov.Phases)
	}
	if ov.Phases[0].PostCount != 2 || ov.Phases[1].PostCount != 1 {
		t.Fatalf("phase counts = %d,%d want 2,1", ov.Phases[0].PostCount, ov.Phases[1].PostCount)
	}
	if ov.Distribution.UnassignedPhasePostCount != 1 {
		t.Fatalf("unassigned = %d, want 1", ov.Distribution.UnassignedPhasePostCount)
	}

	// byStatus emits in the fixed lifecycle order: draft, scheduled, published.
	gotStatusOrder := []string{}
	for _, b := range ov.Distribution.ByStatus {
		gotStatusOrder = append(gotStatusOrder, b.Key)
	}
	wantStatusOrder := []string{"draft", "scheduled", "published"}
	if len(gotStatusOrder) != 3 {
		t.Fatalf("byStatus = %+v", ov.Distribution.ByStatus)
	}
	for i := range wantStatusOrder {
		if gotStatusOrder[i] != wantStatusOrder[i] {
			t.Fatalf("byStatus order = %v, want %v", gotStatusOrder, wantStatusOrder)
		}
	}
	if m := bucketMap(ov.Distribution.ByStatus); m["draft"] != 2 || m["scheduled"] != 1 || m["published"] != 1 {
		t.Fatalf("byStatus counts = %+v", m)
	}

	// byPlatform: names resolved; empty platform → "None".
	plat := bucketMap(ov.Distribution.ByPlatform)
	if plat["pl1"] != 2 || plat["pl2"] != 1 || plat[""] != 1 {
		t.Fatalf("byPlatform = %+v", ov.Distribution.ByPlatform)
	}
	if ov.Distribution.ByPlatform[0].Key != "pl1" || ov.Distribution.ByPlatform[0].Label != "LinkedIn" {
		t.Fatalf("byPlatform top = %+v, want pl1/LinkedIn", ov.Distribution.ByPlatform[0])
	}
	for _, b := range ov.Distribution.ByPlatform {
		if b.Key == "" && b.Label != "None" {
			t.Fatalf("empty platform label = %q, want None", b.Label)
		}
	}

	// byContentType: empty slug → "None".
	ct := bucketMap(ov.Distribution.ByContentType)
	if ct["text-post"] != 2 || ct["article"] != 1 || ct[""] != 1 {
		t.Fatalf("byContentType = %+v", ov.Distribution.ByContentType)
	}
	if ov.Distribution.ByContentType[0].Key != "text-post" {
		t.Fatalf("byContentType top = %+v, want text-post", ov.Distribution.ByContentType[0])
	}

	// Totals reconcile across every breakdown.
	assertReconciles(t, ov)

	// buildOverview leaves GeneratedAt zero (the caller stamps it).
	if !ov.GeneratedAt.IsZero() {
		t.Fatalf("GeneratedAt should be stamped by the caller, got %v", ov.GeneratedAt)
	}
}

func TestBuildOverview_StalePhaseCountsAsUnassigned(t *testing.T) {
	posts := []models.Post{
		{ID: "a", CampaignTypePhaseID: ptr("p1"), PlatformID: "pl1", PlatformPostType: "text-post", Status: models.PostStatusDraft},
		{ID: "b", CampaignTypePhaseID: ptr("deleted-phase"), PlatformID: "pl1", PlatformPostType: "text-post", Status: models.PostStatusDraft},
	}
	ov := buildOverview(sampleCampaign(), posts, nil)
	if ov.Phases[0].PostCount != 1 {
		t.Fatalf("p1 count = %d, want 1", ov.Phases[0].PostCount)
	}
	if ov.Distribution.UnassignedPhasePostCount != 1 {
		t.Fatalf("stale phase should count as unassigned, got %d", ov.Distribution.UnassignedPhasePostCount)
	}
	assertReconciles(t, ov)
}

func TestBuildOverview_EmptyCampaign(t *testing.T) {
	ov := buildOverview(sampleCampaign(), nil, nil)
	if ov.TotalPosts != 0 {
		t.Fatalf("TotalPosts = %d, want 0", ov.TotalPosts)
	}
	if len(ov.Phases) != 2 || ov.Phases[0].PostCount != 0 || ov.Phases[1].PostCount != 0 {
		t.Fatalf("phases should exist with zero counts: %+v", ov.Phases)
	}
	if len(ov.Distribution.ByStatus) != 0 || len(ov.Distribution.ByPlatform) != 0 || len(ov.Distribution.ByContentType) != 0 {
		t.Fatalf("distribution should be empty: %+v", ov.Distribution)
	}
}

func TestBuildOverview_NoCampaignType(t *testing.T) {
	c := sampleCampaign()
	c.CampaignType = nil
	posts := []models.Post{
		{ID: "a", CampaignTypePhaseID: ptr("p1"), PlatformID: "pl1", PlatformPostType: "text-post", Status: models.PostStatusDraft},
	}
	ov := buildOverview(c, posts, nil)
	if ov.Type != "type-1" {
		t.Fatalf("Type should fall back to CampaignTypeID, got %q", ov.Type)
	}
	if len(ov.Phases) != 0 {
		t.Fatalf("no type → no phases, got %+v", ov.Phases)
	}
	if ov.Distribution.UnassignedPhasePostCount != 1 {
		t.Fatalf("with no phases every post is unassigned, got %d", ov.Distribution.UnassignedPhasePostCount)
	}
}

// assertReconciles checks TotalPosts equals the sum of each breakdown and of
// the phase counts plus unassigned.
func assertReconciles(t *testing.T, ov *Overview) {
	t.Helper()
	sum := func(bs []Bucket) int {
		n := 0
		for _, b := range bs {
			n += b.Count
		}
		return n
	}
	if got := sum(ov.Distribution.ByStatus); got != ov.TotalPosts {
		t.Fatalf("byStatus sum = %d, want %d", got, ov.TotalPosts)
	}
	if got := sum(ov.Distribution.ByPlatform); got != ov.TotalPosts {
		t.Fatalf("byPlatform sum = %d, want %d", got, ov.TotalPosts)
	}
	if got := sum(ov.Distribution.ByContentType); got != ov.TotalPosts {
		t.Fatalf("byContentType sum = %d, want %d", got, ov.TotalPosts)
	}
	phaseSum := ov.Distribution.UnassignedPhasePostCount
	for _, p := range ov.Phases {
		phaseSum += p.PostCount
	}
	if phaseSum != ov.TotalPosts {
		t.Fatalf("phase counts + unassigned = %d, want %d", phaseSum, ov.TotalPosts)
	}
}
