package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai/anthropic"
	"github.com/openai/openai-go/option"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/genkit/flows/content_plan"
)

// initContentPlan initialises the Anthropic-backed content plan flow and
// returns an SSE-capable callback suitable for the campaigns handler.
// Returns (nil, nil) when AnthropicAPIKey is empty so the feature degrades
// gracefully without crashing the server.
func initContentPlan(
	ctx context.Context,
	cfg *config.Config,
	embedder ai.Embedder,
	repos content_plan.ContentPlanRepos,
) (func(ctx context.Context, campaignID string, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error), error) {
	if cfg.AnthropicAPIKey == "" {
		log.Println("content_plan: ANTHROPIC_API_KEY not set — generate-draft feature disabled")
		return nil, nil
	}

	// Use a custom HTTP client with no read timeout so long streaming responses
	// (e.g. 30-post generation) are not cut off mid-stream. A 30-second header
	// timeout still guards against hung connections that never start responding.
	streamingClient := &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}
	// Normalise the Content-Type header to exactly "text/event-stream" so that
	// the ssefix decoder registered in ssefix.go is always selected, even when
	// Anthropic includes extra parameters (e.g. "text/event-stream; charset=utf-8").
	normaliseContentType := option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		resp, err := next(req)
		if err != nil || resp == nil {
			return resp, err
		}
		ct := resp.Header.Get("Content-Type")
		if strings.Contains(ct, "text/event-stream") && ct != "text/event-stream" {
			resp.Header.Set("Content-Type", "text/event-stream")
		}
		return resp, nil
	})
	g := genkit.Init(ctx, genkit.WithPlugins(&anthropic.Anthropic{
		Opts: []option.RequestOption{
			option.WithHTTPClient(streamingClient),
			normaliseContentType,
		},
	}))

	flowCfg := content_plan.ContentPlanFlowConfig{
		ModelID:         cfg.ModelID,
		MaxAssets:       cfg.MaxAssetContext,
		MaxOutputTokens: cfg.MaxOutputTokens,
		Embedder:        embedder,
	}
	if err := content_plan.InitContentPlan(g, flowCfg, repos); err != nil {
		return nil, fmt.Errorf("init content plan flow: %w", err)
	}

	return content_plan.NewContentPlanCallback(), nil
}
