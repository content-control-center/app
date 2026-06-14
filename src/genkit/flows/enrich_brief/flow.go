package enrich_brief

import (
	"context"
	"embed"
	"fmt"
	"text/template"

	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
)

//go:embed prompts/enrich_brief.tmpl
var promptFS embed.FS

// EnrichBriefFlow is the singleton Genkit flow. Set by InitEnrichBrief.
// It is registered for Dev-UI discovery; the SSE path uses the runner
// closure below instead so it can stream per-field events.
var EnrichBriefFlow *core.Flow[EnrichBriefRequest, *EnrichBriefResponse, struct{}]

// enrichBriefRunner is the direct closure that threads an OnEventFunc for
// SSE streaming. Set by InitEnrichBrief.
var enrichBriefRunner func(ctx context.Context, req EnrichBriefRequest, onEvent OnEventFunc) (*EnrichBriefResponse, error)

// InitEnrichBrief parses the prompt template and registers the enrichBrief
// Genkit flow. Must be called after the Genkit instance has been
// initialised with the Anthropic plugin.
func InitEnrichBrief(g *genkit.Genkit, cfg EnrichBriefFlowConfig, repos EnrichBriefRepos) error {
	raw, err := promptFS.ReadFile("prompts/enrich_brief.tmpl")
	if err != nil {
		return fmt.Errorf("load enrich_brief.tmpl: %w", err)
	}
	tmpl, err := template.New("enrich_brief").Parse(string(raw))
	if err != nil {
		return fmt.Errorf("parse enrich_brief.tmpl: %w", err)
	}
	systemTmpl := tmpl.Lookup("system")
	contextTmpl := tmpl.Lookup("context")
	if systemTmpl == nil || contextTmpl == nil {
		return fmt.Errorf("enrich_brief.tmpl must define both {{define \"system\"}} and {{define \"context\"}} blocks")
	}

	EnrichBriefFlow = genkit.DefineFlow(g, "enrichBrief",
		func(ctx context.Context, req EnrichBriefRequest) (*EnrichBriefResponse, error) {
			return runEnrichBrief(ctx, g, req, cfg, repos, systemTmpl, contextTmpl, nil)
		},
	)

	enrichBriefRunner = func(ctx context.Context, req EnrichBriefRequest, onEvent OnEventFunc) (*EnrichBriefResponse, error) {
		return runEnrichBrief(ctx, g, req, cfg, repos, systemTmpl, contextTmpl, onEvent)
	}

	return nil
}

// NewEnrichBriefCallback returns a callback suitable for passing to the
// campaigns handler. onEvent is forwarded to the flow for SSE streaming;
// pass nil for a silent, non-streaming call.
func NewEnrichBriefCallback() func(ctx context.Context, req EnrichBriefRequest, onEvent OnEventFunc) (*EnrichBriefResponse, error) {
	return func(ctx context.Context, req EnrichBriefRequest, onEvent OnEventFunc) (*EnrichBriefResponse, error) {
		return enrichBriefRunner(ctx, req, onEvent)
	}
}

// emit calls onEvent when it is non-nil. It is a safe no-op otherwise.
func emit(onEvent OnEventFunc, name SSEEventKind, data any) {
	if onEvent != nil {
		onEvent(name, data)
	}
}
