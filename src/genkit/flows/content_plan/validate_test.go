package content_plan

import (
	"strings"
	"testing"
	"time"

	"github.com/ogen-app/ogen/src/models"
)

// parseTestDate is a panic-on-error parser used by the tests in this file
// so newTestCampaign doesn't need a *testing.T (the batch_test.go variant
// of mustDate uses t.Fatalf which doesn't tolerate a nil receiver).
func parseTestDate(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic("parseTestDate: " + err.Error())
	}
	return d
}

func newTestCampaign() *models.Campaign {
	start := parseTestDate("2026-05-01")
	end := parseTestDate("2026-05-31")
	return &models.Campaign{
		ID: "campaign-1",
		CampaignType: &models.CampaignType{
			Phases: []models.CampaignTypePhase{
				{ID: "ph-1", Sequence: 1},
				{ID: "ph-2", Sequence: 2},
			},
		},
		StartDate: &start,
		EndDate:   &end,
	}
}

func TestBuildPostValidator_Accepts(t *testing.T) {
	campaign := newTestCampaign()
	platforms := []resolvedPlatform{
		{ID: "linkedin", Name: "LinkedIn"},
	}
	validate := buildPostValidator(campaign, platforms)

	post := DraftPost{
		Title:       "Hello",
		PlatformID:  "linkedin",
		ContentType: "text-post",
		PublishDate: "2026-05-15",
		PhaseID:     "ph-1",
	}
	if err := validate(post); err != nil {
		t.Fatalf("expected accept, got: %v", err)
	}
}

func TestBuildPostValidator_RejectsUnknownPlatform(t *testing.T) {
	validate := buildPostValidator(newTestCampaign(), []resolvedPlatform{{ID: "linkedin"}})
	err := validate(DraftPost{
		PlatformID:  "x-twitter",
		PublishDate: "2026-05-15",
		PhaseID:     "ph-1",
	})
	if err == nil || !strings.Contains(err.Error(), "platformId") {
		t.Errorf("err = %v, want platformId rejection", err)
	}
}

func TestBuildPostValidator_RejectsContentTypeNotInAllowlist(t *testing.T) {
	platforms := []resolvedPlatform{
		{ID: "linkedin", AllowedSlugs: []string{"text-post", "newsletter"}},
	}
	validate := buildPostValidator(newTestCampaign(), platforms)
	err := validate(DraftPost{
		PlatformID:  "linkedin",
		ContentType: "carousel",
		PublishDate: "2026-05-15",
		PhaseID:     "ph-1",
	})
	if err == nil || !strings.Contains(err.Error(), "contentType") {
		t.Errorf("err = %v, want contentType rejection", err)
	}
}

func TestBuildPostValidator_AllowsAnyContentTypeWhenAllowedSlugsEmpty(t *testing.T) {
	// Empty AllowedSlugs means "no per-platform constraint" — anything goes.
	platforms := []resolvedPlatform{{ID: "linkedin"}}
	validate := buildPostValidator(newTestCampaign(), platforms)
	if err := validate(DraftPost{
		PlatformID:  "linkedin",
		ContentType: "anything-the-model-invents",
		PublishDate: "2026-05-15",
		PhaseID:     "ph-1",
	}); err != nil {
		t.Errorf("expected accept on empty allowlist, got: %v", err)
	}
}

func TestBuildPostValidator_RejectsOutOfRangeDate(t *testing.T) {
	validate := buildPostValidator(newTestCampaign(), []resolvedPlatform{{ID: "linkedin"}})
	cases := []string{"2026-04-30", "2026-06-01", ""}
	for _, d := range cases {
		err := validate(DraftPost{
			PlatformID:  "linkedin",
			PublishDate: d,
			PhaseID:     "ph-1",
		})
		if err == nil || !strings.Contains(err.Error(), "publishDate") {
			t.Errorf("date %q: err = %v, want publishDate rejection", d, err)
		}
	}
}

func TestBuildPostValidator_RejectsUnknownPhase(t *testing.T) {
	validate := buildPostValidator(newTestCampaign(), []resolvedPlatform{{ID: "linkedin"}})
	err := validate(DraftPost{
		PlatformID:  "linkedin",
		PublishDate: "2026-05-15",
		PhaseID:     "ph-99",
	})
	if err == nil || !strings.Contains(err.Error(), "phaseId") {
		t.Errorf("err = %v, want phaseId rejection", err)
	}
}

// validateOutput is the legacy slice-based wrapper retained for tests and
// other call sites that already have the full slice in hand. Verify it
// stays equivalent to applying buildPostValidator inline.
func TestValidateOutput_ProducesWarningsForRejected(t *testing.T) {
	campaign := newTestCampaign()
	platforms := []resolvedPlatform{{ID: "linkedin"}}
	posts := []DraftPost{
		{Title: "good", PlatformID: "linkedin", PublishDate: "2026-05-15", PhaseID: "ph-1"},
		{Title: "bad-platform", PlatformID: "fake", PublishDate: "2026-05-15", PhaseID: "ph-1"},
		{Title: "bad-date", PlatformID: "linkedin", PublishDate: "2026-12-15", PhaseID: "ph-1"},
		{Title: "good-2", PlatformID: "linkedin", PublishDate: "2026-05-20", PhaseID: "ph-2"},
	}
	valid, warnings := validateOutput(posts, campaign, platforms)
	if len(valid) != 2 {
		t.Errorf("valid len = %d, want 2", len(valid))
	}
	if len(warnings) != 2 {
		t.Errorf("warnings len = %d, want 2 (got %v)", len(warnings), warnings)
	}
}
