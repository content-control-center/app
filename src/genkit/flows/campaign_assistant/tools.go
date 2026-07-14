package campaign_assistant

import (
	"context"
	"fmt"
	"sort"
	"strings"
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
	// generatePosts backs the generatePosts targeted-generation tool (CON-114);
	// maxGeneratePosts caps a single call.
	generatePosts    func(ctx context.Context, req content_plan.GeneratePostsRequest, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error)
	maxGeneratePosts int

	// Results set by tools, read by the runner after generation.
	contentPlanResult    *ContentPlanResult
	briefResult          *BriefResult
	generatedPostsResult *GeneratedPostsResult
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

// GeneratePostsInput is the input for the generatePosts tool (CON-114). The
// model resolves the timeframe against today's date (shown in context) into an
// ISO window before calling.
type GeneratePostsInput struct {
	Platforms   []string `json:"platforms"             jsonschema:"description=Platform names or ids to generate for, e.g. [\"Threads\"]. Must be platforms the campaign already targets."`
	Phase       string   `json:"phase,omitempty"       jsonschema:"description=Phase name, id, or \"current\"; omit for the current phase."`
	Count       int      `json:"count,omitempty"       jsonschema:"description=How many posts to add; omit to infer from the request (a few = 3)."`
	WindowStart string   `json:"windowStart,omitempty" jsonschema:"description=First publish date (ISO YYYY-MM-DD), resolved from the requested timeframe against today."`
	WindowEnd   string   `json:"windowEnd,omitempty"   jsonschema:"description=Last publish date (ISO YYYY-MM-DD)."`
	PostType    string   `json:"postType,omitempty"    jsonschema:"description=Optional post-type slug (e.g. text-post, article); omit for the platform default."`
}

// GeneratePostsOutput is returned to the model after targeted posts are created.
type GeneratePostsOutput struct {
	PostCount      int      `json:"postCount"`
	RequestedCount int      `json:"requestedCount"`
	Clamped        bool     `json:"clamped"` // true when requestedCount exceeded the per-call cap
	PhaseID        string   `json:"phaseId"`
	PhaseName      string   `json:"phaseName"`
	Platforms      []string `json:"platforms"` // resolved platform names
	Warnings       []string `json:"warnings,omitempty"`
}

// ── Tool registration ────────────────────────────────────────────────────────

type toolSet struct {
	runContentPlan      ai.ToolRef
	enrichBrief         ai.ToolRef
	listCampaignPosts   ai.ToolRef
	getCampaignOverview ai.ToolRef
	generatePosts       ai.ToolRef
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

	return &toolSet{
		runContentPlan:      runContentPlan,
		enrichBrief:         enrichBrief,
		listCampaignPosts:   listCampaignPosts,
		getCampaignOverview: getCampaignOverview,
		generatePosts:       generatePosts,
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

func toolGeneratePosts(ctx context.Context, in GeneratePostsInput) (*GeneratePostsOutput, error) {
	st := getRequestState(ctx)
	if st.generatePosts == nil {
		return nil, fmt.Errorf("targeted post generation is not available")
	}
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

	// Count: default 3 when vague, clamp to [1, cap].
	maxN := st.maxGeneratePosts
	if maxN <= 0 {
		maxN = 10
	}
	requested := in.Count
	if requested <= 0 {
		requested = 3
	}
	count := requested
	clamped := false
	if count > maxN {
		count = maxN
		clamped = true
	}

	// Window: default to the next 14 days when omitted; validate otherwise.
	windowStart, windowEnd, err := resolveWindow(in.WindowStart, in.WindowEnd, now)
	if err != nil {
		return nil, err
	}

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

	st.generatedPostsResult = &GeneratedPostsResult{
		PostCount:   len(resp.Posts),
		PlatformIDs: platformIDs,
		PhaseID:     phaseID,
		Warnings:    resp.Warnings,
	}
	return &GeneratePostsOutput{
		PostCount:      len(resp.Posts),
		RequestedCount: requested,
		Clamped:        clamped,
		PhaseID:        phaseID,
		PhaseName:      phaseName,
		Platforms:      platformNames,
		Warnings:       resp.Warnings,
	}, nil
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

// resolveWindow validates the model-resolved publish window, defaulting to the
// next 14 days when omitted and clamping a past start to today.
func resolveWindow(startStr, endStr string, today time.Time) (string, string, error) {
	const iso = "2006-01-02"
	todayStr := today.Format(iso)
	if strings.TrimSpace(startStr) == "" && strings.TrimSpace(endStr) == "" {
		return todayStr, today.AddDate(0, 0, 14).Format(iso), nil
	}
	s, err := time.Parse(iso, strings.TrimSpace(startStr))
	if err != nil {
		return "", "", fmt.Errorf("windowStart must be an ISO date (YYYY-MM-DD)")
	}
	e, err := time.Parse(iso, strings.TrimSpace(endStr))
	if err != nil {
		return "", "", fmt.Errorf("windowEnd must be an ISO date (YYYY-MM-DD)")
	}
	if e.Before(s) {
		return "", "", fmt.Errorf("the timeframe's end is before its start")
	}
	todayParsed, _ := time.Parse(iso, todayStr)
	if s.Before(todayParsed) { // never date drafts in the past
		s = todayParsed
		if e.Before(s) {
			e = s
		}
	}
	return s.Format(iso), e.Format(iso), nil
}
