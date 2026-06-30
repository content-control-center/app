package post_quality

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"

	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/post_actions/logs"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/vendors/llm"
)

// defaultSuggestionCap is the top-N suggestions per dimension when the
// config leaves SuggestionCap at zero.
const defaultSuggestionCap = 3

// PostQualityFlowConfig holds the settings for the assessPostQuality flow.
type PostQualityFlowConfig struct {
	// Provider resolves the model reference + call config by role; post_quality
	// uses the quality role (cfg.QualityModelID) (CON-86 FR12).
	Provider *llm.Provider
	// ModelID is the Anthropic model used for scoring — Sonnet 4.5 by
	// default, specified separately from the generation flows.
	ModelID string
	// MaxOutputTokens caps the model response; 0 falls back to
	// defaultMaxOutputTokens. Keep it under the Anthropic non-streaming
	// limit — evaluate issues a blocking GenerateData call.
	MaxOutputTokens int64
	// SuggestionCap is the top-N suggestions per dimension; 0 falls back to
	// defaultSuggestionCap.
	SuggestionCap int
	// Weights selects the per-PlatformPostType weight profile used to
	// compose the overall score.
	Weights Weights
	// Hub publishes the "assessment finalised" event on success/failure.
	// nil = silent.
	Hub eventhub.Hub

	tmpl *templates
}

// PostQualityRepos bundles the repository dependencies for the flow.
type PostQualityRepos struct {
	Posts       repository.PostRepository
	Campaigns   repository.CampaignRepository
	Assets      repository.AssetRepository
	Chunks      repository.AssetChunksRepository
	Platforms   repository.PlatformRepository
	Evaluations repository.PostEvaluationRepository
	PostLogs    repository.PostLogRepository
}

// postQualityFlow is the registered flow; nil until InitPostQuality runs.
var postQualityFlow *core.Flow[PostQualityRequest, *PostQualityResponse, struct{}]

// postQualityRunner is a closure over (g, cfg, repos) that threads an
// OnEventFunc through for SSE streaming. Set by InitPostQuality.
var postQualityRunner func(ctx context.Context, req PostQualityRequest, onEvent OnEventFunc) (*PostQualityResponse, error)

// InitPostQuality registers the assessPostQuality Genkit flow. It must be
// called after the Genkit instance is initialised with the Anthropic
// plugin.
func InitPostQuality(g *genkit.Genkit, cfg PostQualityFlowConfig, repos PostQualityRepos) error {
	tmpl, err := loadTemplates()
	if err != nil {
		return err
	}
	cfg.tmpl = tmpl
	if cfg.SuggestionCap <= 0 {
		cfg.SuggestionCap = defaultSuggestionCap
	}

	postQualityFlow = genkit.DefineFlow(g, "assessPostQuality",
		func(ctx context.Context, req PostQualityRequest) (*PostQualityResponse, error) {
			return runPostQuality(ctx, g, req, cfg, repos, nil)
		},
	)
	postQualityRunner = func(ctx context.Context, req PostQualityRequest, onEvent OnEventFunc) (*PostQualityResponse, error) {
		return runPostQuality(ctx, g, req, cfg, repos, onEvent)
	}
	return nil
}

// NewPostQualityCallback returns a callback for the posts handler. onEvent
// is forwarded for SSE streaming; pass nil for a silent call.
func NewPostQualityCallback() func(ctx context.Context, postID string, onEvent OnEventFunc) (*PostQualityResponse, error) {
	return func(ctx context.Context, postID string, onEvent OnEventFunc) (*PostQualityResponse, error) {
		return postQualityRunner(ctx, PostQualityRequest{PostID: postID}, onEvent)
	}
}

// emit calls onEvent when non-nil; a safe no-op otherwise.
func emit(onEvent OnEventFunc, name SSEEventKind, data any) {
	if onEvent != nil {
		onEvent(name, data)
	}
}

