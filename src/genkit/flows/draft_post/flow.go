package draft_post

import (
	"context"
	"embed"
	"fmt"
	"text/template"

	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
)

//go:embed prompts/draft_post.tmpl
var promptFS embed.FS

// DraftPostFlow is the singleton Genkit flow. Set by InitDraftPost. It is
// registered for Dev-UI discovery; the SSE path uses the runner closure below
// instead so it can stream per-post events.
var DraftPostFlow *core.Flow[DraftPostRequest, *DraftPostResponse, struct{}]

// draftPostRunner is the direct closure that threads an OnEventFunc for SSE
// streaming. Set by InitDraftPost.
var draftPostRunner func(ctx context.Context, req DraftPostRequest, onEvent OnEventFunc) (*DraftPostResponse, error)

// InitDraftPost parses the prompt template and registers the draftPost Genkit
// flow. Must be called after the Genkit instance has been initialised with the
// Anthropic plugin.
func InitDraftPost(g *genkit.Genkit, cfg DraftPostFlowConfig, repos DraftPostRepos) error {
	raw, err := promptFS.ReadFile("prompts/draft_post.tmpl")
	if err != nil {
		return fmt.Errorf("load draft_post.tmpl: %w", err)
	}
	tmpl, err := template.New("draft_post").Parse(string(raw))
	if err != nil {
		return fmt.Errorf("parse draft_post.tmpl: %w", err)
	}
	cfg.systemTmpl = tmpl.Lookup("system")
	cfg.contextTmpl = tmpl.Lookup("context")
	if cfg.systemTmpl == nil || cfg.contextTmpl == nil {
		return fmt.Errorf("draft_post.tmpl must define both {{define \"system\"}} and {{define \"context\"}} blocks")
	}

	DraftPostFlow = genkit.DefineFlow(g, "draftPost",
		func(ctx context.Context, req DraftPostRequest) (*DraftPostResponse, error) {
			return runDraftPost(ctx, g, req, cfg, repos, nil)
		},
	)

	draftPostRunner = func(ctx context.Context, req DraftPostRequest, onEvent OnEventFunc) (*DraftPostResponse, error) {
		return runDraftPost(ctx, g, req, cfg, repos, onEvent)
	}

	return nil
}

// NewDraftPostCallback returns a callback suitable for the campaign assistant
// tool. onEvent is forwarded to the flow for SSE streaming; pass nil for a
// silent, non-streaming call.
func NewDraftPostCallback() func(ctx context.Context, req DraftPostRequest, onEvent OnEventFunc) (*DraftPostResponse, error) {
	return func(ctx context.Context, req DraftPostRequest, onEvent OnEventFunc) (*DraftPostResponse, error) {
		return draftPostRunner(ctx, req, onEvent)
	}
}

// emit calls onEvent when it is non-nil. It is a safe no-op otherwise.
func emit(onEvent OnEventFunc, name SSEEventKind, data any) {
	if onEvent != nil {
		onEvent(name, data)
	}
}
