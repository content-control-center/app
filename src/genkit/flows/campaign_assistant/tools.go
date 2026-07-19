package campaign_assistant

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/pgvector/pgvector-go"

	"github.com/ogen-app/ogen/src/campaign_actions/overview"
	"github.com/ogen-app/ogen/src/campaign_actions/reschedule"
	"github.com/ogen-app/ogen/src/genkit/embedopts"
	"github.com/ogen-app/ogen/src/genkit/flows/consistency"
	"github.com/ogen-app/ogen/src/genkit/flows/content_plan"
	"github.com/ogen-app/ogen/src/genkit/flows/enrich_brief"
	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
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
	// embedder backs the askCampaignAssets read tool (CON-118); nil / unavailable
	// degrades that tool to "search unavailable".
	embedder ai.Embedder

	// Injected sub-flow callbacks, invoked as tools.
	contentPlan func(ctx context.Context, campaignID string, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error)
	enrichBrief func(ctx context.Context, req enrich_brief.EnrichBriefRequest, onEvent enrich_brief.OnEventFunc) (*enrich_brief.EnrichBriefResponse, error)
	// overview backs the getCampaignOverview read tool (CON-113).
	overview *overview.Service
	// generatePosts backs the generatePosts targeted-generation tool (CON-114);
	// maxGeneratePosts caps a single call.
	generatePosts    func(ctx context.Context, req content_plan.GeneratePostsRequest, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error)
	maxGeneratePosts int
	// checkBrief / checkPosts back the read-only consistency review tools (CON-116).
	checkBrief func(ctx context.Context, campaignID string, onEvent consistency.OnEventFunc) (*consistency.BriefReview, error)
	checkPosts func(ctx context.Context, req consistency.PostsCheckRequest, onEvent consistency.OnEventFunc) (*consistency.PostsReview, error)

	// Results set by tools, read by the runner after generation.
	contentPlanResult    *ContentPlanResult
	briefResult          *BriefResult
	generatedPostsResult *GeneratedPostsResult
	datesResult          *DatesResult
	redistributeResult   *RedistributeResult
	briefReviewResult    *consistency.BriefReview
	postsReviewResult    *consistency.PostsReview
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
	PostCount    int        `json:"postCount"`
	WarningCount int        `json:"warningCount"`
	UsedAssets   []AssetRef `json:"usedAssets,omitempty"` // CON-118: assets that informed the plan
}

// CampaignPostInfo is a single element of the listCampaignPosts output.
type CampaignPostInfo struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	PlatformID string `json:"platformId,omitempty"`
	Status     string `json:"status"`
}

// GeneratePostsInput is the input for the generatePosts tool (CON-114). The
// model resolves the timeframe against today's date (shown in context) into an
// ISO window before calling.
type GeneratePostsInput struct {
	Platforms   []string `json:"platforms"             jsonschema:"description=Platform names or ids to generate for, e.g. [\"Threads\"]. Must be platforms the campaign already targets."`
	Phase       string   `json:"phase,omitempty"       jsonschema:"description=Phase name, id, or \"current\"; omit for the current phase."`
	Count       int      `json:"count,omitempty"       jsonschema:"description=Number of posts to add: the exact number the user names (\"add 1 post\"->1, \"5 articles\"->5), or 3 for a vague \"a few\"/\"some\". Always set it; if omitted only 1 post is created. Capped per call."`
	WindowStart string   `json:"windowStart,omitempty" jsonschema:"description=First publish date (ISO YYYY-MM-DD), resolved from the requested timeframe against today."`
	WindowEnd   string   `json:"windowEnd,omitempty"   jsonschema:"description=Last publish date (ISO YYYY-MM-DD). For a single specific date, set this equal to windowStart."`
	PostType    string   `json:"postType,omitempty"    jsonschema:"description=Optional post-type slug (e.g. text-post, article); omit for the platform default."`
}

// GeneratePostsOutput is returned to the model after targeted posts are created.
type GeneratePostsOutput struct {
	PostCount      int        `json:"postCount"`
	RequestedCount int        `json:"requestedCount"`
	Clamped        bool       `json:"clamped"` // true when requestedCount exceeded the per-call cap
	PhaseID        string     `json:"phaseId"`
	PhaseName      string     `json:"phaseName"`
	Platforms      []string   `json:"platforms"`       // resolved platform names
	Dates          []string   `json:"dates,omitempty"` // CON-114: actual publish dates of the created posts, so the model reports them instead of inventing dates
	Warnings       []string   `json:"warnings,omitempty"`
	UsedAssets     []AssetRef `json:"usedAssets,omitempty"` // CON-118: assets that informed the posts
}

