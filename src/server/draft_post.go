package server

import (
	"context"
	"fmt"

	"github.com/firebase/genkit/go/genkit"

	"github.com/ogen-app/ogen/src/config"
	"github.com/ogen-app/ogen/src/genkit/flows/draft_post"
	"github.com/ogen-app/ogen/src/usage"
	"github.com/ogen-app/ogen/src/vendors/llm"
)

// initDraftPost registers the draftPost generation flow (CON-207) on the shared
// Genkit instance and returns an SSE-capable callback for the campaign assistant
// tool. Copywriting runs on the generation role (Sonnet-tier, cfg.ModelID), like
// content_plan; MaxOutputTokens is left at 0 so the flow uses its own default.
func initDraftPost(
	g *genkit.Genkit,
	cfg *config.Config,
	provider *llm.Provider,
	recorder *usage.Recorder,
	checker *usage.Checker,
	repos draft_post.DraftPostRepos,
) (func(ctx context.Context, req draft_post.DraftPostRequest, onEvent draft_post.OnEventFunc) (*draft_post.DraftPostResponse, error), error) {
	flowCfg := draft_post.DraftPostFlowConfig{
		Provider: provider,
		Recorder: recorder,
		Checker:  checker,
		ModelID:  cfg.ModelID,
	}
	if err := draft_post.InitDraftPost(g, flowCfg, repos); err != nil {
		return nil, fmt.Errorf("init draft post flow: %w", err)
	}
	return draft_post.NewDraftPostCallback(), nil
}
