package server

import (
	"context"
	"fmt"
	"log"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai/anthropic"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/genkit/flows/content_plan"
)

// initContentPlan initialises the Anthropic-backed content plan flow and
// returns a synchronous callback suitable for the campaigns handler.
// Returns (nil, nil) when AnthropicAPIKey is empty so the feature degrades
// gracefully without crashing the server.
func initContentPlan(
	ctx context.Context,
	cfg *config.Config,
	embedder ai.Embedder,
	repos content_plan.ContentPlanRepos,
) (func(ctx context.Context, campaignID string) (*content_plan.ContentPlanResponse, error), error) {
	if cfg.AnthropicAPIKey == "" {
		log.Println("content_plan: ANTHROPIC_API_KEY not set — generate-draft feature disabled")
		return nil, nil
	}

	g := genkit.Init(ctx, genkit.WithPlugins(&anthropic.Anthropic{}))

	flowCfg := content_plan.ContentPlanFlowConfig{
		ModelID:   cfg.ModelID,
		MaxPieces: cfg.MaxPieceContext,
		Embedder:  embedder,
	}
	if err := content_plan.InitContentPlan(g, flowCfg, repos); err != nil {
		return nil, fmt.Errorf("init content plan flow: %w", err)
	}

	return content_plan.NewContentPlanCallback(), nil
}
