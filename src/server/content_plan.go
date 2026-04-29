package server

import (
	"context"
	"fmt"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/eventhub"
	"github.com/content-control-center/app/src/genkit/flows/content_plan"
)

// initContentPlan registers the content plan flow on the shared Genkit
// instance and returns an SSE-capable callback for the campaigns handler.
func initContentPlan(
	g *genkit.Genkit,
	cfg *config.Config,
	embedder ai.Embedder,
	hub eventhub.Hub,
	repos content_plan.ContentPlanRepos,
) (func(ctx context.Context, campaignID string, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error), error) {
	flowCfg := content_plan.ContentPlanFlowConfig{
		ModelID:            cfg.ModelID,
		MaxContextAssets:   cfg.MaxContextAssets,
		MaxContextChars:    cfg.MaxContextChars,
		MaxOutputTokens:    cfg.MaxOutputTokens,
		MaxPostsPerBatch:   cfg.MaxPostsPerBatch,
		MaxParallelBatches: cfg.MaxParallelBatches,
		Embedder:           embedder,
		Hub:                hub,
	}
	if err := content_plan.InitContentPlan(g, flowCfg, repos); err != nil {
		return nil, fmt.Errorf("init content plan flow: %w", err)
	}

	return content_plan.NewContentPlanCallback(), nil
}
