package content_plan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"text/template"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

func generatePosts(
	ctx context.Context,
	g *genkit.Genkit,
	campaign *models.Campaign,
	platforms []resolvedPlatform,
	pieces []resolvedPiece,
	cfg ContentPlanFlowConfig,
) ([]DraftPost, []string, error) {
	estCount := 0
	if campaign.EstimatedPostCount != nil {
		estCount = *campaign.EstimatedPostCount
	}

	dayCount := int(campaign.EndDate.Sub(*campaign.StartDate).Hours() / 24)

	data := contentPlanTemplateData{
		Name:               campaign.Name,
		Description:        campaign.Description,
		Objective:          string(campaign.Objective),
		TargetPersona:      campaign.TargetPersona,
		KeyMessages:        campaign.KeyMessages,
		ToneGuidelines:     campaign.ToneGuidelines,
		Language:           campaign.Language,
		StartDate:          campaign.StartDate.Format("2006-01-02"),
		EndDate:            campaign.EndDate.Format("2006-01-02"),
		DayCount:           dayCount,
		EstimatedPostCount: estCount,
		Platforms:          platforms,
		Pieces:             pieces,
	}

	systemPrompt, err := renderTemplate(cfg.systemTmpl, data)
	if err != nil {
		return nil, nil, fmt.Errorf("render system prompt: %w", err)
	}
	userPrompt, err := renderTemplate(cfg.userTmpl, data)
	if err != nil {
		return nil, nil, fmt.Errorf("render user prompt: %w", err)
	}

	log.Printf("content_plan: system prompt:\n%s", systemPrompt)
	log.Printf("content_plan: user prompt:\n%s", userPrompt)

	// Retry loop: 1 retry with 2s backoff on API or parse failure.
	const maxAttempts = 2
	var resp *ai.ModelResponse
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err = genkit.Generate(ctx, g,
			ai.WithModelName("anthropic/"+cfg.ModelID),
			ai.WithSystem(systemPrompt),
			ai.WithPrompt(userPrompt),
		)
		if err == nil {
			break
		}
		if attempt < maxAttempts {
			log.Printf("content_plan: generatePosts attempt %d failed (%v) — retrying in 2s", attempt, err)
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
	}
	if err != nil {
		return nil, nil, &AIError{Msg: fmt.Sprintf("model call failed after %d attempts: %v", maxAttempts, err)}
	}

	// Log token usage for cost tracking.
	if resp != nil && resp.Usage != nil {
		log.Printf("content_plan: tokens — input=%d output=%d total=%d",
			resp.Usage.InputTokens, resp.Usage.OutputTokens,
			resp.Usage.InputTokens+resp.Usage.OutputTokens)
	}

	text := strings.TrimSpace(resp.Text())
	// Strip optional markdown code fences the model may add despite instructions.
	if strings.HasPrefix(text, "```") {
		if i := strings.Index(text, "\n"); i >= 0 {
			text = text[i+1:]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
		text = strings.TrimSpace(text)
	}

	var posts []DraftPost
	if err := json.Unmarshal([]byte(text), &posts); err != nil {
		return nil, nil, &AIError{Msg: fmt.Sprintf("model response is not valid JSON: %v\nraw: %.200s", err, text)}
	}

	const maxBodyRunes = 500
	var warnings []string
	for i := range posts {
		runes := []rune(posts[i].Body)
		if len(runes) > maxBodyRunes {
			posts[i].Body = string(runes[:maxBodyRunes])
			warnings = append(warnings, fmt.Sprintf("post %d body truncated to %d chars", i, maxBodyRunes))
		}
	}
	return posts, warnings, nil
}

func persistDraftPosts(ctx context.Context, posts []DraftPost, campaign *models.Campaign, postRepo repository.PostRepository) error {
	if len(posts) == 0 {
		return nil
	}

	records := make([]*models.Post, 0, len(posts))
	for _, dp := range posts {
		id, err := models.NewID()
		if err != nil {
			return err
		}

		var scheduledAt *time.Time
		if t, err := time.Parse("2006-01-02", dp.PublishDate); err == nil {
			scheduledAt = &t
		}

		records = append(records, &models.Post{
			ID:                  id,
			CampaignID:          campaign.ID,
			PlatformID:          dp.PlatformID,
			PlatformPostType:    dp.ContentType,
			Title:               dp.Title,
			Content:             dp.Body,
			MediaURLs:           models.StringSlice{},
			Status:              models.PostStatusDraft,
			CTAType:             models.CTATypeNone,
			CTAUrl:              "",
			TargetAudienceNotes: dp.ToneNotes,
			UsedPiecesIDs:       models.StringSlice(dp.PieceRefs),
			ScheduledAt:         scheduledAt,
			CreatedBy:           campaign.CreatedBy,
		})
	}

	return postRepo.CreateBatch(ctx, records)
}

func renderTemplate(tmpl *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
