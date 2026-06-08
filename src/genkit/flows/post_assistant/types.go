package post_assistant

import (
	"github.com/firebase/genkit/go/ai"

	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/repository"
)

// PostAssistantRequest is the input to the postAssistant flow.
type PostAssistantRequest struct {
	PostID      string `json:"postId"`
	Instruction string `json:"instruction"`
}

// PostAssistantResponse is the structured output from the assistant.
// Fields are ordered so that Explanation and UpdatedContent appear first
// in the model's JSON output — this lets them start streaming to the
// client before the smaller metadata fields are generated.
type PostAssistantResponse struct {
	Explanation    string `json:"explanation"                  jsonschema:"description=Human-readable explanation of what was changed or why the request was declined"`
	UpdatedContent string `json:"updatedContent"               jsonschema:"description=The full updated post content as Markdown; empty when action is declined"`
	Action         string `json:"action"                       jsonschema:"description=edited when content was changed or declined when the request is out of scope,enum=edited,enum=declined"`
	SaveVersion    bool   `json:"saveVersion"                  jsonschema:"description=True when a new version snapshot should be created"`
	VersionNote    string `json:"versionNote,omitempty"        jsonschema:"description=Short note describing the version; only present when saveVersion is true"`
}

// PostAssistantRepos bundles all repository dependencies for the flow.
type PostAssistantRepos struct {
	Posts     repository.PostRepository
	Assets    repository.AssetRepository
	Chunks    repository.AssetChunksRepository
	Campaigns repository.CampaignRepository
	Versions  repository.PostVersionRepository
	Messages  repository.PostAssistantMessageRepository
}

// PostAssistantFlowConfig holds settings for the post assistant flow.
type PostAssistantFlowConfig struct {
	ModelID string
	// MaxOutputTokens caps the model's output for a single call. 0 falls
	// back to 64000 — Claude 4.x Haiku/Sonnet's max output. Anthropic
	// charges only for tokens actually emitted, so a generous cap costs
	// nothing on short responses but prevents truncation when the
	// explanation + full updated content + tool inputs combined exceed
	// a smaller cap. When Anthropic stops at the cap, the response's
	// FinishReason is FinishReasonLength and we log "TRUNCATED" loudly.
	MaxOutputTokens int64
	// MaxTurns caps tool-use round-trips. The model needs one extra turn
	// for the final answer after its last tool call, so MaxTurns=N allows
	// up to N-1 tool calls. 0 falls back to a sensible default (8) that
	// covers realistic asset-incorporation scenarios (browse + search a
	// few assets + read chunks + respond).
	MaxTurns int
	Embedder ai.Embedder // nil = semantic search unavailable
	// Hub is the event broker used to publish "operation finalised"
	// events on success/failure. nil = silent (no events emitted).
	Hub eventhub.Hub
}

// ValidationError is returned when preconditions are not met (HTTP 400).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// AIError is returned when the model call fails (HTTP 502).
type AIError struct{ Msg string }

func (e *AIError) Error() string { return e.Msg }

// ── SSE types ─────────────────────────────────────────────────────────────────

// SSEEventKind identifies the SSE event types emitted by the assistant flow.
type SSEEventKind string

const (
	SSEEventExplanationDelta SSEEventKind = "explanation_delta"
	SSEEventContentDelta     SSEEventKind = "content_delta"
	SSEEventToolCall         SSEEventKind = "tool_call"
	SSEEventToolResult       SSEEventKind = "tool_result"
	SSEEventComplete         SSEEventKind = "complete"
	SSEEventError            SSEEventKind = "error"
)

// DeltaEventPayload carries a fragment of a streamed string value.
type DeltaEventPayload struct {
	Delta string `json:"delta"`
}

// ToolCallEventPayload is emitted once per fully-formed tool request from
// the model. Partial streaming fragments of the tool input are suppressed
// to avoid noisy per-character events.
type ToolCallEventPayload struct {
	Name  string `json:"name"`
	Input any    `json:"input,omitempty"`
	Ref   string `json:"ref,omitempty"`
}

// ToolResultEventPayload is emitted once a tool invocation returns.
// Output bytes are intentionally omitted — the client only needs a "done"
// signal to resolve the matching tool_call chip.
type ToolResultEventPayload struct {
	Name string `json:"name"`
	Ref  string `json:"ref,omitempty"`
	OK   bool   `json:"ok"`
}

// ErrorEventPayload is emitted when the flow fails.
type ErrorEventPayload struct {
	Message string `json:"message"`
	Code    int    `json:"code"` // HTTP semantic: 400, 502, 500
}

// OnEventFunc is an optional callback invoked as SSE events are produced.
// A nil OnEventFunc is valid — the flow runs silently.
type OnEventFunc func(name SSEEventKind, data any)
