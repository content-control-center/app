package post_quality

import (
	"testing"

	"github.com/ogen-app/ogen/src/models"
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

func TestValidateOutput(t *testing.T) {
	if err := validateOutput(goodOutput(), 3); err != nil {
		t.Errorf("valid output rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*assessmentOutput)
		cap    int
	}{
		{"score too high", func(o *assessmentOutput) { o.Clarity.Score = 11 }, 3},
		{"score negative", func(o *assessmentOutput) { o.Delivery.Score = -1 }, 3},
		{"missing weakness", func(o *assessmentOutput) { o.Engagement.Weakness = "  " }, 3},
		{"missing span", func(o *assessmentOutput) { o.Correctness.Suggestions[0].Span = "" }, 3},
		{"missing issue", func(o *assessmentOutput) { o.Correctness.Suggestions[0].Issue = "" }, 3},
		{"missing fix", func(o *assessmentOutput) { o.Correctness.Suggestions[0].Fix = "  " }, 3},
		{"bad severity", func(o *assessmentOutput) { o.Correctness.Suggestions[0].Severity = "urgent" }, 3},
		{"over cap", func(o *assessmentOutput) {
			o.Clarity.Suggestions = []suggestionOutput{
				{Severity: "low", Issue: "i", Fix: "f", Span: "s"},
				{Severity: "low", Issue: "i", Fix: "f", Span: "s"},
			}
		}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := goodOutput()
			tt.mutate(o)
			if err := validateOutput(o, tt.cap); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
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
