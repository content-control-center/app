package post_assistant

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"text/template"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/openai/openai-go"

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
			Description:   post.Content,
			Note:          "Initial version",
			Creator:       "user",
		}); err != nil {
			return nil, fmt.Errorf("create initial version: %w", err)
		}
		log.Printf("post_assistant[%s]: created initial version snapshot", req.PostID)
	}

	// ── Assemble context ─────────────────────────────────────────────────────
	actx, err := assembleContext(ctx, post, repos, systemTmpl, contextTmpl)
	if err != nil {
		return nil, fmt.Errorf("assemble context: %w", err)
	}

	// ── Load conversation history ────────────────────────────────────────────
	msgs, err := repos.Messages.ListRecentByPostID(ctx, req.PostID, 20)
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
	ctx = withRequestState(ctx, &requestState{
		postID:   req.PostID,
		assetIDs: post.UsedAssetIDs,
		repos:    repos,
		embedder: cfg.Embedder,
	})

	// ── Call model ───────────────────────────────────────────────────────────
	maxTokens := cfg.MaxOutputTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	modelName := "anthropic/" + cfg.ModelID

	// System + context block forms the stable cached prefix.
	systemBlock := actx.SystemPrompt + "\n\n" + actx.ContextBlock

	resp, err := genkit.Generate(ctx, g,
		ai.WithModelName(modelName),
		ai.WithSystem(systemBlock),
		ai.WithMessages(history...),
		ai.WithPrompt(req.Instruction),
		ai.WithTools(tools.listAssets, tools.getAssetChunks, tools.searchAssetChunks, tools.getCurrentDesc),
		ai.WithMaxTurns(5),
		ai.WithConfig(openai.ChatCompletionNewParams{
			MaxTokens: openai.Int(maxTokens),
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

	modelMsgID, err := models.NewID()
	if err != nil {
		return nil, err
	}
	if err := repos.Messages.Create(ctx, &models.PostAssistantMessage{
		ID:      modelMsgID,
		PostID:  req.PostID,
		Role:    "model",
		Content: text,
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
			Description:   result.UpdatedDescription,
			Note:          result.VersionNote,
			Creator:       "assistant",
		}); err != nil {
			return nil, fmt.Errorf("create version: %w", err)
		}
		log.Printf("post_assistant[%s]: created version %d — %s", req.PostID, nextNum, result.VersionNote)
	}

	// ── Update post content ──────────────────────────────────────────────────
	if result.Action == "edited" {
		post.Content = result.UpdatedDescription
		post.UpdatedAt = time.Now().UTC()
		if err := repos.Posts.Update(ctx, post); err != nil {
			return nil, fmt.Errorf("update post: %w", err)
		}
	}

	log.Printf("post_assistant[%s]: done in %s action=%s saveVersion=%v",
		req.PostID, time.Since(start).Round(time.Millisecond), result.Action, result.SaveVersion)

	return &result, nil
}
