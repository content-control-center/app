package campaign_assistant

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/vendors/llm"
)

func runCampaignAssistant(
	ctx context.Context,
	g *genkit.Genkit,
	req CampaignAssistantRequest,
	cfg CampaignAssistantFlowConfig,
	repos CampaignAssistantRepos,
	systemTmpl, contextTmpl *template.Template,
	tools *toolSet,
	onEvent OnEventFunc,
) (out *CampaignAssistantResponse, retErr error) {
	start := time.Now()
	slog.InfoContext(ctx, "starting", logging.AttrComponent, "genkit.campaign_assistant", "campaign_id", req.CampaignID, "instruction_len", len(req.Instruction))

	// Enforcement gate (CON-86 FR9): block before any provider call when the
	// tenant is already over a cap in enforce mode. Nil checker = no gate.
	if err := cfg.Checker.Enforce(ctx); err != nil {
		return nil, err
	}

	// finaliseOwnerID is captured once the campaign is loaded so the deferred
	// finalisation event is scoped to the campaign owner. Empty before load →
	// finalisation events for very-early failures are skipped.
	var finaliseOwnerID string
	defer func() {
		if cfg.Hub == nil || finaliseOwnerID == "" {
			return
		}
		publishAssistantFinalised(cfg.Hub, req.CampaignID, finaliseOwnerID, out, retErr)
	}()

	if strings.TrimSpace(req.Instruction) == "" {
		return nil, &ValidationError{Msg: "instruction is required"}
	}

	// ── Load campaign (tenant-scoped) ────────────────────────────────────────
	// GetByID runs under the request tenant, so a campaign from another tenant
	// reads as not-found — the assistant never crosses a tenant boundary.
	campaign, err := repos.Campaigns.GetByID(ctx, req.CampaignID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &ValidationError{Msg: "campaign not found"}
		}
		return nil, fmt.Errorf("load campaign: %w", err)
	}
	finaliseOwnerID = campaign.CreatedBy

	// ── Assemble context + load history ──────────────────────────────────────
	actx, err := assembleContext(campaign, systemTmpl, contextTmpl)
	if err != nil {
		return nil, fmt.Errorf("assemble context: %w", err)
	}

	msgs, err := repos.Messages.ListRecentByCampaignID(ctx, req.CampaignID, 10)
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	history := make([]*ai.Message, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case "user":
			history = append(history, ai.NewUserTextMessage(m.Content))
		case "model":
			history = append(history, ai.NewModelTextMessage(m.Content))
		}
	}

	// ── Inject per-request state for tools ───────────────────────────────────
	st := &requestState{
		campaignID:  req.CampaignID,
		campaign:    campaign,
		repos:       repos,
		onEvent:     onEvent,
		contentPlan: cfg.ContentPlan,
		enrichBrief: cfg.EnrichBrief,
	}
	ctx = withRequestState(ctx, st)

	// ── Call model (planner on the cheap RolePlanning model) ─────────────────
	maxTokens := cfg.MaxOutputTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}
	maxTurns := cfg.MaxTurns
	if maxTurns == 0 {
		maxTurns = 4
	}
	modelName := cfg.Provider.Ref(llm.RolePlanning)
	systemBlock := actx.SystemPrompt + "\n\n" + actx.ContextBlock

	// Stream the conversational reply. Only "explanation" is surfaced as a
	// delta; the scanner decodes JSON escapes as they arrive.
	scanner := NewJSONStringScanner(
		[]string{"explanation"},
		func(key, delta string) {
			if key == "explanation" {
				emit(onEvent, SSEEventExplanationDelta, DeltaEventPayload{Delta: delta})
			}
		},
	)

	emittedToolCalls := map[string]bool{}
	emittedToolResults := map[string]bool{}

	streamCb := func(_ context.Context, chunk *ai.ModelResponseChunk) error {
		if chunk == nil || chunk.Aggregated {
			return nil
		}
		for _, part := range chunk.Content {
			switch {
			case part.IsText():
				scanner.Push(part.Text)
			case part.IsToolRequest():
				tr := part.ToolRequest
				if tr == nil || tr.Partial || emittedToolCalls[tr.Ref] {
					continue
				}
				emittedToolCalls[tr.Ref] = true
				emit(onEvent, SSEEventToolCall, ToolCallEventPayload{Name: tr.Name, Input: tr.Input, Ref: tr.Ref})
			case part.IsToolResponse():
				tr := part.ToolResponse
				if tr == nil || emittedToolResults[tr.Ref] {
					continue
				}
				emittedToolResults[tr.Ref] = true
				emit(onEvent, SSEEventToolResult, ToolResultEventPayload{Name: tr.Name, Ref: tr.Ref, OK: true})
			}
		}
		return nil
	}

	// No ai.WithOutputType — genkit's strict validator drops the whole response
	// on common Claude JSON drift. We parse via the tolerant scanner below.
	resp, err := genkit.Generate(ctx, g,
		ai.WithModelName(modelName),
		ai.WithSystem(systemBlock),
		ai.WithMessages(history...),
		ai.WithPrompt(req.Instruction),
		ai.WithTools(tools.runContentPlan, tools.enrichBrief, tools.listCampaignPosts),
		ai.WithMaxTurns(maxTurns),
		ai.WithStreaming(streamCb),
		cfg.Provider.CallConfig(maxTokens),
	)
	if err != nil {
		slog.ErrorContext(ctx, "model call failed", logging.AttrComponent, "genkit.campaign_assistant", "campaign_id", req.CampaignID, "duration_ms", time.Since(start).Milliseconds(), logging.AttrError, err)
		return nil, &AIError{Msg: fmt.Sprintf("model call failed: %v", err)}
	}

	if resp.FinishReason == ai.FinishReasonLength {
		var outputTokens int64
		if resp.Usage != nil {
			outputTokens = int64(resp.Usage.OutputTokens)
		}
		slog.WarnContext(ctx, "response truncated at max tokens", logging.AttrComponent, "genkit.campaign_assistant", "campaign_id", req.CampaignID, "output_tokens", outputTokens, "cap", maxTokens)
	}
	if resp.Usage != nil {
		slog.InfoContext(ctx, "tokens", logging.AttrComponent, "genkit.campaign_assistant", "campaign_id", req.CampaignID, "input", resp.Usage.InputTokens, "output", resp.Usage.OutputTokens, "total", resp.Usage.InputTokens+resp.Usage.OutputTokens)
	}
	// Record the planner's usage under this flow name; the sub-flows record
	// their own under content_plan / enrich_brief, so there's no double count.
	cfg.Recorder.RecordResp(ctx, cfg.Provider.Vendor(), cfg.Provider.Model(llm.RolePlanning), "campaign_assistant", resp)

	// ── Assemble response from scanner ───────────────────────────────────────
	vals := scanner.Values()
	result := CampaignAssistantResponse{}
	if s, ok := vals["explanation"].(string); ok {
		result.Explanation = s
	}
	if s, ok := vals["action"].(string); ok {
		result.Action = s
	}

	// Tool outcomes are authoritative — a terse model reply can't mask a
	// content plan or a brief that actually ran.
	if st.contentPlanResult != nil {
		result.Action = "content_plan_generated"
		result.ContentPlan = st.contentPlanResult
		if result.Explanation == "" {
			if st.contentPlanResult.PostCount == 0 {
				result.Explanation = "I couldn't generate any posts for this campaign."
			} else {
				result.Explanation = fmt.Sprintf("I generated a content plan with %d draft post(s) for this campaign.", st.contentPlanResult.PostCount)
			}
		}
	}
	if st.briefResult != nil {
		result.Action = "brief_enriched"
		result.Brief = st.briefResult
		if result.Explanation == "" {
			result.Explanation = "I enriched the campaign brief and saved it to the campaign."
		}
	}

	// Pure-prose recovery: the model ignored the JSON envelope and answered in
	// plain prose (common for informational questions) and no tool ran. Salvage
	// the raw text as an "answered" reply.
	if result.Explanation == "" && st.contentPlanResult == nil && st.briefResult == nil {
		raw := strings.TrimSpace(scanner.FullText())
		if raw != "" && !strings.Contains(raw, "{") {
			slog.WarnContext(ctx, "model emitted prose-only response, treating as answered", logging.AttrComponent, "genkit.campaign_assistant", "campaign_id", req.CampaignID, "len", len(raw))
			result.Explanation = raw
			result.Action = "answered"
		}
	}

	// Genuinely unusable — nothing came through.
	if result.Explanation == "" {
		raw := scanner.FullText()
		slog.ErrorContext(ctx, "scanner found no usable fields", logging.AttrComponent, "genkit.campaign_assistant", "campaign_id", req.CampaignID, "len", len(raw), "raw_preview", logging.Preview(raw, 500))
		return nil, &AIError{Msg: "model response did not contain the expected fields"}
	}

	if result.Action == "" {
		result.Action = "answered"
	}

	// ── Persist conversation turn ────────────────────────────────────────────
	if err := persistTurn(ctx, repos, req, &result); err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "done", logging.AttrComponent, "genkit.campaign_assistant", "campaign_id", req.CampaignID, "duration_ms", time.Since(start).Milliseconds(), "action", result.Action)

	// Surface tool completions before the canonical "complete" so the UI can
	// refresh the post list / brief as soon as they land.
	if result.ContentPlan != nil {
		emit(onEvent, SSEEventContentPlanComplete, ContentPlanCompleteEventPayload{
			PostCount: result.ContentPlan.PostCount,
			Warnings:  result.ContentPlan.Warnings,
		})
	}
	if result.Brief != nil {
		emit(onEvent, SSEEventEnrichBriefComplete, EnrichBriefCompleteEventPayload{Applied: result.Brief.Applied})
	}

	// Emit the final structured response — the canonical result; deltas before
	// this are preview-only.
	emit(onEvent, SSEEventComplete, &result)

	return &result, nil
}