// runPostQuality executes the six steps of the flow.
func runPostQuality(
	ctx context.Context,
	g *genkit.Genkit,
	req PostQualityRequest,
	cfg PostQualityFlowConfig,
	repos PostQualityRepos,
	onEvent OnEventFunc,
) (out *PostQualityResponse, retErr error) {
	start := time.Now()
	log.Printf("post_quality[%s]: starting", req.PostID)

	// Captured once the post is loaded, so the finalisation event is scoped
	// to the post owner. Empty before validateInput → very-early failures
	// emit no finalisation event.
	var ownerID string
	defer func() {
		if cfg.Hub == nil || ownerID == "" {
			return
		}
		publishAssessmentFinalised(cfg.Hub, req.PostID, ownerID, out, retErr)
	}()

	// ── Step 1: validateInput ────────────────────────────────────────────
	post, err := repos.Posts.GetByID(ctx, req.PostID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &ValidationError{Msg: "post not found"}
		}
		return nil, fmt.Errorf("load post: %w", err)
	}
	if err := validateInput(post); err != nil {
		return nil, err
	}
	ownerID = post.CreatedBy
	emit(onEvent, SSEEventStep, StepEventPayload{Step: "validateInput", Status: "done"})

	// ── Step 2: buildContext ─────────────────────────────────────────────
	prompts, err := buildContext(ctx, post, cfg, repos)
	if err != nil {
		return nil, fmt.Errorf("build context: %w", err)
	}
	emit(onEvent, SSEEventStep, StepEventPayload{Step: "buildContext", Status: "done"})

	// ── Change detection (CON-92): the rendered prompt encodes everything
	// the model sees (post body, platform, type, campaign brief, phase,
	// asset previews); the model id and the resolved weight profile cover
	// the rest of what determines the score. If all of them are unchanged
	// since the stored evaluation, skip the model run and return the cached
	// result — so we re-assess only when something that actually affects the
	// score changed.
	hash := inputHash(prompts, cfg.ModelID, cfg.Weights.For(post.PlatformPostType))
	if cached, cerr := repos.Evaluations.GetByPostID(ctx, post.ID); cerr == nil && cached != nil && cached.InputHash == hash {
		log.Printf("post_quality[%s]: inputs unchanged — returning cached evaluation", req.PostID)
		resp := &PostQualityResponse{
			PostID:      post.ID,
			GeneratedAt: time.Now().UTC(),
			Evaluation:  cached,
			Cached:      true,
		}
		emit(onEvent, SSEEventComplete, resp)
		return resp, nil
	}

	// ── Step 3: evaluate (single model call, 1-retry/2s-backoff) ─────────
	output, err := evaluate(ctx, g, cfg, prompts)
	if err != nil {
		return nil, err
	}
	emit(onEvent, SSEEventStep, StepEventPayload{Step: "evaluate", Status: "done"})

	// ── Step 4: validateOutput (already enforced inside evaluate's retry) ─
	emit(onEvent, SSEEventStep, StepEventPayload{Step: "validateOutput", Status: "done"})

	// ── Step 5: composeScore (deterministic, backend-owned) ──────────────
	result := toEvaluationResult(output)
	profile := cfg.Weights.For(post.PlatformPostType)
	overall := ComposeScore(&result, profile)
	emit(onEvent, SSEEventStep, StepEventPayload{Step: "composeScore", Status: "done"})

	// ── Step 6: persist (upsert evaluation + PostLog) ────────────────────
	eval, err := persist(ctx, repos, post, result, overall, prompts.captionScoped, cfg.ModelID, hash)
	if err != nil {
		return nil, fmt.Errorf("persist evaluation: %w", err)
	}
	emit(onEvent, SSEEventStep, StepEventPayload{Step: "persist", Status: "done"})

	log.Printf("post_quality[%s]: done in %s (overall=%.1f%%, type=%s)",
		req.PostID, time.Since(start).Round(time.Millisecond), overall, post.PlatformPostType)

	resp := &PostQualityResponse{
		PostID:      post.ID,
		GeneratedAt: time.Now().UTC(),
		Evaluation:  eval,
	}
	emit(onEvent, SSEEventComplete, resp)
	return resp, nil
}

