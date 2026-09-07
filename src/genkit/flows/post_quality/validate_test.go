package post_quality

import (
	"testing"

	"github.com/ogen-app/ogen/src/domain/models"
)

func TestValidateInput(t *testing.T) {
	ok := &models.Post{Content: "hello", PlatformID: "pl1", PlatformPostType: "text-post"}
	if err := validateInput(ok); err != nil {
		t.Errorf("valid post rejected: %v", err)
	}

	tests := []struct {
		name string
		post *models.Post
	}{
		{"empty body", &models.Post{Content: "   ", PlatformID: "pl1", PlatformPostType: "text-post"}},
		{"no platform", &models.Post{Content: "hi", PlatformPostType: "text-post"}},
		{"no type", &models.Post{Content: "hi", PlatformID: "pl1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInput(tt.post)
			if err == nil {
				t.Fatal("expected ValidationError, got nil")
			}
			if _, ok := err.(*ValidationError); !ok {
				t.Errorf("expected *ValidationError, got %T", err)
			}
		})
	}
}

func goodDim() dimensionOutput {
	return dimensionOutput{
		Score: 7, Rationale: "r", Weakness: "w",
		Suggestions: []suggestionOutput{{Severity: "high", Issue: "i", Fix: "f", Span: "s"}},
	}
}

func goodOutput() *assessmentOutput {
	return &assessmentOutput{
		Correctness: goodDim(), Clarity: goodDim(), Engagement: goodDim(), Delivery: goodDim(),
	}
}

func TestValidateOutputValid(t *testing.T) {
	o := goodOutput()
	if err := validateOutput(o, 3); err != nil {
		t.Errorf("valid output rejected: %v", err)
	}
	if len(o.Correctness.Suggestions) != 1 {
		t.Errorf("valid suggestion should be kept, got %d", len(o.Correctness.Suggestions))
	}
}

func TestValidateOutputHardFailsOnScore(t *testing.T) {
	// An out-of-range score is the only fatal condition.
	for _, tt := range []struct {
		name   string
		mutate func(*assessmentOutput)
	}{
		{"score too high", func(o *assessmentOutput) { o.Clarity.Score = 11 }},
		{"score negative", func(o *assessmentOutput) { o.Delivery.Score = -1 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o := goodOutput()
			tt.mutate(o)
			if err := validateOutput(o, 3); err == nil {
				t.Error("expected hard failure, got nil")
			}
		})
	}
}

func TestValidateOutputToleratesBlankProse(t *testing.T) {
	// Blank rationale/weakness no longer fail the evaluation — the scores
	// are still usable.
	o := goodOutput()
	o.Clarity.Rationale = ""
	o.Engagement.Weakness = "  "
	if err := validateOutput(o, 3); err != nil {
		t.Errorf("blank rationale/weakness should be tolerated, got %v", err)
	}
}

func TestValidateOutputDropsBadSuggestions(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*assessmentOutput)
	}{
		{"missing span", func(o *assessmentOutput) { o.Correctness.Suggestions[0].Span = "" }},
		{"missing issue", func(o *assessmentOutput) { o.Correctness.Suggestions[0].Issue = "" }},
		{"missing fix", func(o *assessmentOutput) { o.Correctness.Suggestions[0].Fix = "  " }},
		{"bad severity", func(o *assessmentOutput) { o.Correctness.Suggestions[0].Severity = "urgent" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o := goodOutput()
			tt.mutate(o)
			if err := validateOutput(o, 3); err != nil {
				t.Fatalf("should sanitize, not fail: %v", err)
			}
			if len(o.Correctness.Suggestions) != 0 {
				t.Errorf("bad suggestion should be dropped, got %d", len(o.Correctness.Suggestions))
			}
		})
	}
}

func TestValidateOutputTrimsOverCap(t *testing.T) {
	o := goodOutput()
	o.Clarity.Suggestions = []suggestionOutput{
		{Severity: "low", Issue: "i", Fix: "f", Span: "s"},
		{Severity: "low", Issue: "i", Fix: "f", Span: "s"},
		{Severity: "low", Issue: "i", Fix: "f", Span: "s"},
	}
	if err := validateOutput(o, 2); err != nil {
		t.Fatalf("over-cap should trim, not fail: %v", err)
	}
	if len(o.Clarity.Suggestions) != 2 {
		t.Errorf("expected trim to 2, got %d", len(o.Clarity.Suggestions))
	}
}

func TestToEvaluationResult(t *testing.T) {
	out := goodOutput()
	out.Engagement.Score = 9
	r := toEvaluationResult(out)

	if r.Engagement.Score != 9 || r.Correctness.Rationale != "r" || r.Clarity.Weakness != "w" {
		t.Errorf("dimension fields not mapped: %+v", r)
	}
	// Each suggestion must be stamped with its parent dimension.
	if r.Delivery.Suggestions[0].Dimension != models.DimensionDelivery {
		t.Errorf("suggestion dimension = %q, want %q",
			r.Delivery.Suggestions[0].Dimension, models.DimensionDelivery)
	}
	if r.Correctness.Suggestions[0].Severity != models.SeverityHigh {
		t.Errorf("severity not mapped: %q", r.Correctness.Suggestions[0].Severity)
	}
	// Weight/Contribution must be left zero — ComposeScore fills them.
	if r.Correctness.Weight != 0 || r.Correctness.Contribution != 0 {
		t.Errorf("weight/contribution should be zero before ComposeScore")
	}
}
