package server

import (
	"context"
	"fmt"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/ogen-app/ogen/src/campaign_actions/overview"
	"github.com/ogen-app/ogen/src/config"
	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/genkit/flows/campaign_assistant"
	"github.com/ogen-app/ogen/src/genkit/flows/consistency"
	"github.com/ogen-app/ogen/src/genkit/flows/content_plan"
	"github.com/ogen-app/ogen/src/genkit/flows/enrich_brief"
	"github.com/ogen-app/ogen/src/usage"
	"github.com/ogen-app/ogen/src/vendors/llm"
)

// initCampaignAssistant registers the campaign assistant flow on the shared
// Genkit instance and returns a callback for the campaigns handler. It reuses
// the already-registered content_plan and enrich_brief callbacks as tools
// (CON-112), so it must be initialised after those two.
func initCampaignAssistant(
	g *genkit.Genkit,
	cfg *config.Config,
	provider *llm.Provider,
	recorder *usage.Recorder,
	checker *usage.Checker,
	embedder ai.Embedder,
	hub eventhub.Hub,
	repos campaign_assistant.CampaignAssistantRepos,
	contentPlanFn func(ctx context.Context, campaignID string, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error),
	enrichBriefFn func(ctx context.Context, req enrich_brief.EnrichBriefRequest, onEvent enrich_brief.OnEventFunc) (*enrich_brief.EnrichBriefResponse, error),
	overviewSvc *overview.Service,
	generatePostsFn func(ctx context.Context, req content_plan.GeneratePostsRequest, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error),
	checkBriefFn func(ctx context.Context, campaignID string, onEvent consistency.OnEventFunc) (*consistency.BriefReview, error),
	checkPostsFn func(ctx context.Context, req consistency.PostsCheckRequest, onEvent consistency.OnEventFunc) (*consistency.PostsReview, error),
) (func(ctx context.Context, req campaign_assistant.CampaignAssistantRequest, onEvent campaign_assistant.OnEventFunc) (*campaign_assistant.CampaignAssistantResponse, error), error) {
	flowCfg := campaign_assistant.CampaignAssistantFlowConfig{
		Provider: provider,
		Recorder: recorder,
		Checker:  checker,
		Embedder: embedder,
		ModelID:  cfg.PlanningModelID,
		// Router slimming (CON-112 perf): the planner only emits a short JSON
		// envelope (explanation + action) plus tool calls, so 2048 is ample and
		// bounds worst-case streaming.
		//
		// MaxTurns bounds tool-use round-trips. It was 2 (CON-112), but that
		// double-counted "turns" and heavy sub-flow cost: the cheap Haiku planner
		// legitimately chains a couple of read tools (getCampaignOverview →
		// listCampaignPosts → act) and blew the budget, surfacing a hard
		// "exceeded maximum tool call iterations" 502 (CON-213). We now give the
		// read chain room (4) and keep the real cost guard — "at most one heavy
		// Sonnet sub-flow per turn" — enforced precisely inside the tools
		// themselves (see requestState.heavyActionRan), not via a blunt turn cap.
		MaxOutputTokens: 2048,
		MaxTurns:        4,
		Hub:             hub,
		// CON-112: pre-warm the strict-tool grammar cache at boot when we're also
		// stabilizing tool order (otherwise the warmed key wouldn't match).
		PrewarmTools:     cfg.AnthropicStableToolOrder,
		ContentPlan:      contentPlanFn,
		EnrichBrief:      enrichBriefFn,
		Overview:         overviewSvc,
		GeneratePosts:    generatePostsFn,
		MaxGeneratePosts: cfg.GeneratePostsMax,
		CheckBrief:       checkBriefFn,
		CheckPosts:       checkPostsFn,
	}
	if err := campaign_assistant.InitCampaignAssistant(g, flowCfg, repos); err != nil {
		return nil, fmt.Errorf("init campaign assistant flow: %w", err)
	}
	return campaign_assistant.NewCampaignAssistantCallback(), nil
}
