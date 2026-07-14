package campaign_assistant

import (
	"context"

	"github.com/ogen-app/ogen/src/campaignoverview"
	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/genkit/flows/content_plan"
	"github.com/ogen-app/ogen/src/genkit/flows/enrich_brief"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/usage"
	"github.com/ogen-app/ogen/src/vendors/llm"
)

// CampaignAssistantRequest is the input to the campaignAssistant flow — one
// conversational turn scoped to a single campaign.
type CampaignAssistantRequest struct {
	CampaignID  string `json:"campaignId"`
	Instruction string `json:"instruction"`
}

// CampaignAssistantResponse is the structured output for one turn. Explanation
// is ordered first so it streams to the client before the metadata. ContentPlan
// and Brief are populated by the server (not the model) when the matching tool
// ran this turn.
type CampaignAssistantResponse struct {
	Explanation string `json:"explanation"          jsonschema:"description=Conversational reply to the user"`
	Action      string `json:"action"               jsonschema:"description=answered for a grounded reply; content_plan_generated when the runContentPlan tool ran; brief_enriched when the enrichBrief tool ran; declined when the request is out of scope,enum=answered,enum=content_plan_generated,enum=brief_enriched,enum=declined"`
	// ContentPlan is set by the server when the runContentPlan tool created
	// draft posts this turn. Action is then "content_plan_generated".
	ContentPlan *ContentPlanResult `json:"contentPlan,omitempty" jsonschema:"-"`
	// Brief is set by the server when the enrichBrief tool applied a new brief
	// to the campaign this turn. Action is then "brief_enriched".
	Brief *BriefResult `json:"brief,omitempty" jsonschema:"-"`
}

// ContentPlanResult summarises a runContentPlan tool invocation.
type ContentPlanResult struct {
	PostCount int      `json:"postCount"`
	Warnings  []string `json:"warnings,omitempty"`
}

// BriefResult reports whether the enrichBrief tool applied a new brief.
type BriefResult struct {
	Applied bool `json:"applied"`
}

// CampaignAssistantRepos bundles the repository dependencies for the flow.
type CampaignAssistantRepos struct {
	Messages  repository.CampaignAssistantMessageRepository
	Campaigns repository.CampaignRepository
	// Posts backs the listCampaignPosts read tool used for grounded Q&A.
	Posts repository.PostRepository
}

// CampaignAssistantFlowConfig holds static settings for the flow.
type CampaignAssistantFlowConfig struct {
	// Provider resolves the model reference + call config by role. The
	// orchestration/routing loop runs on RolePlanning (cheap/fast); the
	// content_plan / enrich_brief sub-flows keep their own generation model.
	Provider *llm.Provider
	// Recorder captures the planner's usage event; nil disables recording.
	Recorder *usage.Recorder
	// Checker gates the flow against the tenant's spend caps; nil = no gate.
	Checker *usage.Checker
	ModelID string
	// MaxOutputTokens caps a single planner call. 0 falls back to 8192 — the
	// planner only emits a short JSON envelope, never long prose.
	MaxOutputTokens int64
	// MaxTurns caps tool-use round-trips. 0 falls back to 4 (routing rarely
	// needs more than one tool call plus the final answer).
	MaxTurns int
	// Hub publishes "operation finalised" events on success/failure. nil = silent.
	Hub eventhub.Hub

	// ContentPlan and EnrichBrief are the existing flow callbacks, invoked as
	// tools. Injected so the assistant reuses them verbatim — no duplicate LLM
	// generation. nil disables the corresponding tool.
	ContentPlan func(ctx context.Context, campaignID string, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error)
	EnrichBrief func(ctx context.Context, req enrich_brief.EnrichBriefRequest, onEvent enrich_brief.OnEventFunc) (*enrich_brief.EnrichBriefResponse, error)
	// Overview backs the getCampaignOverview read tool (CON-113). nil disables
	// the tool.
	Overview *campaignoverview.Service
}

// ValidationError is returned when preconditions are not met (HTTP 400).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// AIError is returned when the model call fails or returns nothing usable
// (HTTP 502).
type AIError struct{ Msg string }

func (e *AIError) Error() string { return e.Msg }

// ── SSE types ─────────────────────────────────────────────────────────────────

// SSEEventKind identifies the SSE event types emitted by the assistant flow.
// The content_plan_* and enrich_brief_* events are the nested sub-flow events
// re-emitted namespaced so the client sees live progress while a tool runs.
type SSEEventKind string

const (
	SSEEventExplanationDelta SSEEventKind = "explanation_delta"
	SSEEventToolCall         SSEEventKind = "tool_call"
	SSEEventToolResult       SSEEventKind = "tool_result"

	SSEEventContentPlanStarted  SSEEventKind = "content_plan_started"
	SSEEventContentPlanStep     SSEEventKind = "content_plan_step"
	SSEEventContentPlanPost     SSEEventKind = "content_plan_post"
	SSEEventContentPlanWarning  SSEEventKind = "content_plan_warning"
	SSEEventContentPlanComplete SSEEventKind = "content_plan_complete"

	SSEEventEnrichBriefStarted          SSEEventKind = "enrich_brief_started"
	SSEEventEnrichBriefDescriptionDelta SSEEventKind = "enrich_brief_description_delta"
	SSEEventEnrichBriefPersonaDelta     SSEEventKind = "enrich_brief_persona_delta"
	SSEEventEnrichBriefMessagesDelta    SSEEventKind = "enrich_brief_messages_delta"
	SSEEventEnrichBriefToneDelta        SSEEventKind = "enrich_brief_tone_delta"
	SSEEventEnrichBriefComplete         SSEEventKind = "enrich_brief_complete"

	SSEEventComplete SSEEventKind = "complete"
	SSEEventError    SSEEventKind = "error"
)

// DeltaEventPayload carries a fragment of a streamed string value.
type DeltaEventPayload struct {
	Delta string `json:"delta"`
}

// ToolCallEventPayload is emitted once per fully-formed tool request.
type ToolCallEventPayload struct {
	Name  string `json:"name"`
	Input any    `json:"input,omitempty"`
	Ref   string `json:"ref,omitempty"`
}

// ToolResultEventPayload is emitted once a tool invocation returns.
type ToolResultEventPayload struct {
	Name string `json:"name"`
	Ref  string `json:"ref,omitempty"`
	OK   bool   `json:"ok"`
}

// ContentPlanStartedEventPayload is emitted when the runContentPlan tool begins.
type ContentPlanStartedEventPayload struct{}

// ContentPlanCompleteEventPayload is emitted once the content plan is persisted.
type ContentPlanCompleteEventPayload struct {
	PostCount int      `json:"postCount"`
	Warnings  []string `json:"warnings,omitempty"`
}

// EnrichBriefStartedEventPayload is emitted when the enrichBrief tool begins.
type EnrichBriefStartedEventPayload struct {
	Instruction string `json:"instruction,omitempty"`
}

// EnrichBriefCompleteEventPayload is emitted once the brief is applied.
type EnrichBriefCompleteEventPayload struct {
	Applied bool `json:"applied"`
}

// ErrorEventPayload is emitted when the flow fails mid-stream.
type ErrorEventPayload struct {
	Message string `json:"message"`
	Code    int    `json:"code"` // HTTP semantic: 400, 402, 502, 500
}

// OnEventFunc is an optional callback invoked as SSE events are produced.
// A nil OnEventFunc is valid — the flow runs silently.
type OnEventFunc func(name SSEEventKind, data any)
