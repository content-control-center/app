package campaign_assistant

import (
	"context"
	"fmt"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/ogen-app/ogen/src/campaign_actions/overview"
	"github.com/ogen-app/ogen/src/genkit/flows/content_plan"
	"github.com/ogen-app/ogen/src/genkit/flows/enrich_brief"
	"github.com/ogen-app/ogen/src/models"
)

// ── Context key for per-request state ────────────────────────────────────────

type ctxKey int

const requestStateKey ctxKey = iota

// requestState is the per-turn state the tools read and write. The runner sets
// it before generation and reads the *Result fields afterwards to finalise the
// response (mirrors post_assistant).
type requestState struct {
	campaignID string
	campaign   *models.Campaign
	repos      CampaignAssistantRepos
	onEvent    OnEventFunc

	// Injected sub-flow callbacks, invoked as tools.
	contentPlan func(ctx context.Context, campaignID string, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error)
	enrichBrief func(ctx context.Context, req enrich_brief.EnrichBriefRequest, onEvent enrich_brief.OnEventFunc) (*enrich_brief.EnrichBriefResponse, error)
	// overview backs the getCampaignOverview read tool (CON-113).
	overview *overview.Service

	// Results set by tools, read by the runner after generation.
	contentPlanResult *ContentPlanResult
	briefResult       *BriefResult
}

func withRequestState(ctx context.Context, s *requestState) context.Context {
	return context.WithValue(ctx, requestStateKey, s)
}

func getRequestState(ctx context.Context) *requestState {
	return ctx.Value(requestStateKey).(*requestState)
}

// ── Tool input/output types ──────────────────────────────────────────────────

// EnrichBriefInput is the input for the enrichBrief tool.
type EnrichBriefInput struct {
	Instruction string `json:"instruction,omitempty" jsonschema:"description=Optional freeform steering for the brief (e.g. make it more B2B and technical). Omit to enrich from the campaign's title and type alone."`
}

// EnrichBriefOutput is returned to the model after the brief is applied.
type EnrichBriefOutput struct {
	Description    string `json:"description"`
	TargetPersona  string `json:"targetPersona"`
	KeyMessages    string `json:"keyMessages"`
	ToneGuidelines string `json:"toneGuidelines"`
	Applied        bool   `json:"applied"`
}

// RunContentPlanOutput is returned to the model after a content plan runs.
type RunContentPlanOutput struct {
	PostCount    int `json:"postCount"`
	WarningCount int `json:"warningCount"`
}

// CampaignPostInfo is a single element of the listCampaignPosts output.
type CampaignPostInfo struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	PlatformID string `json:"platformId,omitempty"`
	Status     string `json:"status"`
}

// ── Tool registration ────────────────────────────────────────────────────────

type toolSet struct {
	runContentPlan      ai.ToolRef
	enrichBrief         ai.ToolRef
	listCampaignPosts   ai.ToolRef
	getCampaignOverview ai.ToolRef
}

func defineTools(g *genkit.Genkit) *toolSet {
	runContentPlan := genkit.DefineTool(g, "runContentPlan",
		"Generates a full content plan (a set of draft posts across the campaign's platforms) for the current campaign and saves the posts. "+
			"Call this when the user asks to generate, create, or regenerate a content plan. Takes no arguments. Returns how many posts were created.",
		func(ctx *ai.ToolContext, _ struct{}) (*RunContentPlanOutput, error) {
			return toolRunContentPlan(ctx)
		},
	)

	enrichBrief := genkit.DefineTool(g, "enrichBrief",
		"Improves the campaign brief (description, target persona, key messages, tone guidelines) and saves it to the campaign automatically. "+
			"Call this when the user asks to enrich, improve, refine, or rewrite the brief. Pass the user's steering as instruction when they give any. Returns the applied brief.",
		func(ctx *ai.ToolContext, in EnrichBriefInput) (*EnrichBriefOutput, error) {
			return toolEnrichBrief(ctx, in)
		},
	)

	listCampaignPosts := genkit.DefineTool(g, "listCampaignPosts",
		"Lists the campaign's existing posts with their id, title, platform, and status. Use it to answer questions about what's already in the campaign. Takes no arguments.",
		func(ctx *ai.ToolContext, _ struct{}) ([]CampaignPostInfo, error) {
			return toolListCampaignPosts(ctx)
		},
	)

	getCampaignOverview := genkit.DefineTool(g, "getCampaignOverview",
		"Returns a quick overview of the campaign: its phases (with per-phase post counts) and how the content is distributed by status, platform, and content type. "+
			"Use it for questions like 'give me a quick overview', 'how is the content distributed', 'which phase has the most/least content', or 'what's the state of this campaign'. Takes no arguments.",
		func(ctx *ai.ToolContext, _ struct{}) (*overview.Overview, error) {
			return toolGetCampaignOverview(ctx)
		},
	)

	return &toolSet{
		runContentPlan:      runContentPlan,
		enrichBrief:         enrichBrief,
		listCampaignPosts:   listCampaignPosts,
		getCampaignOverview: getCampaignOverview,
	}
}