// SetCampaignDatesInput is the input for the setCampaignDates tool (CON-115).
// The model resolves relative phrasing ("beginning of July") against today.
type SetCampaignDatesInput struct {
	StartDate string `json:"startDate,omitempty" jsonschema:"description=New campaign start date (ISO YYYY-MM-DD); omit to leave it unchanged."`
	EndDate   string `json:"endDate,omitempty"   jsonschema:"description=New campaign end date (ISO YYYY-MM-DD); omit to leave it unchanged."`
}

// SetCampaignDatesOutput is returned to the model after the dates are saved.
type SetCampaignDatesOutput struct {
	StartDate         string `json:"startDate"`
	EndDate           string `json:"endDate"`
	PostsOutsideRange int    `json:"postsOutsideRange"` // eligible (draft/ready) posts now dated outside the new range
}

// RedistributePostsOutput is returned to the model after redistribution.
type RedistributePostsOutput struct {
	PostsUpdated int `json:"postsUpdated"`
	PhaseCount   int `json:"phaseCount"`
}

// CheckPostsInput is the input for the checkPostsConsistency tool (CON-116).
type CheckPostsInput struct {
	Max int `json:"max,omitempty" jsonschema:"description=Optional cap on how many posts to review; omit for the default."`
}

// AskCampaignAssetsInput is the input for the askCampaignAssets read tool (CON-118).
type AskCampaignAssetsInput struct {
	Query string `json:"query" jsonschema:"description=The question to answer from the campaign's attached assets, e.g. 'what does the pricing PDF say about enterprise tiers?'"`
}

// AskCampaignAssetsOutput returns the asset excerpts most relevant to the query
// for the planner to answer from (CON-118).
type AskCampaignAssetsOutput struct {
	Excerpts  []AssetExcerpt `json:"excerpts"`
	Available bool           `json:"available"`      // false when asset search could not run
	Note      string         `json:"note,omitempty"` // status when excerpts is empty
}

// AssetExcerpt is one relevant chunk of an attached asset.
type AssetExcerpt struct {
	AssetID string `json:"assetId"`
	Title   string `json:"title"`
	Pages   string `json:"pages,omitempty"`
	Text    string `json:"text"`
}

// ── Tool registration ────────────────────────────────────────────────────────

