package consistency

import (
	"context"

	"github.com/firebase/genkit/go/genkit"
)

// checkBriefRunner / checkPostsRunner are the direct closures over (g, cfg,
// repos) that thread an OnEventFunc for SSE streaming. Set by InitConsistency.
var (
	checkBriefRunner func(ctx context.Context, campaignID string, onEvent OnEventFunc) (*BriefReview, error)
	checkPostsRunner func(ctx context.Context, req PostsCheckRequest, onEvent OnEventFunc) (*PostsReview, error)
)

// InitConsistency wires the read-only consistency review runners onto the given
// Genkit instance. There is no genkit.DefineFlow registration (nothing needs
// Dev-UI discovery); the callbacks below invoke the runners directly. Must be
// called after the Genkit instance has the Anthropic plugin.
func InitConsistency(g *genkit.Genkit, cfg ConsistencyFlowConfig, repos ConsistencyRepos) error {
	tmpl, err := loadTemplates()
	if err != nil {
		return err
	}
	cfg.tmpl = tmpl

	checkBriefRunner = func(ctx context.Context, campaignID string, onEvent OnEventFunc) (*BriefReview, error) {
		return runCheckBrief(ctx, g, campaignID, cfg, repos, onEvent)
	}
	checkPostsRunner = func(ctx context.Context, req PostsCheckRequest, onEvent OnEventFunc) (*PostsReview, error) {
		return runCheckPosts(ctx, g, req, cfg, repos, onEvent)
	}
	return nil
}

// NewCheckBriefCallback returns the brief-review callback for the campaigns
// handler and the campaign_assistant checkBrief tool.
func NewCheckBriefCallback() func(ctx context.Context, campaignID string, onEvent OnEventFunc) (*BriefReview, error) {
	return func(ctx context.Context, campaignID string, onEvent OnEventFunc) (*BriefReview, error) {
		return checkBriefRunner(ctx, campaignID, onEvent)
	}
}

// NewCheckPostsCallback returns the posts-review callback.
func NewCheckPostsCallback() func(ctx context.Context, req PostsCheckRequest, onEvent OnEventFunc) (*PostsReview, error) {
	return func(ctx context.Context, req PostsCheckRequest, onEvent OnEventFunc) (*PostsReview, error) {
		return checkPostsRunner(ctx, req, onEvent)
	}
}

// emit calls onEvent when it is non-nil. Safe no-op otherwise.
func emit(onEvent OnEventFunc, name SSEEventKind, data any) {
	if onEvent != nil {
		onEvent(name, data)
	}
}
