package post_quality

import (
	"fmt"
	"strings"

	"github.com/ogen-app/ogen/src/models"
)

// validateInput enforces the flow's preconditions on the loaded Post:
// it must have non-whitespace body text, a Platform, and a post Type.
// Returns *ValidationError (→ HTTP 400) so an unevaluable post never
// reaches a model call.
func validateInput(post *models.Post) error {
	if strings.TrimSpace(post.Content) == "" {
		return &ValidationError{Msg: "post has no body text to evaluate"}
	}
	if post.PlatformID == "" {
		return &ValidationError{Msg: "post has no platform; assign a platform before assessing quality"}
	}
	if post.PlatformPostType == "" {
		return &ValidationError{Msg: "post has no type; assign a post type before assessing quality"}
	}
	return nil
}

// validateOutput checks the model's structured output against the schema
// contract: every score in [0,10], a non-empty weakness per dimension
// (mandatory weakness), every suggestion span-anchored with a valid
// severity, and no more than cap suggestions per dimension. A failure
// returns a plain error so the caller retries — it is not a
// ValidationError (that is reserved for bad input → 400).
func validateOutput(out *assessmentOutput, suggestionCap int) error {
	dims := []struct {
		key models.EvaluationDimensionKey
		dim dimensionOutput
	}{
		{models.DimensionCorrectness, out.Correctness},
		{models.DimensionClarity, out.Clarity},
		{models.DimensionEngagement, out.Engagement},
		{models.DimensionDelivery, out.Delivery},
	}
	for _, d := range dims {
		if d.dim.Score < 0 || d.dim.Score > 10 {
			return fmt.Errorf("%s: score %d out of range 0-10", d.key, d.dim.Score)
		}
		if strings.TrimSpace(d.dim.Weakness) == "" {
			return fmt.Errorf("%s: missing mandatory weakness", d.key)
		}
		if len(d.dim.Suggestions) > suggestionCap {
			return fmt.Errorf("%s: %d suggestions exceed cap of %d", d.key, len(d.dim.Suggestions), suggestionCap)
		}
		for i, s := range d.dim.Suggestions {
			if strings.TrimSpace(s.Span) == "" {
				return fmt.Errorf("%s: suggestion %d has no span", d.key, i)
			}
			if !validSeverity(s.Severity) {
				return fmt.Errorf("%s: suggestion %d has invalid severity %q", d.key, i, s.Severity)
			}
		}
	}
	return nil
}

func validSeverity(s string) bool {
	switch models.SuggestionSeverity(s) {
	case models.SeverityHigh, models.SeverityMedium, models.SeverityLow:
		return true
	}
	return false
}