type toolSet struct {
	runContentPlan        ai.ToolRef
	enrichBrief           ai.ToolRef
	listCampaignPosts     ai.ToolRef
	getCampaignOverview   ai.ToolRef
	generatePosts         ai.ToolRef
	setCampaignDates      ai.ToolRef
	redistributePosts     ai.ToolRef
	checkBrief            ai.ToolRef
	checkPostsConsistency ai.ToolRef
	askCampaignAssets     ai.ToolRef
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

	generatePosts := genkit.DefineTool(g, "generatePosts",
		"Adds a few NEW draft posts to the campaign for a specific platform, phase, and timeframe — e.g. 'add a few Threads posts in the current phase for the upcoming weeks'. "+
			"Different from runContentPlan (which regenerates the WHOLE plan across all platforms/phases). Only platforms the campaign already targets are allowed. "+
			"Resolve the requested timeframe into windowStart/windowEnd (ISO dates) using today's date shown in the context. Returns how many posts were created.",
		func(ctx *ai.ToolContext, in GeneratePostsInput) (*GeneratePostsOutput, error) {
			return toolGeneratePosts(ctx, in)
		},
	)

	setCampaignDates := genkit.DefineTool(g, "setCampaignDates",
		"Changes the campaign's start and/or end date. Call this when the user asks to move, shift, extend, or shorten the campaign's dates (e.g. 'move the campaign end to the beginning of July', 'push the start to next Monday'). "+
			"Resolve any relative phrasing into ISO YYYY-MM-DD dates using today's date shown in the context; pass only the field(s) that change. The change is saved automatically. "+
			"If the reply reports posts now outside the new range, offer to redistribute them (do not redistribute unless the user asks).",
		func(ctx *ai.ToolContext, in SetCampaignDatesInput) (*SetCampaignDatesOutput, error) {
			return toolSetCampaignDates(ctx, in)
		},
	)

	redistributePosts := genkit.DefineTool(g, "redistributePosts",
		"Redistributes the publish dates of the campaign's non-published posts (drafts and ready-for-publish) evenly across the campaign timeline, phase by phase. "+
			"Call this when the user asks to redistribute, re-spread, rebalance, or re-schedule the drafts / unpublished posts. It never moves already-scheduled or published posts. Takes no arguments.",
		func(ctx *ai.ToolContext, _ struct{}) (*RedistributePostsOutput, error) {
			return toolRedistributePosts(ctx)
		},
	)

	checkBrief := genkit.DefineTool(g, "checkBrief",
		"Reviews the campaign brief for internal consistency and completeness (goal alignment, persona, key messages, tone) and returns specific findings with suggestions. "+
			"Read-only — it does NOT change the brief. Call it when the user asks to check/review the brief or whether it's consistent. Takes no arguments. "+
			"After reporting the findings, OFFER to improve the brief with enrichBrief when there are issues, but do NOT call enrichBrief in the same turn unless the user asks.",
		func(ctx *ai.ToolContext, _ struct{}) (*consistency.BriefReview, error) {
			return toolCheckBrief(ctx)
		},
	)

	checkPostsConsistency := genkit.DefineTool(g, "checkPostsConsistency",
		"Checks whether the campaign's non-published posts follow the brief (persona, key messages, tone) and returns per-post findings for the ones that drift. "+
			"Read-only — it does NOT change any post. Call it when the user asks whether the posts match/follow the brief. Only the first N posts are checked (report the cap when it applies).",
		func(ctx *ai.ToolContext, in CheckPostsInput) (*consistency.PostsReview, error) {
			return toolCheckPostsConsistency(ctx, in)
		},
	)

	askCampaignAssets := genkit.DefineTool(g, "askCampaignAssets",
		"Answers a question grounded in the campaign's attached assets (uploaded PDFs, markdown, etc.). "+
			"Call this when the user asks what an attached asset says or wants facts pulled from the campaign's assets — e.g. \"what does the pricing PDF say about enterprise tiers?\". "+
			"Read-only: it returns the most relevant excerpts (with asset title + page) for you to answer from; it does NOT generate posts. "+
			"If the result reports available:false or no excerpts, tell the user you couldn't search / find matching asset content.",
		func(ctx *ai.ToolContext, in AskCampaignAssetsInput) (*AskCampaignAssetsOutput, error) {
			return toolAskCampaignAssets(ctx, in)
		},
	)

	return &toolSet{
		runContentPlan:        runContentPlan,
		enrichBrief:           enrichBrief,
		listCampaignPosts:     listCampaignPosts,
		getCampaignOverview:   getCampaignOverview,
		generatePosts:         generatePosts,
		setCampaignDates:      setCampaignDates,
		redistributePosts:     redistributePosts,
		checkBrief:            checkBrief,
		checkPostsConsistency: checkPostsConsistency,
		askCampaignAssets:     askCampaignAssets,
	}
}

// ── Tool implementations ─────────────────────────────────────────────────────

func toolRunContentPlan(ctx context.Context) (*RunContentPlanOutput, error) {
	st := getRequestState(ctx)
	if st.contentPlan == nil {
		return nil, fmt.Errorf("content plan generation is not available")
	}

	// CON-118: generate from the campaign's attached assets when it has any.
	ensureCampaignAssetUse(ctx, st)

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

	// CON-118: report which attached assets informed the plan.
	used := toAssetRefs(resp.UsedAssets)
	if len(used) > 0 {
		emit(st.onEvent, SSEEventAssetsUsed, AssetsUsedEventPayload{Assets: used})
	}

	res := &ContentPlanResult{PostCount: len(resp.Posts), Warnings: resp.Warnings, UsedAssets: used}
	st.contentPlanResult = res
	return &RunContentPlanOutput{PostCount: res.PostCount, WarningCount: len(res.Warnings), UsedAssets: used}, nil
}

// toAssetRefs maps the content_plan provenance list into the assistant's local
// AssetRef type for SSE events + tool output (CON-118).
func toAssetRefs(in []content_plan.AssetRef) []AssetRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]AssetRef, len(in))
	for i, a := range in {
		out[i] = AssetRef{ID: a.ID, Title: a.Title}
	}
	return out
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

