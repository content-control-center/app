package content_plan

import (
	"testing"

	"github.com/ogen-app/ogen/src/models"
)

func targetedPlatforms() []resolvedPlatform {
	return []resolvedPlatform{
		{ID: "pl1", Name: "LinkedIn", PostTypes: "text-post, article", AllowedSlugs: []string{"text-post", "article"}},
		{ID: "pl2", Name: "Threads", PostTypes: "text-post", AllowedSlugs: []string{"text-post"}},
	}
}

func TestSelectPlatforms_SubsetAndScope(t *testing.T) {
	all := targetedPlatforms()

	// Subset by id.
	got, err := selectPlatforms(all, []string{"pl2"}, "")
	if err != nil {
		t.Fatalf("subset: %v", err)
	}
	if len(got) != 1 || got[0].ID != "pl2" {
		t.Fatalf("subset = %+v", got)
	}

	// A non-target platform is rejected.
	if _, err := selectPlatforms(all, []string{"pl9"}, ""); err == nil {
		t.Fatal("expected error for a non-target platform")
	}

	// Empty selection is rejected.
	if _, err := selectPlatforms(all, nil, ""); err == nil {
		t.Fatal("expected error for empty platform selection")
	}
}

func TestSelectPlatforms_NarrowPostType(t *testing.T) {
	all := targetedPlatforms()

	// Narrowing to a supported post type restricts PostTypes + AllowedSlugs.
	got, err := selectPlatforms(all, []string{"pl1"}, "article")
	if err != nil {
		t.Fatalf("narrow: %v", err)
	}
	if got[0].PostTypes != "article" || len(got[0].AllowedSlugs) != 1 || got[0].AllowedSlugs[0] != "article" {
		t.Fatalf("narrow did not restrict post type: %+v", got[0])
	}

	// A post type the platform doesn't support is rejected.
	if _, err := selectPlatforms(all, []string{"pl2"}, "article"); err == nil {
		t.Fatal("expected error: Threads does not support article")
	}
}

func TestResolvePhaseByID(t *testing.T) {
	c := &models.Campaign{CampaignType: &models.CampaignType{Phases: []models.CampaignTypePhase{
		{ID: "p1", Name: "Hook", Purpose: "grab", Sequence: 1},
		{ID: "p2", Name: "Nurture", Purpose: "build", Sequence: 2},
	}}}

	ph, ok := resolvePhaseByID(c, "p2")
	if !ok || ph.Name != "Nurture" || ph.Sequence != 2 {
		t.Fatalf("resolvePhaseByID(p2) = %+v ok=%v", ph, ok)
	}
	if _, ok := resolvePhaseByID(c, "nope"); ok {
		t.Fatal("expected not-found for unknown phase id")
	}
}

func TestNewPostValidator_Window(t *testing.T) {
	platforms := []resolvedPlatform{{ID: "pl1", AllowedSlugs: []string{"text-post"}}}
	phaseIDs := map[string]bool{"ph1": true}
	v := newPostValidator(platforms, phaseIDs, "2026-02-01", "2026-02-14")

	ok := DraftPost{PlatformID: "pl1", ContentType: "text-post", PublishDate: "2026-02-05", PhaseID: "ph1"}
	if err := v(ok); err != nil {
		t.Fatalf("valid post rejected: %v", err)
	}

	// Outside the window (even if within the campaign range elsewhere).
	outside := ok
	outside.PublishDate = "2026-03-01"
	if err := v(outside); err == nil {
		t.Fatal("expected out-of-window rejection")
	}

	// Not the targeted phase.
	wrongPhase := ok
	wrongPhase.PhaseID = "ph2"
	if err := v(wrongPhase); err == nil {
		t.Fatal("expected wrong-phase rejection")
	}

	// Disallowed content type.
	wrongType := ok
	wrongType.ContentType = "article"
	if err := v(wrongType); err == nil {
		t.Fatal("expected disallowed content-type rejection")
	}
}
