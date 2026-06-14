package server

import (
	"context"
	"fmt"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/ogen-app/ogen/src/config"
	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/genkit/flows/post_assistant"
	"github.com/ogen-app/ogen/src/postclone"
	"github.com/ogen-app/ogen/src/postrestore"
)

// initPostAssistant registers the post assistant flow on the shared Genkit
// instance and returns a callback for the posts handler.
func initPostAssistant(
	g *genkit.Genkit,
	cfg *config.Config,
	embedder ai.Embedder,
	hub eventhub.Hub,
	repos post_assistant.PostAssistantRepos,
	cloneSvc *postclone.Service,
	restoreSvc *postrestore.Service,
) (func(ctx context.Context, req post_assistant.PostAssistantRequest, onEvent post_assistant.OnEventFunc) (*post_assistant.PostAssistantResponse, error), error) {
	flowCfg := post_assistant.PostAssistantFlowConfig{
		ModelID:         cfg.ModelID,
		MaxOutputTokens: cfg.MaxOutputTokens,
		Embedder:        embedder,
		Hub:             hub,
		CloneService:    cloneSvc,
		RestoreService:  restoreSvc,
	}
	if err := post_assistant.InitPostAssistant(g, flowCfg, repos); err != nil {
		return nil, fmt.Errorf("init post assistant flow: %w", err)
	}

	return post_assistant.NewPostAssistantCallback(), nil
}
