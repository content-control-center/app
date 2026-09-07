package consistency

import (
	"strings"
	"testing"

	"github.com/ogen-app/ogen/src/domain/models"
)

func TestEligiblePosts(t *testing.T) {
	posts := []models.Post{
		{ID: "a", Status: models.PostStatusDraft, Content: "hello"},
		{ID: "b", Status: models.PostStatusReadyForPublish, Content: "ready"},
		{ID: "c", Status: models.PostStatusScheduled, Content: "later"},
		{ID: "d", Status: models.PostStatusPublished, Content: "live"},       // excluded: published
		{ID: "e", Status: models.PostStatusNotPublished, Content: "skipped"}, // excluded: not_published
		{ID: "f", Status: models.PostStatusDraft, Content: "   "},            // excluded: blank content
		{ID: "g", Status: models.PostStatusDraft, Content: ""},               // excluded: empty content
	}

	got := eligiblePosts(posts)
	var ids []string
	for _, p := range got {
		ids = append(ids, p.ID)
	}
	want := []string{"a", "b", "c"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("eligiblePosts = %v, want %v", ids, want)
	}
}

func TestSeverityHigh(t *testing.T) {
	if !severityHigh("high") {
		t.Fatal(`severityHigh("high") = false, want true`)
	}
	for _, s := range []string{"medium", "low", "High", "HIGH", ""} {
		if severityHigh(s) {
			t.Errorf("severityHigh(%q) = true, want false", s)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("  short  ", 100); got != "short" {
		t.Fatalf("truncate trims and passes through short = %q", got)
	}
	// Rune-safe truncation adds an ellipsis.
	got := truncate("abcdef", 3)
	if got != "abc…" {
		t.Fatalf("truncate(abcdef,3) = %q, want abc…", got)
	}
	// Multi-byte runes are not split.
	got = truncate("héllo wörld", 4)
	if got != "héll…" {
		t.Fatalf("truncate multibyte = %q, want héll…", got)
	}
}

func TestLoadTemplates_BriefUser(t *testing.T) {
	tpl, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}

	sys, err := tpl.renderBriefSystem()
	if err != nil {
		t.Fatalf("renderBriefSystem: %v", err)
	}
	for _, marker := range []string{"consistency reviewer", "goal_alignment", "persona", "completeness"} {
		if !strings.Contains(sys, marker) {
			t.Errorf("brief system prompt missing %q", marker)
		}
	}

	c := &models.Campaign{
		Name:           "Launch",
		Description:    "Sell the thing",
		TargetPersona:  "Busy CTOs",
		KeyMessages:    "Fast. Reliable.",
		ToneGuidelines: "Confident",
		Language:       "en",
		CampaignType: &models.CampaignType{
			Name:        "product_launch",
			Label:       "Product Launch",
			Description: "Drive awareness and signups",
			Phases: []models.CampaignTypePhase{
				{Sequence: 1, Name: "Tease", Purpose: "build anticipation"},
			},
		},
	}
	usr, err := tpl.renderBriefUser(c)
	if err != nil {
		t.Fatalf("renderBriefUser: %v", err)
	}
	for _, want := range []string{"Launch", "Product Launch", "Drive awareness", "Tease", "Busy CTOs", "Fast. Reliable.", "Confident"} {
		if !strings.Contains(usr, want) {
			t.Errorf("brief user prompt missing %q\n---\n%s", want, usr)
		}
	}
}

func TestLoadTemplates_PostsUserTruncatesBody(t *testing.T) {
	tpl, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}

	long := strings.Repeat("x", maxPostBodyChars+50)
	c := &models.Campaign{TargetPersona: "Devs", KeyMessages: "Ship", ToneGuidelines: "Terse"}
	posts := []models.Post{{ID: "p1", Title: "First", Content: long}}
	out, err := tpl.renderPostsUser(c, posts)
	if err != nil {
		t.Fatalf("renderPostsUser: %v", err)
	}

	if !strings.Contains(out, "[postId: p1]") {
		t.Fatalf("posts user prompt missing post id marker\n%s", out)
	}
	if !strings.Contains(out, "…") {
		t.Fatalf("expected truncated body ellipsis for an over-long post")
	}
	if strings.Count(out, "x") > maxPostBodyChars {
		t.Fatalf("post body not truncated: got %d x's, cap %d", strings.Count(out, "x"), maxPostBodyChars)
	}
}