func toolGeneratePosts(ctx context.Context, in GeneratePostsInput) (*GeneratePostsOutput, error) {
	st := getRequestState(ctx)
	if st.generatePosts == nil {
		return nil, fmt.Errorf("targeted post generation is not available")
	}
	// CON-118: generate from the campaign's attached assets when it has any.
	ensureCampaignAssetUse(ctx, st)
	campaign := st.campaign
	now := time.Now().UTC()

	// Resolve platform names/ids against the campaign's target platforms
	// (CON-114 "stay in scope").
	platformIDs, platformNames, err := resolveTargetPlatforms(campaign, in.Platforms)
	if err != nil {
		return nil, err
	}

	// Resolve the phase — "current"/empty derives from the campaign timeline.
	phaseID, phaseName, err := resolvePhase(campaign, in.Phase, now)
	if err != nil {
		return nil, err
	}

	// Count: honor an explicit number exactly (so "add 1 post" yields 1); fall
	// back to 1 (the safe minimum) when the model omitted it; clamp to the
	// per-call cap. Extracted as resolveGenerateCount for unit testing.
	count, requested, clamped := resolveGenerateCount(in.Count, st.maxGeneratePosts)

	// Window: default to the next 14 days when omitted; validate otherwise.
	windowStart, windowEnd, err := resolveWindow(in.WindowStart, in.WindowEnd, now)
	if err != nil {
		return nil, err
	}
	// A lone post has nothing to spread across a 14-day window, so "generate 1
	// for Jul 22" must land ON Jul 22 — not the window's midpoint. When the model
	// gave only a start for a single post, collapse the derived range to that day
	// so validation pins the publish date exactly (CON-114).
	windowEnd = singlePostWindowEnd(windowStart, windowEnd, in.WindowEnd, count)

	emit(st.onEvent, SSEEventGeneratePostsStarted, GeneratePostsStartedEventPayload{
		PlatformIDs: platformIDs,
		PhaseID:     phaseID,
		Count:       count,
	})

	// Forward the engine's nested events, namespaced.
	nested := content_plan.OnEventFunc(func(name content_plan.SSEEventKind, data any) {
		switch name {
		case content_plan.SSEEventStep:
			emit(st.onEvent, SSEEventGeneratePostsStep, data)
		case content_plan.SSEEventPost:
			emit(st.onEvent, SSEEventGeneratePostsPost, data)
		case content_plan.SSEEventWarning:
			emit(st.onEvent, SSEEventGeneratePostsWarning, data)
		}
	})

	resp, err := st.generatePosts(ctx, content_plan.GeneratePostsRequest{
		CampaignID:  st.campaignID,
		PlatformIDs: platformIDs,
		PhaseID:     phaseID,
		Count:       count,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		PostType:    in.PostType,
	}, nested)
	if err != nil {
		return nil, err
	}

	// CON-118: report which attached assets informed the generated posts.
	used := toAssetRefs(resp.UsedAssets)
	if len(used) > 0 {
		emit(st.onEvent, SSEEventAssetsUsed, AssetsUsedEventPayload{Assets: used})
	}

	st.generatedPostsResult = &GeneratedPostsResult{
		PostCount:   len(resp.Posts),
		PlatformIDs: platformIDs,
		PhaseID:     phaseID,
		Warnings:    resp.Warnings,
		UsedAssets:  used,
	}
	return &GeneratePostsOutput{
		PostCount:      len(resp.Posts),
		RequestedCount: requested,
		Clamped:        clamped,
		PhaseID:        phaseID,
		PhaseName:      phaseName,
		Platforms:      platformNames,
		Dates:          publishDatesOf(resp.Posts),
		Warnings:       resp.Warnings,
		UsedAssets:     used,
	}, nil
}

// resolveGenerateCount maps the model-supplied count to the number of posts the
// generatePosts tool will actually create. An explicit positive count is honored
// exactly. A missing or non-positive count defaults to 1 — the safe minimum, so
// a planner that omits count (as Haiku does for "generate 1 post") can never
// over-produce; the model is instead told to pass 3 for a vague "a few". Anything
// above the per-call cap (maxN, default 10) is clamped down. Returns the
// effective count, the requested count after the default is applied (surfaced as
// RequestedCount), and whether the request was clamped.
func resolveGenerateCount(requested, maxN int) (count, requestedOut int, clamped bool) {
	if maxN <= 0 {
		maxN = 10
	}
	if requested <= 0 {
		requested = 1
	}
	count = requested
	if count > maxN {
		count = maxN
		clamped = true
	}
	return count, requested, clamped
}

// singlePostWindowEnd collapses a derived date range to a single day when the
// tool is creating exactly one post and the user gave no explicit end. A lone
// post has nothing to spread across a window, so "generate 1 for Jul 22" must
// land on Jul 22 rather than the midpoint of resolveWindow's 14-day default. An
// explicit end (rawEnd != "") or a multi-post request keeps the resolved end.
func singlePostWindowEnd(resolvedStart, resolvedEnd, rawEnd string, count int) string {
	if count == 1 && rawEnd == "" {
		return resolvedStart
	}
	return resolvedEnd
}

