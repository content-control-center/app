package post_assistant

import (
	"context"
	"embed"
	"fmt"
	"text/template"

	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
)

//go:embed prompts/post_assistant.tmpl
var promptFS embed.FS

// PostAssistantFlow is the singleton Genkit flow. Set by InitPostAssistant.
var PostAssistantFlow *core.Flow[PostAssistantRequest, *PostAssistantResponse, struct{}]

// postAssistantRunner is the direct closure that bypasses the Genkit flow
// wrapper, allowing per-request state injection.
var postAssistantRunner func(ctx context.Context, req PostAssistantRequest) (*PostAssistantResponse, error)

// InitPostAssistant registers the postAssistant Genkit flow and its tools.
// Must be called after the Genkit instance has been initialised with the
// Anthropic plugin.
func InitPostAssistant(g *genkit.Genkit, cfg PostAssistantFlowConfig, repos PostAssistantRepos) error {
	raw, err := promptFS.ReadFile("prompts/post_assistant.tmpl")
	if err != nil {
		return fmt.Errorf("load post_assistant.tmpl: %w", err)
	}
	tmpl, err := template.New("post_assistant").Parse(string(raw))
	if err != nil {
		return fmt.Errorf("parse post_assistant.tmpl: %w", err)
	}
	systemTmpl := tmpl.Lookup("system")
	contextTmpl := tmpl.Lookup("context")
	if systemTmpl == nil || contextTmpl == nil {
		return fmt.Errorf("post_assistant.tmpl must define both {{define \"system\"}} and {{define \"context\"}} blocks")
	}

	tools := defineTools(g)

	PostAssistantFlow = genkit.DefineFlow(g, "postAssistant",
		func(ctx context.Context, req PostAssistantRequest) (*PostAssistantResponse, error) {
			return runPostAssistant(ctx, g, req, cfg, repos, systemTmpl, contextTmpl, tools)
		},
	)

	postAssistantRunner = func(ctx context.Context, req PostAssistantRequest) (*PostAssistantResponse, error) {
		return runPostAssistant(ctx, g, req, cfg, repos, systemTmpl, contextTmpl, tools)
	}

	return nil
}

// NewPostAssistantCallback returns a callback suitable for passing to the
// posts handler.
func NewPostAssistantCallback() func(ctx context.Context, req PostAssistantRequest) (*PostAssistantResponse, error) {
	return func(ctx context.Context, req PostAssistantRequest) (*PostAssistantResponse, error) {
		return postAssistantRunner(ctx, req)
	}
}
