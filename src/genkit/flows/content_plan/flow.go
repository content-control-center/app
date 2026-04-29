package content_plan

import (
	"context"
	"embed"
	"fmt"
	"log"
	"text/template"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"

	"github.com/content-control-center/app/src/eventhub"
	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

//go:embed prompts/content_plan.tmpl
var promptFS embed.FS

// ContentPlanFlow is the singleton flow for generating a content plan.
// Set by InitContentPlan; nil until then.
var ContentPlanFlow *core.Flow[ContentPlanRequest, *ContentPlanResponse, struct{}]

// contentPlanRunner is a direct closure over (g, cfg, repos) that bypasses
// the Genkit flow wrapper, allowing an OnEventFunc to be threaded through.
// Set by InitContentPlan alongside ContentPlanFlow.
var contentPlanRunner func(ctx context.Context, req ContentPlanRequest, onEvent OnEventFunc) (*ContentPlanResponse, error)

// ContentPlanFlowConfig holds the settings for the content plan flow.
type ContentPlanFlowConfig struct {
	ModelID          string
	MaxContextAssets int         // max assets when no embedder (creation-order fallback)
	MaxContextChars  int         // character budget for asset context in the prompt
	MaxOutputTokens  int64       // max_tokens sent to the model; 0 falls back to 8192
	// MaxPostsPerBatch caps the number of posts the model is asked to
	// produce in a single batched call. Sized so 800 tokens/post stays
	// comfortably under MaxOutputTokens with headroom for slower
	// per-post tail latencies. 0 falls back to 30.
	MaxPostsPerBatch int
	// MaxParallelBatches caps how many batches run concurrently — a
	// safety knob against Anthropic per-account rate limits when very
	// large campaigns produce many batches. 0 falls back to 5.
	MaxParallelBatches int
	Embedder           ai.Embedder // nil = skip semantic ranking, fall back to creation order
	// Hub is the event broker used to publish "operation finalised"
	// events on success/failure. nil = silent (no events emitted).
	Hub        eventhub.Hub
	systemTmpl *template.Template
	userTmpl   *template.Template
}

// ContentPlanRepos bundles all repository dependencies for the flow.
type ContentPlanRepos struct {
	Campaigns  repository.CampaignRepository
	Assets     repository.AssetRepository
	Chunks     repository.AssetChunksRepository
	Platforms  repository.PlatformRepository
	Posts      repository.PostRepository
}

// InitContentPlan registers the generateContentPlan Genkit flow. It must be
// called after the Genkit instance has been initialised with the Anthropic
// plugin.
func InitContentPlan(g *genkit.Genkit, cfg ContentPlanFlowConfig, repos ContentPlanRepos) error {
	raw, err := promptFS.ReadFile("prompts/content_plan.tmpl")
	if err != nil {
		return fmt.Errorf("load content_plan.tmpl: %w", err)
	}
	tmpl, err := template.New("content_plan").Parse(string(raw))
	if err != nil {
		return fmt.Errorf("parse content_plan.tmpl: %w", err)
	}
	cfg.systemTmpl = tmpl.Lookup("system")
	cfg.userTmpl = tmpl.Lookup("user")
	if cfg.systemTmpl == nil || cfg.userTmpl == nil {
		return fmt.Errorf("content_plan.tmpl must define both {{define \"system\"}} and {{define \"user\"}} blocks")
	}

	ContentPlanFlow = genkit.DefineFlow(g, "generateContentPlan",
		func(ctx context.Context, req ContentPlanRequest) (*ContentPlanResponse, error) {
			return runContentPlan(ctx, g, req, cfg, repos, nil)
		},
	)

	contentPlanRunner = func(ctx context.Context, req ContentPlanRequest, onEvent OnEventFunc) (*ContentPlanResponse, error) {
		return runContentPlan(ctx, g, req, cfg, repos, onEvent)
	}

	return nil
}

// NewContentPlanCallback returns a callback suitable for passing to the
// campaigns handler. onEvent is forwarded to the flow for SSE streaming;
// pass nil for a silent, non-streaming call.
func NewContentPlanCallback() func(ctx context.Context, campaignID string, onEvent OnEventFunc) (*ContentPlanResponse, error) {
	return func(ctx context.Context, campaignID string, onEvent OnEventFunc) (*ContentPlanResponse, error) {
		return contentPlanRunner(ctx, ContentPlanRequest{CampaignID: campaignID}, onEvent)
	}
}

// emit calls onEvent when it is non-nil. It is a safe no-op otherwise.
func emit(onEvent OnEventFunc, name SSEEventKind, data any) {
	if onEvent != nil {
		onEvent(name, data)
	}
}

// publishContentPlanFinalised announces the end of a content-plan run on
// the shared event hub. Topic is "entity:campaign:<id>"; type is
// "content_plan_completed" on success, "content_plan_failed" on error.
func publishContentPlanFinalised(
	hub eventhub.Hub,
	campaignID, ownerID string,
	resp *ContentPlanResponse,
	err error,
) {
	if hub == nil {
		return
	}
	id, idErr := models.NewID()
	if idErr != nil {
		log.Printf("content_plan: cannot mint event id: %v", idErr)
		return
	}
	ev := eventhub.Event{
		ID:     id,
		Topic:  "entity:campaign:" + campaignID,
		UserID: ownerID,
	}
	if err != nil {
		ev.Type = "content_plan_failed"
		ev.Payload = map[string]any{
			"campaignId": campaignID,
			"error":      err.Error(),
		}
	} else {
		postCount := 0
		warningCount := 0
		if resp != nil {
			postCount = len(resp.Posts)
			warningCount = len(resp.Warnings)
		}
		ev.Type = "content_plan_completed"
		ev.Payload = map[string]any{
			"campaignId":   campaignID,
			"postCount":    postCount,
			"warningCount": warningCount,
		}
	}
	if pubErr := hub.Publish(context.Background(), ev); pubErr != nil {
		log.Printf("content_plan[%s]: hub publish failed: %v", campaignID, pubErr)
	}
}

// runContentPlan executes the six steps of the flow.
func runContentPlan(
	ctx context.Context,
	g *genkit.Genkit,
	req ContentPlanRequest,
	cfg ContentPlanFlowConfig,
	repos ContentPlanRepos,
	onEvent OnEventFunc,
) (out *ContentPlanResponse, retErr error) {
	start := time.Now()
	log.Printf("content_plan[%s]: starting", req.CampaignID)

	// finaliseOwnerID is captured once the campaign is loaded so the
	// deferred finalisation event can be scoped to the campaign owner.
	// Empty before validateInput → finalisation events for very-early
	// failures are skipped.
	var finaliseOwnerID string

	defer func() {
		if cfg.Hub == nil || finaliseOwnerID == "" {
			return
		}
		publishContentPlanFinalised(cfg.Hub, req.CampaignID, finaliseOwnerID, out, retErr)
	}()

	// ── Step 1: validateInput ─────────────────────────────────────────────────
	log.Printf("content_plan[%s]: step 1/6 validateInput", req.CampaignID)
	campaign, err := validateInput(ctx, req.CampaignID, repos.Campaigns, repos.Assets)
	if err != nil {
		log.Printf("content_plan[%s]: validateInput failed after %s: %v", req.CampaignID, time.Since(start).Round(time.Millisecond), err)
		return nil, err
	}
	finaliseOwnerID = campaign.CreatedBy
	log.Printf("content_plan[%s]: validateInput done (campaign=%q platforms=%d)", req.CampaignID, campaign.Name, len(campaign.TargetPlatforms))
	emit(onEvent, SSEEventStep, StepEventPayload{Step: "validateInput", Status: "done"})

	// ── Step 2: resolveAssets ─────────────────────────────────────────────────
	log.Printf("content_plan[%s]: step 2/6 resolveAssets (useAssets=%v)", req.CampaignID, campaign.UseAssets)
	assets, assetWarnings, err := resolveAssets(ctx, campaign, cfg, repos)
	if err != nil {
		log.Printf("content_plan[%s]: resolveAssets failed after %s: %v", req.CampaignID, time.Since(start).Round(time.Millisecond), err)
		return nil, err
	}
	log.Printf("content_plan[%s]: resolveAssets done (%d assets, %d warnings)", req.CampaignID, len(assets), len(assetWarnings))
	emit(onEvent, SSEEventStep, StepEventPayload{Step: "resolveAssets", Status: "done"})
	warnings := assetWarnings

	// ── Step 3: resolvePlatforms ──────────────────────────────────────────────
	log.Printf("content_plan[%s]: step 3/6 resolvePlatforms", req.CampaignID)
	platforms, err := resolvePlatforms(ctx, campaign.TargetPlatforms, repos.Platforms)
	if err != nil {
		log.Printf("content_plan[%s]: resolvePlatforms failed after %s: %v", req.CampaignID, time.Since(start).Round(time.Millisecond), err)
		return nil, err
	}
	if len(platforms) == 0 {
		return nil, &ValidationError{Msg: "none of the campaign's target platforms could be resolved — check that platform IDs are valid"}
	}
	if len(platforms) < len(campaign.TargetPlatforms) {
		log.Printf("content_plan[%s]: WARNING resolvePlatforms resolved %d/%d platforms — some IDs may be stale", req.CampaignID, len(platforms), len(campaign.TargetPlatforms))
	}
	log.Printf("content_plan[%s]: resolvePlatforms done (%d platforms)", req.CampaignID, len(platforms))
	emit(onEvent, SSEEventStep, StepEventPayload{Step: "resolvePlatforms", Status: "done"})

	// ── Step 4: generatePosts ─────────────────────────────────────────────────
	log.Printf("content_plan[%s]: step 4/6 generatePosts (model=%s estimatedCount=%d)", req.CampaignID, cfg.ModelID, campaign.EstimatedPostCount)
	posts, genWarnings, err := generatePosts(ctx, g, campaign, platforms, assets, cfg, onEvent)
	if err != nil {
		log.Printf("content_plan[%s]: generatePosts failed after %s: %v", req.CampaignID, time.Since(start).Round(time.Millisecond), err)
		return nil, err
	}
	log.Printf("content_plan[%s]: generatePosts done (%d posts, %d warnings)", req.CampaignID, len(posts), len(genWarnings))
	emit(onEvent, SSEEventStep, StepEventPayload{Step: "generatePosts", Status: "done"})
	warnings = append(warnings, genWarnings...)

	// ── Step 5: validateOutput ────────────────────────────────────────────────
	log.Printf("content_plan[%s]: step 5/6 validateOutput", req.CampaignID)
	validPosts, valWarnings := validateOutput(posts, campaign, platforms)
	log.Printf("content_plan[%s]: validateOutput done (%d valid, %d dropped)", req.CampaignID, len(validPosts), len(posts)-len(validPosts))
	emit(onEvent, SSEEventStep, StepEventPayload{Step: "validateOutput", Status: "done"})
	warnings = append(warnings, valWarnings...)

	// ── Step 6: persistDraftPosts ─────────────────────────────────────────────
	log.Printf("content_plan[%s]: step 6/6 persistDraftPosts (%d posts)", req.CampaignID, len(validPosts))
	if err := persistDraftPosts(ctx, validPosts, campaign, repos.Posts); err != nil {
		log.Printf("content_plan[%s]: persistDraftPosts failed after %s: %v", req.CampaignID, time.Since(start).Round(time.Millisecond), err)
		return nil, err
	}
	emit(onEvent, SSEEventStep, StepEventPayload{Step: "persistDraftPosts", Status: "done"})

	log.Printf("content_plan[%s]: done in %s (%d posts, %d total warnings)", req.CampaignID, time.Since(start).Round(time.Millisecond), len(validPosts), len(warnings))
	return &ContentPlanResponse{
		CampaignID:  campaign.ID,
		GeneratedAt: time.Now().UTC(),
		Posts:       validPosts,
		Warnings:    warnings,
	}, nil
}