// publishDatesOf extracts the actual publish dates of the created posts, so the
// model reports the real dates in its reply instead of inventing them (CON-114).
func publishDatesOf(posts []content_plan.DraftPost) []string {
	out := make([]string, 0, len(posts))
	for _, p := range posts {
		if p.PublishDate != "" {
			out = append(out, p.PublishDate)
		}
	}
	return out
}

// minAskAssetsSimilarity is the cosine-similarity floor for asset Q&A. Lower
// than content_plan's generation threshold: a specific question benefits from
// recall, and the planner filters the returned excerpts (CON-118).
const minAskAssetsSimilarity = 0.5

// askAssetsChunkLimit caps how many chunks the Q&A tool returns to the planner.
const askAssetsChunkLimit = 8

// toolAskCampaignAssets answers a question grounded in the campaign's attached
// assets by embedding the query and searching the assets' chunks (CON-118). It
// is read-only and degrades cleanly (available:false / a note) rather than
// failing the turn when the embedder is unavailable or nothing matches.
func toolAskCampaignAssets(ctx context.Context, in AskCampaignAssetsInput) (*AskCampaignAssetsOutput, error) {
	st := getRequestState(ctx)
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return nil, fmt.Errorf("ask a question about the campaign's assets")
	}
	if !embedopts.Available(st.embedder) || st.repos.Chunks == nil || st.repos.Assets == nil {
		return &AskCampaignAssetsOutput{Available: false, Note: "asset search is unavailable right now"}, nil
	}

	ids, err := readyCampaignAssetIDs(ctx, st.campaign, st.repos.Assets)
	if err != nil {
		return nil, fmt.Errorf("resolve campaign assets: %w", err)
	}
	if len(ids) == 0 {
		return &AskCampaignAssetsOutput{Available: true, Note: "this campaign has no ready assets to search"}, nil
	}

	qResp, err := st.embedder.Embed(ctx, &ai.EmbedRequest{
		Input:   []*ai.Document{ai.DocumentFromText(query, nil)},
		Options: embedopts.Query(),
	})
	if err != nil || len(qResp.Embeddings) == 0 {
		return &AskCampaignAssetsOutput{Available: false, Note: "asset search is unavailable right now"}, nil
	}

	chunks, err := st.repos.Chunks.SearchSimilar(ctx, pgvector.NewHalfVector(qResp.Embeddings[0].Embedding), ids, minAskAssetsSimilarity, askAssetsChunkLimit)
	if err != nil {
		return nil, fmt.Errorf("search assets: %w", err)
	}
	if len(chunks) == 0 {
		return &AskCampaignAssetsOutput{Available: true, Note: "no attached asset content matched the question"}, nil
	}

	titles := make(map[string]string)
	excerpts := make([]AssetExcerpt, 0, len(chunks))
	for _, c := range chunks {
		title, ok := titles[c.AssetID]
		if !ok {
			if a, err := st.repos.Assets.GetByID(ctx, c.AssetID); err == nil {
				title = a.Title
			}
			titles[c.AssetID] = title
		}
		excerpts = append(excerpts, AssetExcerpt{
			AssetID: c.AssetID,
			Title:   title,
			Pages:   pageRef(c.PageStart, c.PageEnd),
			Text:    c.Content,
		})
	}
	return &AskCampaignAssetsOutput{Excerpts: excerpts, Available: true}, nil
}

// readyCampaignAssetIDs resolves the campaign's attached, ready asset IDs — the
// explicit AssetIDs list when set, otherwise all tenant-ready assets — excluding
// failed/partial. Mirrors content_plan's candidate resolution (CON-118). It does
// NOT require campaign.UseAssets: that flag governs automatic inclusion during
// generation, whereas Q&A is an explicit request to consult the assets.
func readyCampaignAssetIDs(ctx context.Context, campaign *models.Campaign, assets repository.AssetRepository) ([]string, error) {
	bad := func(status string) bool {
		return status == models.AssetStatusFailed || status == models.AssetStatusPartial
	}
	if len(campaign.AssetIDs) > 0 {
		out := make([]string, 0, len(campaign.AssetIDs))
		for _, id := range campaign.AssetIDs {
			a, err := assets.GetByID(ctx, id)
			if err != nil || bad(a.Status) {
				continue
			}
			out = append(out, a.ID)
		}
		return out, nil
	}
	all, err := assets.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(all))
	for _, a := range all {
		if !bad(a.Status) {
			out = append(out, a.ID)
		}
	}
	return out, nil
}