// ── Tool implementations ─────────────────────────────────────────────────────

func toolRunContentPlan(ctx context.Context) (*RunContentPlanOutput, error) {
	st := getRequestState(ctx)
	if st.contentPlan == nil {
		return nil, fmt.Errorf("content plan generation is not available")
	}

	emit(st.onEvent, SSEEventContentPlanStarted, ContentPlanStartedEventPayload{})

	// Forward the sub-flow's native events, namespaced, so the client sees
	// posts stream in live while the tool runs (execution-time optimisation).
	nested := content_plan.OnEventFunc(func(name content_plan.SSEEventKind, data any) {
		switch name {
		case content_plan.SSEEventStep:
			emit(st.onEvent, SSEEventContentPlanStep, data)
		case content_plan.SSEEventPost:
			emit(st.onEvent, SSEEventContentPlanPost, data)
		case content_plan.SSEEventWarning:
			emit(st.onEvent, SSEEventContentPlanWarning, data)
			// nested complete/error are handled at the tool-return level below.
		}
	})

	resp, err := st.contentPlan(ctx, st.campaignID, nested)
	if err != nil {
		return nil, err
	}

	res := &ContentPlanResult{PostCount: len(resp.Posts), Warnings: resp.Warnings}
	st.contentPlanResult = res
	return &RunContentPlanOutput{PostCount: res.PostCount, WarningCount: len(res.Warnings)}, nil
}

func toolEnrichBrief(ctx context.Context, in EnrichBriefInput) (*EnrichBriefOutput, error) {
	st := getRequestState(ctx)
	if st.enrichBrief == nil {
		return nil, fmt.Errorf("brief enrichment is not available")
	}

	emit(st.onEvent, SSEEventEnrichBriefStarted, EnrichBriefStartedEventPayload{Instruction: in.Instruction})

	nested := enrich_brief.OnEventFunc(func(name enrich_brief.SSEEventKind, data any) {
		switch name {
		case enrich_brief.SSEEventDescriptionDelta:
			emit(st.onEvent, SSEEventEnrichBriefDescriptionDelta, data)
		case enrich_brief.SSEEventPersonaDelta:
			emit(st.onEvent, SSEEventEnrichBriefPersonaDelta, data)
		case enrich_brief.SSEEventMessagesDelta:
			emit(st.onEvent, SSEEventEnrichBriefMessagesDelta, data)
		case enrich_brief.SSEEventToneDelta:
			emit(st.onEvent, SSEEventEnrichBriefToneDelta, data)
		}
	})

	resp, err := st.enrichBrief(ctx, enrich_brief.EnrichBriefRequest{
		CampaignID:  st.campaignID,
		Instruction: in.Instruction,
	}, nested)
	if err != nil {
		return nil, err
	}

	// Auto-apply (CON-112): write the enriched brief straight to the campaign.
	// The four fields map 1:1 to EnrichBriefResponse. Update is tenant-scoped
	// via the TenantScoped BeforeUpdate hook.
	c := st.campaign
	c.Description = resp.Description
	c.TargetPersona = resp.TargetPersona
	c.KeyMessages = resp.KeyMessages
	c.ToneGuidelines = resp.ToneGuidelines
	c.UpdatedAt = time.Now().UTC()
	if err := st.repos.Campaigns.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("apply enriched brief: %w", err)
	}

	st.briefResult = &BriefResult{Applied: true}
	return &EnrichBriefOutput{
		Description:    resp.Description,
		TargetPersona:  resp.TargetPersona,
		KeyMessages:    resp.KeyMessages,
		ToneGuidelines: resp.ToneGuidelines,
		Applied:        true,
	}, nil
}

func toolListCampaignPosts(ctx context.Context) ([]CampaignPostInfo, error) {
	st := getRequestState(ctx)
	posts, err := st.repos.Posts.ListByCampaign(ctx, st.campaignID)
	if err != nil {
		return nil, fmt.Errorf("list campaign posts: %w", err)
	}
	out := make([]CampaignPostInfo, 0, len(posts))
	for _, p := range posts {
		out = append(out, CampaignPostInfo{
			ID:         p.ID,
			Title:      p.Title,
			PlatformID: p.PlatformID,
			Status:     string(p.Status),
		})
	}
	return out, nil
}

func toolGetCampaignOverview(ctx context.Context) (*overview.Overview, error) {
	st := getRequestState(ctx)
	if st.overview == nil {
		return nil, fmt.Errorf("campaign overview is not available")
	}
	return st.overview.Overview(ctx, st.campaignID)
}
