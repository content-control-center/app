package server

import (
	"context"
	"fmt"

	"github.com/firebase/genkit/go/genkit"

	"github.com/ogen-app/ogen/src/config"
	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/genkit/flows/post_quality"
)

// initPostQuality registers the assessPostQuality flow on the shared
// Genkit instance and returns an SSE-capable callback for the posts
// handler. The weight profiles come from config (empty → built-in
// defaults); a malformed override fails here so the misconfiguration
// surfaces at boot rather than at first request.
func initPostQuality(
	g *genkit.Genkit,
	cfg *config.Config,
	hub eventhub.Hub,
	repos post_quality.PostQualityRepos,
) (func(ctx context.Context, postID string, onEvent post_quality.OnEventFunc) (*post_quality.PostQualityResponse, error), error) {
	weights, err := post_quality.WeightsFromJSON(cfg.QualityWeightProfiles)
	if err != nil {
		return nil, fmt.Errorf("post quality weight profiles: %w", err)
	}

	flowCfg := post_quality.PostQualityFlowConfig{
		ModelID:         cfg.QualityModelID,
		MaxOutputTokens: cfg.MaxOutputTokens,
		Weights:         weights,
		Hub:             hub,
	}
	if err := post_quality.InitPostQuality(g, flowCfg, repos); err != nil {
		return nil, fmt.Errorf("init post quality flow: %w", err)
	}

	return post_quality.NewPostQualityCallback(), nil
}