// ensureCampaignAssetUse turns on asset-sourced content generation when the
// campaign has ready attached assets but UseAssets is still off, and persists
// the flag so it sticks for later turns and the UI (CON-118). content_plan
// injects the attached assets into the generation prompt whenever UseAssets is
// true, so flipping it here is all that's needed. Best-effort: if the flag
// can't be persisted, generation just proceeds without assets, as before.
func ensureCampaignAssetUse(ctx context.Context, st *requestState) {
	if st.campaign.UseAssets || st.repos.Assets == nil || st.repos.Campaigns == nil {
		return
	}
	if len(st.campaign.AssetIDs) == 0 {
		return // no assets attached to this campaign
	}
	ids, err := readyCampaignAssetIDs(ctx, st.campaign, st.repos.Assets)
	if err != nil || len(ids) == 0 {
		return // nothing ready to use
	}
	st.campaign.UseAssets = true
	if err := st.repos.Campaigns.Update(ctx, st.campaign); err != nil {
		st.campaign.UseAssets = false // keep in-memory state consistent with the DB
		slog.WarnContext(ctx, "could not enable asset use for generation",
			logging.AttrComponent, "genkit.campaign_assistant",
			"campaign_id", st.campaignID, logging.AttrError, err)
	}
}

// pageRef renders an asset chunk's page span for citation (CON-118).
func pageRef(start, end *int) string {
	if start == nil {
		return ""
	}
	if end == nil || *end == *start {
		return fmt.Sprintf("p. %d", *start)
	}
	return fmt.Sprintf("pp. %d-%d", *start, *end)
}

// resolveTargetPlatforms maps requested platform names/ids to campaign-target
// platform ids. It errors (naming the targets) when a requested platform isn't
// one the campaign already targets — no silent scope expansion.
func resolveTargetPlatforms(campaign *models.Campaign, requested []string) (ids, names []string, err error) {
	if len(requested) == 0 {
		return nil, nil, fmt.Errorf("say which platform to add posts for")
	}
	byID := make(map[string]models.Platform, len(campaign.Platforms))
	byName := make(map[string]models.Platform, len(campaign.Platforms))
	targetNames := make([]string, 0, len(campaign.Platforms))
	for _, p := range campaign.Platforms {
		byID[p.ID] = p
		byName[strings.ToLower(p.Name)] = p
		targetNames = append(targetNames, p.Name)
	}
	seen := make(map[string]bool)
	for _, r := range requested {
		p, ok := byID[r]
		if !ok {
			p, ok = byName[strings.ToLower(strings.TrimSpace(r))]
		}
		if !ok {
			return nil, nil, fmt.Errorf("%q is not one of this campaign's target platforms (%s) — add it to the campaign first, or pick a targeted one", r, strings.Join(targetNames, ", "))
		}
		if seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		ids = append(ids, p.ID)
		names = append(names, p.Name)
	}
	return ids, names, nil
}

func campaignPhases(campaign *models.Campaign) []models.CampaignTypePhase {
	if campaign.CampaignType == nil {
		return nil
	}
	return campaign.CampaignType.Phases
}

// resolvePhase resolves "current"/empty to the timeline-current phase; otherwise
// matches by id or name.
func resolvePhase(campaign *models.Campaign, phase string, today time.Time) (id, name string, err error) {
	phases := campaignPhases(campaign)
	if len(phases) == 0 {
		return "", "", fmt.Errorf("this campaign's type has no phases")
	}
	p := strings.TrimSpace(phase)
	if p == "" || strings.EqualFold(p, "current") {
		cur := currentPhase(campaign, today)
		return cur.ID, cur.Name, nil
	}
	for _, ph := range phases {
		if ph.ID == p || strings.EqualFold(ph.Name, p) {
			return ph.ID, ph.Name, nil
		}
	}
	pnames := make([]string, 0, len(phases))
	for _, ph := range phases {
		pnames = append(pnames, ph.Name)
	}
	return "", "", fmt.Errorf("unknown phase %q; the campaign's phases are: %s", phase, strings.Join(pnames, ", "))
}