// persistTurn stores the user instruction verbatim and the model turn as a
// compact JSON envelope (no bulky generated content) so history stays small.
func persistTurn(ctx context.Context, repos CampaignAssistantRepos, req CampaignAssistantRequest, result *CampaignAssistantResponse) error {
	userMsgID, err := models.NewID()
	if err != nil {
		return err
	}
	if err := repos.Messages.Create(ctx, &models.CampaignAssistantMessage{
		ID:         userMsgID,
		CampaignID: req.CampaignID,
		Role:       "user",
		Content:    req.Instruction,
	}); err != nil {
		return fmt.Errorf("persist user message: %w", err)
	}

	postCount := 0
	if result.ContentPlan != nil {
		postCount = result.ContentPlan.PostCount
	}
	briefApplied := result.Brief != nil && result.Brief.Applied
	historyJSON, err := json.Marshal(struct {
		Action       string `json:"action"`
		Explanation  string `json:"explanation"`
		PostCount    int    `json:"postCount,omitempty"`
		BriefApplied bool   `json:"briefApplied,omitempty"`
	}{
		Action:       result.Action,
		Explanation:  result.Explanation,
		PostCount:    postCount,
		BriefApplied: briefApplied,
	})
	if err != nil {
		return fmt.Errorf("marshal model history: %w", err)
	}
	modelMsgID, err := models.NewID()
	if err != nil {
		return err
	}
	if err := repos.Messages.Create(ctx, &models.CampaignAssistantMessage{
		ID:         modelMsgID,
		CampaignID: req.CampaignID,
		Role:       "model",
		Content:    string(historyJSON),
	}); err != nil {
		return fmt.Errorf("persist model message: %w", err)
	}
	return nil
}
