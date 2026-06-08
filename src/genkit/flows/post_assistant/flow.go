package post_assistant

import (
	"context"
	"embed"
	"fmt"
	"log"
	"text/template"

	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"

	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/models"
)

//go:embed prompts/post_assistant.tmpl
var promptFS embed.FS

// PostAssistantFlow is the singleton Genkit flow. Set by InitPostAssistant.
var PostAssistantFlow *core.Flow[PostAssistantRequest, *PostAssistantResponse, struct{}]

// postAssistantRunner is the direct closure that bypasses the Genkit flow
// wrapper, allowing per-request state injection and SSE event streaming.
var postAssistantRunner func(ctx context.Context, req PostAssistantRequest, onEvent OnEventFunc) (*PostAssistantResponse, error)

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
			return runPostAssistant(ctx, g, req, cfg, repos, systemTmpl, contextTmpl, tools, nil)
		},
	)

	postAssistantRunner = func(ctx context.Context, req PostAssistantRequest, onEvent OnEventFunc) (*PostAssistantResponse, error) {
		return runPostAssistant(ctx, g, req, cfg, repos, systemTmpl, contextTmpl, tools, onEvent)
	}

	return nil
}

// NewPostAssistantCallback returns a callback suitable for passing to the
// posts handler. onEvent is forwarded to the flow for SSE streaming; pass
// nil for a silent, non-streaming call.
func NewPostAssistantCallback() func(ctx context.Context, req PostAssistantRequest, onEvent OnEventFunc) (*PostAssistantResponse, error) {
	return func(ctx context.Context, req PostAssistantRequest, onEvent OnEventFunc) (*PostAssistantResponse, error) {
		return postAssistantRunner(ctx, req, onEvent)
	}
}

// emit calls onEvent when it is non-nil. It is a safe no-op otherwise.
func emit(onEvent OnEventFunc, name SSEEventKind, data any) {
	if onEvent != nil {
		onEvent(name, data)
	}
}

// publishAssistantFinalised announces the end of an assistant run on the
// shared event hub. Topic is "entity:post:<id>"; type is
// "assistant_completed" on success, "assistant_failed" on error.
//
// Used by the SSE event-stream consumer to drive cross-tab notifications
// (the per-request /assistant SSE already serves the requesting tab).
func publishAssistantFinalised(
	hub eventhub.Hub,
	postID, ownerID string,
	resp *PostAssistantResponse,
	err error,
) {
	if hub == nil {
		return
	}
	id, idErr := models.NewID()
	if idErr != nil {
		log.Printf("post_assistant: cannot mint event id: %v", idErr)
		return
	}
	ev := eventhub.Event{
		ID:     id,
		Topic:  "entity:post:" + postID,
		UserID: ownerID,
	}
	if err != nil {
		ev.Type = "assistant_failed"
		ev.Payload = map[string]any{
			"postId": postID,
			"error":  err.Error(),
		}
	} else {
		action := ""
		saveVersion := false
		if resp != nil {
			action = resp.Action
			saveVersion = resp.SaveVersion
		}
		ev.Type = "assistant_completed"
		ev.Payload = map[string]any{
			"postId":      postID,
			"action":      action,
			"saveVersion": saveVersion,
		}
	}
	if pubErr := hub.Publish(context.Background(), ev); pubErr != nil {
		log.Printf("post_assistant[%s]: hub publish failed: %v", postID, pubErr)
	}
}
