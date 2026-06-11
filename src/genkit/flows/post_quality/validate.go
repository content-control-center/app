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

// validateOutput enforces the one part of the CON-85 contract that makes an
// evaluation unusable, and SANITIZES the rest in place rather than
// discarding the whole response. Per the add-genkit-flow skill (gotcha #13)
// and observed Haiku behaviour, a blank rationale/weakness or a thin
// suggestion must not 502 an otherwise-usable evaluation — the four scores
// are the product.
//
// Hard failure (triggers the retry): a dimension score outside [0,10] — an
// out-of-range score breaks the deterministic overall.
//
// Sanitized in place (never fatal): suggestions missing a span (the spec's
// guard against generic advice), missing issue/fix, or with an invalid
// severity are dropped; survivors are trimmed to suggestionCap. Empty
// rationale/weakness are tolerated — the prompt requests them emphatically,
// but a rare miss is not worth discarding the scores.
//
// It mutates out (dropping/trimming suggestions), so the caller persists the
// sanitized set. The plain error (not *ValidationError) signals a retry;
// *ValidationError is reserved for bad input → 400.
func validateOutput(out *assessmentOutput, suggestionCap int) error {
	dims := []struct {
		key models.EvaluationDimensionKey
		dim *dimensionOutput
	}{
		{models.DimensionCorrectness, &out.Correctness},
		{models.DimensionClarity, &out.Clarity},
		{models.DimensionEngagement, &out.Engagement},
		{models.DimensionDelivery, &out.Delivery},
	}
	for _, d := range dims {
		if d.dim.Score < 0 || d.dim.Score > 10 {
			return fmt.Errorf("%s: score %d out of range 0-10", d.key, d.dim.Score)
		}
		d.dim.Suggestions = sanitizeSuggestions(d.dim.Suggestions, suggestionCap)
	}
	return nil
}

// sanitizeSuggestions drops suggestions that are unusable (no span, no
// issue, no fix, or an invalid severity) and trims the survivors to cap.
// The span check realises the spec's "every suggestion has a span" by
// keeping only span-anchored ones, without failing the whole evaluation.
func sanitizeSuggestions(in []suggestionOutput, suggestionCap int) []suggestionOutput {
	kept := in[:0]
	for _, s := range in {
		if strings.TrimSpace(s.Span) == "" ||
			strings.TrimSpace(s.Issue) == "" ||
			strings.TrimSpace(s.Fix) == "" ||
			!validSeverity(s.Severity) {
			continue
		}
		kept = append(kept, s)
	}
	if suggestionCap >= 0 && len(kept) > suggestionCap {
		kept = kept[:suggestionCap]
	}
	return kept
}

func validSeverity(s string) bool {
	switch models.SuggestionSeverity(s) {
	case models.SeverityHigh, models.SeverityMedium, models.SeverityLow:
		return true
	}
	return false
}
