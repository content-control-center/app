package post_quality

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/ogen-app/ogen/src/models"
)

// retryBackoff is the pause before the single retry on a failed or
// invalid evaluation (CON-85: "1-retry / 2s-backoff convention").
const retryBackoff = 2 * time.Second

// defaultMaxOutputTokens caps the scoring response when the config leaves
// MaxOutputTokens at 0. It is deliberately small: a four-dimension critique
// with capped suggestions runs only a few thousand tokens, and evaluate
// issues a NON-streaming GenerateData call. The Anthropic SDK rejects a
// non-streaming request whose max_tokens implies a >10-minute run — its
// estimate is 1h × max_tokens/128000, so anything above ~21k tokens is
// refused (and Opus-tier models cap non-streaming at 8192). 8192 stays
// under every limit while leaving ample headroom for the response.
const defaultMaxOutputTokens = 8192

// dimensionTargets enumerates the four dimensions and the label shown to the
// model. The order matches the assembly in evaluate.
var dimensionTargets = []struct {
	key   models.EvaluationDimensionKey
	label string
}{
	{models.DimensionCorrectness, "Correctness"},
	{models.DimensionClarity, "Clarity"},
	{models.DimensionEngagement, "Engagement"},
	{models.DimensionDelivery, "Delivery"},
}

// evaluate scores the post ONE dimension per model call and assembles the
// four results. A single nested call (all four dimensions in one schema) has
// the model fully fill only the first sub-object and stub the rest: genkit's
// Anthropic constrained output is prompt-injected, not hard-constrained, so
// the model can satisfy the schema by leaving later objects empty. A focused
// per-dimension call returns one complete object every time. The four calls
// run concurrently and share the cached system prefix.
//
// Each call still uses genkit.GenerateData (native structured output) rather
// than the skill's JSONStringScanner: the scanner drops nested arrays, and a
// dimension carries a suggestion array, so the scanner cannot parse it
// without abandoning the structured-suggestions contract. Field order in
// dimensionOutput keeps rationale before score (guarded by
// TestRationalePrecedesScore).
func evaluate(
	ctx context.Context,
	g *genkit.Genkit,
	cfg PostQualityFlowConfig,
	prompts *renderedPrompts,
) (*assessmentOutput, error) {
	results := make([]dimensionOutput, len(dimensionTargets))
	errs := make([]error, len(dimensionTargets))

	var wg sync.WaitGroup
	for i, t := range dimensionTargets {
		wg.Add(1)
		go func(i int, label string) {
			defer wg.Done()
			results[i], errs[i] = evaluateDimension(ctx, g, cfg, prompts, label)
		}(i, t.label)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err // already *AIError
		}
	}

	out := &assessmentOutput{
		Correctness: results[0],
		Clarity:     results[1],
		Engagement:  results[2],
		Delivery:    results[3],
	}
	// Sanitize suggestions across the assembled output (drop span-less / thin
	// suggestions, trim to cap). Score-range is the only hard error, and the
	// focused calls return valid scores, so this does not fail here.
	if err := validateOutput(out, cfg.SuggestionCap); err != nil {
		return nil, &AIError{Msg: fmt.Sprintf("evaluation failed validation: %v", err)}
	}
	return out, nil
}

// evaluateDimension runs the focused model call for one dimension with the
// 1-retry/2s-backoff convention. It retries on a model error or an empty
// rationale — the signal that the dimension was not actually evaluated; a
// focused call reliably returns one, so this rarely trips.
func evaluateDimension(
	ctx context.Context,
	g *genkit.Genkit,
	cfg PostQualityFlowConfig,
	prompts *renderedPrompts,
	label string,
) (dimensionOutput, error) {
	maxTokens := cfg.MaxOutputTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxOutputTokens
	}
	modelName := "anthropic/" + cfg.ModelID
	userPrompt := prompts.user + dimensionInstruction(label, cfg.SuggestionCap)

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return dimensionOutput{}, ctx.Err()
			case <-time.After(retryBackoff):
			}
		}

		// WithSystem/WithPrompt run their first arg through fmt.Sprintf, so
		// pass the text as a "%s" argument — never as the format string.
		// Otherwise a post containing "6% of", "5% CPU" etc. gets mangled
		// into "6%!o(MISSING)f" (the post text interpreted as fmt verbs).
		out, resp, err := genkit.GenerateData[dimensionOutput](ctx, g,
			ai.WithModelName(modelName),
			ai.WithSystem("%s", prompts.system),
			ai.WithPrompt("%s", userPrompt),
			ai.WithConfig(anthropic.MessageNewParams{MaxTokens: maxTokens}),
		)
		if resp != nil && resp.FinishReason == ai.FinishReasonLength {
			var outputTokens int64
			if resp.Usage != nil {
				outputTokens = int64(resp.Usage.OutputTokens)
			}
			log.Printf("post_quality[%s]: TRUNCATED — finish_reason=length, output_tokens=%d, cap=%d",
				label, outputTokens, maxTokens)
		}
		if err != nil {
			lastErr = fmt.Errorf("model call: %w", err)
			log.Printf("post_quality[%s]: attempt %d failed: %v", label, attempt+1, lastErr)
			continue
		}
		if strings.TrimSpace(out.Rationale) == "" {
			lastErr = fmt.Errorf("empty rationale")
			log.Printf("post_quality[%s]: attempt %d returned empty rationale", label, attempt+1)
			continue
		}
		return *out, nil
	}
	return dimensionOutput{}, &AIError{Msg: fmt.Sprintf("%s evaluation failed after retry: %v", label, lastErr)}
}

// dimensionInstruction is the per-call directive appended to the post-context
// user prompt, naming the single dimension to score.
func dimensionInstruction(label string, suggestionCap int) string {
	return fmt.Sprintf(
		"\n\nScore ONLY the %s dimension against its anchored bands. Write the rationale (2-4 specific sentences) before the score, name one concrete weakness, choose a 0-10 score, and give at most %d span-anchored, guidance-only suggestions — all for %s only.",
		label, suggestionCap, label,
	)
}

// toEvaluationResult maps the model's structured output into the persisted
// result type. Weight and Contribution are left zero here — ComposeScore
// fills them.
func toEvaluationResult(out *assessmentOutput) models.EvaluationResult {
	return models.EvaluationResult{
		Correctness: toDimension(out.Correctness, models.DimensionCorrectness),
		Clarity:     toDimension(out.Clarity, models.DimensionClarity),
		Engagement:  toDimension(out.Engagement, models.DimensionEngagement),
		Delivery:    toDimension(out.Delivery, models.DimensionDelivery),
	}
}

func toDimension(d dimensionOutput, key models.EvaluationDimensionKey) models.EvaluationDimension {
	suggestions := make([]models.EvaluationSuggestion, 0, len(d.Suggestions))
	for _, s := range d.Suggestions {
		suggestions = append(suggestions, models.EvaluationSuggestion{
			Dimension: key, // implied by the parent block; stamped here
			Severity:  models.SuggestionSeverity(s.Severity),
			Issue:     s.Issue,
			Fix:       s.Fix,
			Span:      s.Span,
		})
	}
	return models.EvaluationDimension{
		Score:       d.Score,
		Rationale:   d.Rationale,
		Weakness:    d.Weakness,
		Suggestions: suggestions,
	}
}