// currentPhase returns the phase whose timeline window contains today, by
// partitioning [StartDate, EndDate] across phases in sequence order. Falls back
// to the first phase when the campaign has no dates.
func currentPhase(campaign *models.Campaign, today time.Time) models.CampaignTypePhase {
	phases := append([]models.CampaignTypePhase(nil), campaignPhases(campaign)...)
	sort.SliceStable(phases, func(i, j int) bool { return phases[i].Sequence < phases[j].Sequence })
	if campaign.StartDate == nil || campaign.EndDate == nil {
		return phases[0]
	}
	start, end := *campaign.StartDate, *campaign.EndDate
	if !today.After(start) {
		return phases[0]
	}
	if today.After(end) {
		return phases[len(phases)-1]
	}
	windows := equalDayWindows(start, end, len(phases))
	for i, w := range windows {
		if !today.Before(w[0]) && !today.After(w[1]) {
			return phases[i]
		}
	}
	return phases[len(phases)-1]
}

// equalDayWindows partitions [start, end] into n contiguous inclusive day
// windows (remainder to earliest), mirroring content_plan's date partition.
func equalDayWindows(start, end time.Time, n int) [][2]time.Time {
	out := make([][2]time.Time, n)
	totalDays := int(end.Sub(start).Hours()/24) + 1
	if totalDays < n {
		for i := range out {
			out[i] = [2]time.Time{start, end}
		}
		return out
	}
	base := totalDays / n
	rem := totalDays % n
	cursor := start
	for i := 0; i < n; i++ {
		d := base
		if i < rem {
			d++
		}
		winEnd := cursor.AddDate(0, 0, d-1)
		out[i] = [2]time.Time{cursor, winEnd}
		cursor = winEnd.AddDate(0, 0, 1)
	}
	return out
}

// resolveWindow validates the model-resolved publish window and fills in a
// missing bound rather than failing: the planner often resolves only one side
// of a vague timeframe ("upcoming weeks" → a start, no end). A non-empty but
// malformed bound is still rejected. It defaults to a 14-day span and clamps a
// past start to today so drafts are never dated in the past.
func resolveWindow(startStr, endStr string, today time.Time) (string, string, error) {
	const iso = "2006-01-02"
	const spanDays = 14
	todayDate, _ := time.Parse(iso, today.Format(iso))
	startStr, endStr = strings.TrimSpace(startStr), strings.TrimSpace(endStr)

	var s, e time.Time
	haveStart, haveEnd := startStr != "", endStr != ""
	if haveStart {
		t, err := time.Parse(iso, startStr)
		if err != nil {
			return "", "", fmt.Errorf("windowStart must be an ISO date (YYYY-MM-DD)")
		}
		s = t
	}
	if haveEnd {
		t, err := time.Parse(iso, endStr)
		if err != nil {
			return "", "", fmt.Errorf("windowEnd must be an ISO date (YYYY-MM-DD)")
		}
		e = t
	}

	// Derive whichever bound the model left out.
	switch {
	case !haveStart && !haveEnd:
		s, e = todayDate, todayDate.AddDate(0, 0, spanDays)
	case haveStart && !haveEnd:
		e = s.AddDate(0, 0, spanDays)
	case !haveStart && haveEnd:
		s = todayDate
	}

	if e.Before(s) {
		return "", "", fmt.Errorf("the timeframe's end is before its start")
	}
	// Reject an explicitly-requested past date rather than silently clamping it
	// to today (CON-114). Derived bounds default to today, so only a user-supplied
	// start/end can be in the past here; the planner is told to catch this first
	// and reply conversationally, and this is the backstop.
	if haveStart && s.Before(todayDate) {
		return "", "", fmt.Errorf("%s is in the past — choose %s (today) or a later date", startStr, todayDate.Format(iso))
	}
	if haveEnd && e.Before(todayDate) {
		return "", "", fmt.Errorf("%s is in the past — choose %s (today) or a later date", endStr, todayDate.Format(iso))
	}
	return s.Format(iso), e.Format(iso), nil
}