// inputHash fingerprints everything that determines an assessment's stored
// result: the rendered system and user prompts plus the model id (what the
// model sees), and the resolved weight profile (which ComposeScore folds
// into OverallPct and each dimension's Weight/Contribution — values the
// model never produces). The assess flow compares it against the stored
// hash to decide whether to re-run (CON-92); including the profile means a
// weights config change invalidates the cache rather than serving a stale
// score. A unit-separator between parts prevents one field's content from
// bleeding into the next.
func inputHash(prompts *renderedPrompts, modelID string, profile Profile) string {
	h := sha256.New()
	h.Write([]byte(prompts.system))
	h.Write([]byte{0x1f})
	h.Write([]byte(prompts.user))
	h.Write([]byte{0x1f})
	h.Write([]byte(modelID))
	h.Write([]byte{0x1f})
	fmt.Fprintf(h, "%g,%g,%g,%g", profile.Correctness, profile.Clarity, profile.Engagement, profile.Delivery)
	return hex.EncodeToString(h.Sum(nil))
}

// persist upserts the evaluation (overwriting any prior run for the post)
// and records the operation in PostLog. The PostLog append is best-effort:
// a logging failure does not fail the evaluation.
func persist(
	ctx context.Context,
	repos PostQualityRepos,
	post *models.Post,
	result models.EvaluationResult,
	overall float64,
	captionScoped bool,
	modelID string,
	hash string,
) (*models.PostEvaluation, error) {
	id, err := models.NewID()
	if err != nil {
		return nil, fmt.Errorf("mint evaluation id: %w", err)
	}
	eval := &models.PostEvaluation{
		ID:               id,
		PostID:           post.ID,
		PlatformID:       post.PlatformID,
		PlatformPostType: post.PlatformPostType,
		CaptionScoped:    captionScoped,
		OverallPct:       overall,
		Result:           result,
		ModelID:          modelID,
		InputHash:        hash,
	}
	if err := repos.Evaluations.Upsert(ctx, eval); err != nil {
		return nil, err
	}
	appendQualityLog(ctx, repos, post.ID, eval)
	return eval, nil
}

// appendQualityLog records the assessment in the Post's audit history.
// Best-effort — failures are logged, not propagated.
func appendQualityLog(ctx context.Context, repos PostQualityRepos, postID string, eval *models.PostEvaluation) {
	if repos.PostLogs == nil {
		return
	}
	logID, err := models.NewID()
	if err != nil {
		log.Printf("post_quality[%s]: cannot mint log id: %v", postID, err)
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"overallPct":  eval.OverallPct,
		"modelId":     eval.ModelID,
		"correctness": eval.Result.Correctness.Score,
		"clarity":     eval.Result.Clarity.Score,
		"engagement":  eval.Result.Engagement.Score,
		"delivery":    eval.Result.Delivery.Score,
	})
	if err := repos.PostLogs.Append(ctx, &models.PostLog{
		ID:        logID,
		PostID:    postID,
		EventType: models.PostLogEventQualityAssessed,
		Actor:     models.ActorSystem,
		Summary:   fmt.Sprintf("quality assessed: %.0f%%", eval.OverallPct),
		Payload:   logs.SanitizeAndCap(string(payload)),
	}); err != nil {
		log.Printf("post_quality[%s]: logs append failed: %v", postID, err)
	}
}

// publishAssessmentFinalised announces the end of an assessment run on the
// shared event hub. Topic is "entity:post:<id>"; type is
// "assessment_completed" on success, "assessment_failed" on error.
func publishAssessmentFinalised(
	hub eventhub.Hub,
	postID, ownerID string,
	resp *PostQualityResponse,
	err error,
) {
	if hub == nil {
		return
	}
	id, idErr := models.NewID()
	if idErr != nil {
		log.Printf("post_quality: cannot mint event id: %v", idErr)
		return
	}
	ev := eventhub.Event{
		ID:     id,
		Topic:  "entity:post:" + postID,
		UserID: ownerID,
	}
	if err != nil {
		ev.Type = "assessment_failed"
		ev.Payload = map[string]any{"postId": postID, "error": err.Error()}
	} else {
		overall := 0.0
		if resp != nil && resp.Evaluation != nil {
			overall = resp.Evaluation.OverallPct
		}
		ev.Type = "assessment_completed"
		ev.Payload = map[string]any{"postId": postID, "overallPct": overall}
	}
	if pubErr := hub.Publish(context.Background(), ev); pubErr != nil {
		log.Printf("post_quality[%s]: hub publish failed: %v", postID, pubErr)
	}
}
