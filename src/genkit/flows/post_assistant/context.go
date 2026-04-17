package post_assistant

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"text/template"
	"time"
	"unicode/utf8"

	"github.com/content-control-center/app/src/genkit/flows"
	"github.com/content-control-center/app/src/models"
)

// maxSummaryChars caps the combined asset summaries at ~1000 tokens.
const maxSummaryChars = 4000

// previewChars is the target length for a single asset preview.
const previewChars = 200

// contextCacheTTL controls how long a cached context block stays valid.
const contextCacheTTL = 5 * time.Minute

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

// contextCacheEntry is a cached assistantContext keyed by post ID.
type contextCacheEntry struct {
	ctx       *assistantContext
	contentAt string // post.Content snapshot used to build this entry
	expiresAt time.Time
}

var (
	contextCache   = map[string]*contextCacheEntry{}
	contextCacheMu sync.Mutex
)

// assembleContextCached returns the cached context for the given post if it
// exists, the post content hasn't changed, and the TTL hasn't expired.
// Otherwise it assembles a fresh context, caches it, and returns it.
func assembleContextCached(
	ctx context.Context,
	post *models.Post,
	repos PostAssistantRepos,
	systemTmpl, contextTmpl *template.Template,
) (*assistantContext, error) {
	contextCacheMu.Lock()
	if entry, ok := contextCache[post.ID]; ok {
		if time.Now().Before(entry.expiresAt) && entry.contentAt == post.Content {
			contextCacheMu.Unlock()
			return entry.ctx, nil
		}
		delete(contextCache, post.ID)
	}
	contextCacheMu.Unlock()

	actx, err := assembleContext(ctx, post, repos, systemTmpl, contextTmpl)
	if err != nil {
		return nil, err
	}

	contextCacheMu.Lock()
	contextCache[post.ID] = &contextCacheEntry{
		ctx:       actx,
		contentAt: post.Content,
		expiresAt: time.Now().Add(contextCacheTTL),
	}
	contextCacheMu.Unlock()

	return actx, nil
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

	// Extract plain text from BlockNote JSON so the model sees readable
	// content rather than raw JSON. Falls back to raw content on error.
	postDescription := post.Content
	if plainText, err := flows.ExtractText(post.Content); err == nil && plainText != "" {
		postDescription = plainText
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
		PostDescription:     postDescription,
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

	// Fetch all assets in parallel.
	type result struct {
		index   int
		summary assetSummary
		ok      bool
	}
	results := make([]result, len(assetIDs))
	var wg sync.WaitGroup
	for i, id := range assetIDs {
		wg.Add(1)
		go func(idx int, assetID string) {
			defer wg.Done()
			asset, err := repos.Assets.GetByID(ctx, assetID)
			if err != nil {
				return
			}
			chunks, err := repos.Chunks.GetByAssetID(ctx, assetID)
			if err != nil {
				return
			}
			preview := ""
			if len(chunks) > 0 {
				preview = truncateRunes(chunks[0].Content, previewChars)
			}
			assetType := ""
			if asset.Type != nil {
				assetType = *asset.Type
			}
			results[idx] = result{
				index: idx,
				summary: assetSummary{
					ID:         asset.ID,
					Name:       asset.Title,
					Type:       assetType,
					ChunkCount: len(chunks),
					Preview:    preview,
				},
				ok: true,
			}
		}(i, id)
	}
	wg.Wait()

	summaries := make([]assetSummary, 0, len(assetIDs))
	for _, r := range results {
		if r.ok {
			summaries = append(summaries, r.summary)
		}
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