func toolSetCampaignDates(ctx context.Context, in SetCampaignDatesInput) (*SetCampaignDatesOutput, error) {
	st := getRequestState(ctx)
	c := st.campaign
	const iso = "2006-01-02"

	if strings.TrimSpace(in.StartDate) == "" && strings.TrimSpace(in.EndDate) == "" {
		return nil, fmt.Errorf("give a new start or end date")
	}
	start, end := c.StartDate, c.EndDate
	if s := strings.TrimSpace(in.StartDate); s != "" {
		t, err := time.Parse(iso, s)
		if err != nil {
			return nil, fmt.Errorf("startDate must be an ISO date (YYYY-MM-DD)")
		}
		start = &t
	}
	if e := strings.TrimSpace(in.EndDate); e != "" {
		t, err := time.Parse(iso, e)
		if err != nil {
			return nil, fmt.Errorf("endDate must be an ISO date (YYYY-MM-DD)")
		}
		end = &t
	}
	if start == nil || end == nil {
		return nil, fmt.Errorf("the campaign needs both a start and an end date — please provide both")
	}
	if !start.Before(*end) {
		return nil, fmt.Errorf("the start date must be before the end date")
	}
	if end.Sub(*start) < 24*time.Hour {
		return nil, fmt.Errorf("the campaign must span at least one day")
	}

	// Count eligible (draft/ready) posts that will fall outside the new range.
	// Load before persisting the date change so a lookup failure aborts cleanly
	// instead of saving the dates and silently reporting zero out-of-range posts.
	posts, err := st.repos.Posts.ListByCampaign(ctx, st.campaignID)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	lo, hi := dateOnly(*start), dateOnly(*end)
	outside := 0
	for _, p := range posts {
		if reschedule.Eligible(p.Status) && p.ScheduledAt != nil {
			d := dateOnly(*p.ScheduledAt)
			if d.Before(lo) || d.After(hi) {
				outside++
			}
		}
	}

	c.StartDate = start
	c.EndDate = end
	c.UpdatedAt = time.Now().UTC()
	if err := st.repos.Campaigns.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("update campaign dates: %w", err)
	}

	st.datesResult = &DatesResult{StartDate: start.Format(iso), EndDate: end.Format(iso), PostsOutsideRange: outside}
	return &SetCampaignDatesOutput{StartDate: start.Format(iso), EndDate: end.Format(iso), PostsOutsideRange: outside}, nil
}

func toolRedistributePosts(ctx context.Context) (*RedistributePostsOutput, error) {
	st := getRequestState(ctx)
	c := st.campaign
	if c.StartDate == nil || c.EndDate == nil {
		return nil, fmt.Errorf("set the campaign's start and end dates first, then I can redistribute the posts")
	}

	posts, err := st.repos.Posts.ListByCampaign(ctx, st.campaignID)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}

	byID := make(map[string]*models.Post, len(posts))
	for i := range posts {
		byID[posts[i].ID] = &posts[i]
	}

	phasesTouched := make(map[string]bool)
	var changed []*models.Post
	for _, a := range reschedule.Plan(c, posts) {
		p := byID[a.PostID]
		if p == nil {
			continue
		}
		if p.ScheduledAt != nil && dateOnly(*p.ScheduledAt).Equal(a.ScheduledAt) {
			continue // already on the target date
		}
		at := a.ScheduledAt
		p.ScheduledAt = &at
		p.UpdatedAt = time.Now().UTC()
		changed = append(changed, p)
		key := ""
		if p.CampaignTypePhaseID != nil {
			key = *p.CampaignTypePhaseID
		}
		phasesTouched[key] = true
	}

	if len(changed) > 0 {
		if err := st.repos.Posts.UpdateScheduledAtBatch(ctx, changed); err != nil {
			return nil, fmt.Errorf("persist redistributed posts: %w", err)
		}
	}

	st.redistributeResult = &RedistributeResult{PostsUpdated: len(changed), PhaseCount: len(phasesTouched)}
	return &RedistributePostsOutput{PostsUpdated: len(changed), PhaseCount: len(phasesTouched)}, nil
}

// dateOnly truncates a time to its calendar date at 00:00 UTC.
func dateOnly(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

func toolCheckBrief(ctx context.Context) (*consistency.BriefReview, error) {
	st := getRequestState(ctx)
	if st.checkBrief == nil {
		return nil, fmt.Errorf("brief review is not available")
	}
	emit(st.onEvent, SSEEventCheckBriefStarted, struct{}{})
	nested := consistency.OnEventFunc(func(_ consistency.SSEEventKind, _ any) {}) // step events not surfaced individually
	review, err := st.checkBrief(ctx, st.campaignID, nested)
	if err != nil {
		return nil, err
	}
	st.briefReviewResult = review
	return review, nil
}

func toolCheckPostsConsistency(ctx context.Context, in CheckPostsInput) (*consistency.PostsReview, error) {
	st := getRequestState(ctx)
	if st.checkPosts == nil {
		return nil, fmt.Errorf("posts review is not available")
	}
	emit(st.onEvent, SSEEventCheckPostsStarted, struct{}{})
	nested := consistency.OnEventFunc(func(_ consistency.SSEEventKind, _ any) {})
	review, err := st.checkPosts(ctx, consistency.PostsCheckRequest{CampaignID: st.campaignID, Max: in.Max}, nested)
	if err != nil {
		return nil, err
	}
	st.postsReviewResult = review
	return review, nil
}
