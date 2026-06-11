package post_quality

import (
	"context"
	"fmt"
	"log"
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

// evaluate runs the single model call that scores the post, returning the
// validated structured output. On a model error or an output that fails
// validateOutput, it backs off once and retries; a second failure returns
// an *AIError. The model never returns the overall percentage — only the
// per-dimension critique.
//
// Deliberate deviation from the add-genkit-flow skill (gotcha #11 / Step 6):
// this flow uses genkit.GenerateData (native structured output) rather than
// the JSONStringScanner the skill prescribes. The scanner extracts only
// top-level scalar fields — it explicitly drops nested objects and arrays —
// and this flow's output is deeply nested (four dimension objects, each with
// an array of suggestion objects), so the scanner cannot parse it without
// abandoning the CON-85 structured-suggestions contract. Native structured
// output is the right tool for a nested schema; the skill's resilience
// concern (its strict validator discards the whole response on Claude JSON
// drift) is bounded here by the 1-retry/2s-backoff below and by truncation
// logging, and there is no user-visible streamed content to salvage (the
// /assess endpoint streams step events only). A bonus: with the schema
// enforced, generation order follows struct order, which is what makes the
// rationale-before-score contract hold (guarded by TestRationalePrecedesScore).
func evaluate(
	ctx context.Context,
	g *genkit.Genkit,
	cfg PostQualityFlowConfig,
	prompts *renderedPrompts,
) (*assessmentOutput, error) {
	maxTokens := cfg.MaxOutputTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxOutputTokens
	}
	modelName := "anthropic/" + cfg.ModelID

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryBackoff):
			}
		}

		out, resp, err := genkit.GenerateData[assessmentOutput](ctx, g,
			ai.WithModelName(modelName),
			ai.WithSystem(prompts.system),
			ai.WithPrompt(prompts.user),
			ai.WithConfig(anthropic.MessageNewParams{MaxTokens: maxTokens}),
		)
		// Deterministic truncation signal (skill Step 7 / gotcha #13). A
		// max_tokens stop usually leaves the structured output unparseable,
		// so it surfaces as a parse error + retry below — log it distinctly
		// so the output budget can be tuned before users hit the failure.
		if resp != nil && resp.FinishReason == ai.FinishReasonLength {
			var outputTokens int64
			if resp.Usage != nil {
				outputTokens = int64(resp.Usage.OutputTokens)
			}
			log.Printf("post_quality: TRUNCATED — finish_reason=length, output_tokens=%d, cap=%d; raise the model output budget",
				outputTokens, maxTokens)
		}
		if err != nil {
			lastErr = fmt.Errorf("model call: %w", err)
			log.Printf("post_quality: evaluate attempt %d failed: %v", attempt+1, lastErr)
			continue
		}
		if verr := validateOutput(out, cfg.SuggestionCap); verr != nil {
			lastErr = fmt.Errorf("output validation: %w", verr)
			log.Printf("post_quality: evaluate attempt %d invalid output: %v", attempt+1, verr)
			continue
		}
		return out, nil
	}
	return nil, &AIError{Msg: fmt.Sprintf("evaluation failed after retry: %v", lastErr)}
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
