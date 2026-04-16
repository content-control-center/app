package post_assistant

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
	"unicode/utf8"

	"github.com/content-control-center/app/src/models"
)

// maxSummaryChars caps the combined asset summaries at ~1000 tokens.
const maxSummaryChars = 4000

// previewChars is the target length for a single asset preview.
const previewChars = 200

// assetSummary is a lightweight representation of an attached asset.
type assetSummary struct {
	ID         string
	Name       string
	Type       string
	ChunkCount int
	Preview    string
}

// assistantContext holds the rendered prompts ready for the model call.
type assistantContext struct {
	SystemPrompt string // system instructions (stable across turns)
	ContextBlock string // campaign + phase + assets (stable across turns)
}

func assembleContext(
	ctx context.Context,
	post *models.Post,
	repos PostAssistantRepos,
	systemTmpl, contextTmpl *template.Template,
) (*assistantContext, error) {
	campaign := post.Campaign
	if campaign == nil {
		c, err := repos.Campaigns.GetByID(ctx, post.CampaignID)
		if err != nil {
			return nil, fmt.Errorf("load campaign: %w", err)
		}
		campaign = c
	}

	var phaseName, phaseDescription string
	if post.CampaignTypePhase != nil {
		phaseName = post.CampaignTypePhase.Name
		phaseDescription = post.CampaignTypePhase.Purpose
	}

	summaries, err := buildAssetSummaries(ctx, post.UsedAssetIDs, repos)
	if err != nil {
		return nil, err
	}

	data := contextTemplateData{
		CampaignName:        campaign.Name,
		CampaignDescription: campaign.Description,
		TargetPersona:       campaign.TargetPersona,
		KeyMessages:         campaign.KeyMessages,
		ToneGuidelines:      campaign.ToneGuidelines,
		Language:            campaign.Language,
		PhaseName:           phaseName,
		PhaseDescription:    phaseDescription,
		PostDescription:     post.Content,
		Assets:              summaries,
	}

	systemPrompt, err := renderTemplate(systemTmpl, data)
	if err != nil {
		return nil, fmt.Errorf("render system prompt: %w", err)
	}
	contextBlock, err := renderTemplate(contextTmpl, data)
	if err != nil {
		return nil, fmt.Errorf("render context block: %w", err)
	}

	return &assistantContext{
		SystemPrompt: systemPrompt,
		ContextBlock: contextBlock,
	}, nil
}

type contextTemplateData struct {
	CampaignName        string
	CampaignDescription string
	TargetPersona       string
	KeyMessages         string
	ToneGuidelines      string
	Language            string
	PhaseName           string
	PhaseDescription    string
	PostDescription     string
	Assets              []assetSummary
}

func buildAssetSummaries(ctx context.Context, assetIDs []string, repos PostAssistantRepos) ([]assetSummary, error) {
	if len(assetIDs) == 0 {
		return nil, nil
	}

	summaries := make([]assetSummary, 0, len(assetIDs))
	for _, id := range assetIDs {
		asset, err := repos.Assets.GetByID(ctx, id)
		if err != nil {
			continue // skip missing assets
		}

		chunks, err := repos.Chunks.GetByAssetID(ctx, id)
		if err != nil {
			continue
		}

		preview := ""
		if len(chunks) > 0 {
			preview = truncateRunes(chunks[0].Content, previewChars)
		}

		assetType := ""
		if asset.Type != nil {
			assetType = *asset.Type
		}

		summaries = append(summaries, assetSummary{
			ID:         asset.ID,
			Name:       asset.Title,
			Type:       assetType,
			ChunkCount: len(chunks),
			Preview:    preview,
		})
	}

	// Shorten previews proportionally if combined size exceeds budget.
	totalChars := 0
	for _, s := range summaries {
		totalChars += len(s.Preview)
	}
	if totalChars > maxSummaryChars && len(summaries) > 0 {
		budget := maxSummaryChars / len(summaries)
		for i := range summaries {
			summaries[i].Preview = truncateRunes(summaries[i].Preview, budget)
		}
	}

	return summaries, nil
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}

func renderTemplate(tmpl *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
