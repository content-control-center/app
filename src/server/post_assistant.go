package server

import (
	"context"
	"fmt"
	"log"

	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai/anthropic"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/genkit/flows/post_assistant"
)

// initPostAssistant initialises the Anthropic-backed post assistant flow and
// returns a callback suitable for the posts handler.
// Returns (nil, nil) when AnthropicAPIKey is empty so the feature degrades
// gracefully without crashing the server.
func initPostAssistant(
	ctx context.Context,
	cfg *config.Config,
	repos post_assistant.PostAssistantRepos,
) (func(ctx context.Context, req post_assistant.PostAssistantRequest) (*post_assistant.PostAssistantResponse, error), error) {
	if cfg.AnthropicAPIKey == "" {
		log.Println("post_assistant: ANTHROPIC_API_KEY not set — post assistant feature disabled")
		return nil, nil
	}

	g := genkit.Init(ctx, genkit.WithPlugins(&anthropic.Anthropic{}))

	flowCfg := post_assistant.PostAssistantFlowConfig{
		ModelID:         cfg.ModelID,
		MaxOutputTokens: cfg.MaxOutputTokens,
	}
	if err := post_assistant.InitPostAssistant(g, flowCfg, repos); err != nil {
		return nil, fmt.Errorf("init post assistant flow: %w", err)
	}

	return post_assistant.NewPostAssistantCallback(), nil
}
