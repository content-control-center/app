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

	"github.com/content-control-center/app/src/markdown"
	"github.com/content-control-center/app/src/models"
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
) (*PostAssistantResponse, error) {
	start := time.Now()
	log.Printf("post_assistant[%s]: starting instruction=%.80s", req.PostID, req.Instruction)

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
	if maxTokens == 0 || maxTokens > 8192 {
		maxTokens = 8192
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
	resp, err := genkit.Generate(ctx, g,
		ai.WithModelName(modelName),
		ai.WithSystem(systemBlock),
		ai.WithMessages(history...),
		ai.WithPrompt(req.Instruction),
		ai.WithTools(tools.listAssets, tools.getAssetChunks, tools.searchAssetChunks, tools.getCurrentContent),
		ai.WithMaxTurns(3),
		ai.WithOutputType(PostAssistantResponse{}),
		ai.WithStreaming(streamCb),
		ai.WithConfig(anthropic.MessageNewParams{
			MaxTokens: maxTokens,
		}),
	)
	if err != nil {
		log.Printf("post_assistant[%s]: model call failed after %s: %v", req.PostID, time.Since(start).Round(time.Millisecond), err)
		return nil, &AIError{Msg: fmt.Sprintf("model call failed: %v", err)}
	}

	if resp.Usage != nil {
		log.Printf("post_assistant[%s]: tokens — input=%d output=%d total=%d",
			req.PostID, resp.Usage.InputTokens, resp.Usage.OutputTokens,
			resp.Usage.InputTokens+resp.Usage.OutputTokens)
	}

	// ── Parse response ───────────────────────────────────────────────────────
	text := strings.TrimSpace(resp.Text())
	// Strip markdown code fences if present.
	if strings.HasPrefix(text, "```") {
		if i := strings.Index(text, "\n"); i >= 0 {
			text = text[i+1:]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
		text = strings.TrimSpace(text)
	}

	var result PostAssistantResponse
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, &AIError{Msg: fmt.Sprintf("failed to parse model response as JSON: %v\nraw: %.300s", err, text)}
	}

	// Convert Markdown content to BlockNote JSON for storage.
	if result.Action == "edited" && result.UpdatedContent != "" {
		result.UpdatedContent = markdown.ToBlocks([]byte(result.UpdatedContent))
	}

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

	// Store only the explanation (not the full JSON with updatedContent)
	// to keep conversation history lightweight for subsequent turns.
	modelMsgID, err := models.NewID()
	if err != nil {
		return nil, err
	}
	modelSummary := result.Action + ": " + result.Explanation
	if err := repos.Messages.Create(ctx, &models.PostAssistantMessage{
		ID:      modelMsgID,
		PostID:  req.PostID,
		Role:    "model",
		Content: modelSummary,
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
