package post_assistant

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/ogen-app/ogen/src/models"
)

func runPostAssistant(
	ctx context.Context,
	g *genkit.Genkit,
	req PostAssistantRequest,
	cfg PostAssistantFlowConfig,
	repos PostAssistantRepos,
	systemTmpl, contextTmpl *template.Template,
	tools *toolSet,
	onEvent OnEventFunc,
) (out *PostAssistantResponse, retErr error) {
	start := time.Now()
	log.Printf("post_assistant[%s]: starting instruction=%.80s", req.PostID, req.Instruction)

	// finaliseOwnerID is captured once the post is loaded so the deferred
	// finalisation event can be scoped to the post owner. Empty before
	// load → finalisation events for very-early failures are skipped.
	var finaliseOwnerID string

	defer func() {
		if cfg.Hub == nil || finaliseOwnerID == "" {
			return
		}
		publishAssistantFinalised(cfg.Hub, req.PostID, finaliseOwnerID, out, retErr)
	}()

	if req.Instruction == "" {
		return nil, &ValidationError{Msg: "instruction is required"}
	}

	// ── Load post ────────────────────────────────────────────────────────────
	post, err := repos.Posts.GetByID(ctx, req.PostID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &ValidationError{Msg: "post not found"}
		}
		return nil, fmt.Errorf("load post: %w", err)
	}
	finaliseOwnerID = post.CreatedBy

	// ── Ensure initial version ───────────────────────────────────────────────
	count, err := repos.Versions.CountByPostID(ctx, req.PostID)
	if err != nil {
		return nil, fmt.Errorf("count versions: %w", err)
	}
	if count == 0 && post.Content != "" {
		id, err := models.NewID()
		if err != nil {
			return nil, err
		}
		if err := repos.Versions.Create(ctx, &models.PostVersion{
			ID:            id,
			PostID:        req.PostID,
			VersionNumber: 1,
			Content:       post.Content,
			Note:          "Initial version",
			Creator:       "user",
		}); err != nil {
			return nil, fmt.Errorf("create initial version: %w", err)
		}
		log.Printf("post_assistant[%s]: created initial version snapshot", req.PostID)
	}

	// ── Assemble context + load history in parallel ─────────────────────────
	var actx *assistantContext
	var ctxErr error
	var history []*ai.Message
	var histErr error

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		actx, ctxErr = assembleContextCached(ctx, post, repos, systemTmpl, contextTmpl)
	}()
	go func() {
		defer wg.Done()
		msgs, err := repos.Messages.ListRecentByPostID(ctx, req.PostID, 10)
		if err != nil {
			histErr = err
			return
		}
		history = make([]*ai.Message, 0, len(msgs))
		for _, m := range msgs {
			switch m.Role {
			case "user":
				history = append(history, ai.NewUserTextMessage(m.Content))
			case "model":
				history = append(history, ai.NewModelTextMessage(m.Content))
			}
		}
	}()
	wg.Wait()

	if ctxErr != nil {
		return nil, fmt.Errorf("assemble context: %w", ctxErr)
	}
	if histErr != nil {
		return nil, fmt.Errorf("load history: %w", histErr)
	}

	// ── Inject per-request state for tools ───────────────────────────────────
	ctx = withRequestState(ctx, &requestState{
		postID:   req.PostID,
		assetIDs: post.UsedAssetIDs,
		repos:    repos,
		embedder: cfg.Embedder,
	})

	// ── Call model ───────────────────────────────────────────────────────────
	// Assistant responses are short (description + explanation), so cap
	// output tokens well below the content-plan default.
	maxTokens := cfg.MaxOutputTokens
	if maxTokens == 0 {
		maxTokens = 64000
	}
	maxTurns := cfg.MaxTurns
	if maxTurns == 0 {
		maxTurns = 8
	}

	modelName := "anthropic/" + cfg.ModelID

	// System + context block forms the stable cached prefix.
	systemBlock := actx.SystemPrompt + "\n\n" + actx.ContextBlock

	// Set up an incremental JSON scanner that watches the two string-valued
	// fields whose deltas we surface to the client. The scanner decodes
	// JSON escapes as they arrive, so the client never sees raw \n / \uXXXX.
	scanner := NewJSONStringScanner(
		[]string{"explanation", "updatedContent"},
		func(key, delta string) {
			switch key {
			case "explanation":
				emit(onEvent, SSEEventExplanationDelta, DeltaEventPayload{Delta: delta})
			case "updatedContent":
				emit(onEvent, SSEEventContentDelta, DeltaEventPayload{Delta: delta})
			}
		},
	)

	// Tool calls arrive in many partial streaming fragments as the model
	// builds the input JSON; we only surface the final complete request.
	// Tool responses are small and emitted once. Both are deduped by Ref
	// since genkit may replay the same part in later chunks.
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
				if tr == nil || tr.Partial {
					continue
				}
				if emittedToolCalls[tr.Ref] {
					continue
				}
				emittedToolCalls[tr.Ref] = true
				emit(onEvent, SSEEventToolCall, ToolCallEventPayload{
					Name:  tr.Name,
					Input: tr.Input,
					Ref:   tr.Ref,
				})
			case part.IsToolResponse():
				tr := part.ToolResponse
				if tr == nil || emittedToolResults[tr.Ref] {
					continue
				}
				emittedToolResults[tr.Ref] = true
				emit(onEvent, SSEEventToolResult, ToolResultEventPayload{
					Name: tr.Name,
					Ref:  tr.Ref,
					OK:   true,
				})
			}
		}
		return nil
	}

	// Use streaming mode — the Anthropic API requires it for requests
	// that may involve tool calls (which can exceed the 10-minute timeout
	// for non-streaming requests). The streaming callback fans chunks out
	// as SSE events for the UI.
	// NB: we deliberately do NOT pass ai.WithOutputType — genkit's
	// post-generation schema validator parses the raw text strictly and
	// returns (nil, err) on any blemish (trailing comma, stray char, etc.),
	// discarding the full response. Dropping the constraint lets us do the
	// parse ourselves with a tolerant preprocessor below. Format discipline
	// is enforced via the prompt, which is already explicit.
	resp, err := genkit.Generate(ctx, g,
		ai.WithModelName(modelName),
		ai.WithSystem(systemBlock),
		ai.WithMessages(history...),
		ai.WithPrompt(req.Instruction),
		ai.WithTools(tools.listAssets, tools.getAssetChunks, tools.searchAssetChunks, tools.getCurrentContent),
		ai.WithMaxTurns(maxTurns),
		ai.WithStreaming(streamCb),
		ai.WithConfig(anthropic.MessageNewParams{
			MaxTokens: maxTokens,
		}),
	)
	if err != nil {
		log.Printf("post_assistant[%s]: model call failed after %s: %v", req.PostID, time.Since(start).Round(time.Millisecond), err)
		return nil, &AIError{Msg: fmt.Sprintf("model call failed: %v", err)}
	}

	// Deterministic truncation signal: when Anthropic's stop_reason is
	// "max_tokens", genkit surfaces it as FinishReasonLength. Log loudly
	// so the cap can be tuned (env MAX_OUTPUT_TOKENS) before users see
	// the recovery branches kick in.
	if resp.FinishReason == ai.FinishReasonLength {
		var outputTokens int64
		if resp.Usage != nil {
			outputTokens = int64(resp.Usage.OutputTokens)
		}
		log.Printf("post_assistant[%s]: TRUNCATED — finish_reason=length, output_tokens=%d, cap=%d. Bump MAX_OUTPUT_TOKENS or shorten the post/instruction.",
			req.PostID, outputTokens, maxTokens)
	}

	if resp.Usage != nil {
		log.Printf("post_assistant[%s]: tokens — input=%d output=%d total=%d",
			req.PostID, resp.Usage.InputTokens, resp.Usage.OutputTokens,
			resp.Usage.InputTokens+resp.Usage.OutputTokens)
	}

	// ── Assemble response from scanner ───────────────────────────────────────
	// The scanner has been processing every chunk in the streaming callback
	// above. Its Values() method returns the parsed top-level fields —
	// strings decoded, literals coerced — without going through
	// encoding/json. This bypasses the whole class of Claude JSON-drift
	// bugs (trailing commas, missing separators, preamble prose, literal
	// newlines inside strings, truncation) that would otherwise hard-fail
	// the final Unmarshal. See scanner_test.go TestValues_* for coverage.
	vals := scanner.Values()
	result := PostAssistantResponse{}
	if s, ok := vals["explanation"].(string); ok {
		result.Explanation = s
	}
	if s, ok := vals["updatedContent"].(string); ok {
		result.UpdatedContent = s
	}
	if s, ok := vals["action"].(string); ok {
		result.Action = s
	}
	if b, ok := vals["saveVersion"].(bool); ok {
		result.SaveVersion = b
	}
	if s, ok := vals["versionNote"].(string); ok {
		result.VersionNote = s
	}

	// Pure-prose recovery: occasionally the model ignores the JSON
	// envelope entirely and answers in plain prose (often when the user
	// asks an informational question). Salvage the raw text as the
	// explanation of a "declined" response so the user at least sees
	// the answer in the chat bubble. The prompt is the proper fix —
	// this is the safety net for when the prompt fails to constrain.
	if result.Explanation == "" && result.UpdatedContent == "" {
		raw := strings.TrimSpace(scanner.FullText())
		if raw != "" && !strings.Contains(raw, "{") {
			log.Printf("post_assistant[%s]: model emitted prose-only response (len=%d) — treating as informational/declined", req.PostID, len(raw))
			result.Explanation = raw
			result.Action = "declined"
		}
	}

	// Graceful degradation against truncated responses (max_tokens hit
	// mid-content). Field order in the prompt is
	// explanation → updatedContent → action → saveVersion → versionNote,
	// so when the model runs out of tokens during updatedContent the
	// trailing metadata fields are the first to drop off. If we got
	// usable content, recover the missing fields from defaults instead
	// of failing the whole turn.
	switch {
	case result.Explanation == "" && result.UpdatedContent == "":
		// Genuinely unusable — neither field came through.
		raw := scanner.FullText()
		log.Printf("post_assistant[%s]: scanner found no usable fields (len=%d): %.500s",
			req.PostID, len(raw), raw)
		return nil, &AIError{Msg: "model response did not contain the expected fields"}

	case result.Action == "" && result.UpdatedContent != "":
		// The model wouldn't have emitted updatedContent if it had
		// decided to decline; infer "edited".
		log.Printf("post_assistant[%s]: action missing — inferring 'edited' from non-empty updatedContent (likely max_tokens truncation)", req.PostID)
		result.Action = "edited"
	}

	// Surface a generic explanation if the model got truncated before
	// it could write one.
	if result.Explanation == "" && result.UpdatedContent != "" {
		result.Explanation = "Updated post content."
	}

	// Content is persisted and returned as Markdown. The frontend is the
	// only layer that converts to/from BlockNote JSON for editor rendering.

	// ── Persist conversation turn ────────────────────────────────────────────
	userMsgID, err := models.NewID()
	if err != nil {
		return nil, err
	}
	if err := repos.Messages.Create(ctx, &models.PostAssistantMessage{
		ID:      userMsgID,
		PostID:  req.PostID,
		Role:    "user",
		Content: req.Instruction,
	}); err != nil {
		return nil, fmt.Errorf("persist user message: %w", err)
	}

	// Persist the model turn as the same JSON shape the assistant emits —
	// minus `updatedContent`, which is bulky and would bloat history on
	// subsequent turns. Storing JSON lets the UI reload the action /
	// saveVersion / versionNote badges on page refresh without a round-trip
	// through a custom parse, and gives the model its own prior response
	// back in its native output format.
	modelMsgID, err := models.NewID()
	if err != nil {
		return nil, err
	}
	historyJSON, err := json.Marshal(struct {
		Action      string `json:"action"`
		Explanation string `json:"explanation"`
		SaveVersion bool   `json:"saveVersion"`
		VersionNote string `json:"versionNote,omitempty"`
	}{
		Action:      result.Action,
		Explanation: result.Explanation,
		SaveVersion: result.SaveVersion,
		VersionNote: result.VersionNote,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal model history: %w", err)
	}
	if err := repos.Messages.Create(ctx, &models.PostAssistantMessage{
		ID:      modelMsgID,
		PostID:  req.PostID,
		Role:    "model",
		Content: string(historyJSON),
	}); err != nil {
		return nil, fmt.Errorf("persist model message: %w", err)
	}

	// ── Handle versioning ────────────────────────────────────────────────────
	if result.SaveVersion && result.Action == "edited" {
		latest, err := repos.Versions.GetLatestByPostID(ctx, req.PostID)
		if err != nil {
			return nil, fmt.Errorf("get latest version: %w", err)
		}
		nextNum := 1
		if latest != nil {
			nextNum = latest.VersionNumber + 1
		}

		versionID, err := models.NewID()
		if err != nil {
			return nil, err
		}
		if err := repos.Versions.Create(ctx, &models.PostVersion{
			ID:            versionID,
			PostID:        req.PostID,
			VersionNumber: nextNum,
			Content:       result.UpdatedContent,
			Note:          result.VersionNote,
			Creator:       "assistant",
		}); err != nil {
			return nil, fmt.Errorf("create version: %w", err)
		}
		log.Printf("post_assistant[%s]: created version %d — %s", req.PostID, nextNum, result.VersionNote)
	}

	// ── Update post content ──────────────────────────────────────────────────
	if result.Action == "edited" {
		post.Content = result.UpdatedContent
		post.UpdatedAt = time.Now().UTC()
		if err := repos.Posts.Update(ctx, post); err != nil {
			return nil, fmt.Errorf("update post: %w", err)
		}
	}

	log.Printf("post_assistant[%s]: done in %s action=%s saveVersion=%v",
		req.PostID, time.Since(start).Round(time.Millisecond), result.Action, result.SaveVersion)

	// Emit the final structured response. The client treats this as the
	// canonical result; delta events before this are preview-only.
	emit(onEvent, SSEEventComplete, &result)

	return &result, nil
}
