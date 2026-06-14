package post_assistant

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"
	"unicode/utf8"

	"github.com/ogen-app/ogen/src/models"
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
// fingerprint captures every post field that feeds into the rendered
// prompt — any change to it invalidates the entry. Tracked explicitly
// rather than diffing field-by-field so adding a new field is a one-line
// update to postFingerprint.
type contextCacheEntry struct {
	ctx         *assistantContext
	fingerprint string
	expiresAt   time.Time
}

var (
	contextCache   = map[string]*contextCacheEntry{}
	contextCacheMu sync.Mutex
)

// postFingerprint returns a stable string that changes whenever any
// prompt-affecting field of the post changes (content, attached assets,
// phase). Asset IDs are joined without sorting so a reorder also busts
// the cache — the order is surfaced to the model via listAssets output.
func postFingerprint(post *models.Post) string {
	var b strings.Builder
	b.WriteString(post.Content)
	b.WriteByte('\x1f')
	b.WriteString(strings.Join(post.UsedAssetIDs, ","))
	b.WriteByte('\x1f')
	if post.CampaignTypePhaseID != nil {
		b.WriteString(*post.CampaignTypePhaseID)
	}
	return b.String()
}

// assembleContextCached returns the cached context for the given post if
// the fingerprint matches and the TTL hasn't expired. Otherwise it
// assembles a fresh context, caches it, and returns it.
func assembleContextCached(
	ctx context.Context,
	post *models.Post,
	repos PostAssistantRepos,
	systemTmpl, contextTmpl *template.Template,
) (*assistantContext, error) {
	fp := postFingerprint(post)

	contextCacheMu.Lock()
	if entry, ok := contextCache[post.ID]; ok {
		if time.Now().Before(entry.expiresAt) && entry.fingerprint == fp {
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
		ctx:         actx,
		fingerprint: fp,
		expiresAt:   time.Now().Add(contextCacheTTL),
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

	versions, err := buildVersionSummaries(ctx, post, repos)
	if err != nil {
		return nil, err
	}

	// Available platforms power the clonePost tool's platform resolution.
	// Best-effort: a load failure (or no platforms repo) just omits the
	// section — the tool still validates server-side.
	var platforms []platformOption
	if repos.Platforms != nil {
		if ps, perr := repos.Platforms.List(ctx); perr == nil {
			platforms = toPlatformOptions(ps)
		}
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
		PostContent:         post.Content,
		Assets:              summaries,
		Platforms:           platforms,
		Versions:            versions,
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
	PostContent         string
	Assets              []assetSummary
	Platforms           []platformOption
	Versions            []versionSummary
}

// versionSummary is a single entry in the post's version history,
// surfaced to the model so it can map "v2" / "the previous version" to a
// concrete version number for the restoreVersion tool. Content is
// deliberately omitted to keep the prompt small — only the metadata the
// model needs to choose a number is included.
type versionSummary struct {
	Number    int
	Note      string
	Creator   string
	IsCurrent bool
}

// buildVersionSummaries lists the post's saved versions (oldest first),
// flagging the latest (highest version_number) as the current HEAD.
// Best-effort: a load failure is propagated, but no versions simply
// omits the section. Rendered into the cached context block; a content
// change (the common case) busts the cache, so the only staleness window
// is a manual snapshot within the TTL — harmless, since the restore tool
// and service always validate the chosen number against the live DB.
func buildVersionSummaries(ctx context.Context, post *models.Post, repos PostAssistantRepos) ([]versionSummary, error) {
	if repos.Versions == nil {
		return nil, nil
	}
	versions, err := repos.Versions.ListByPostID(ctx, post.ID)
	if err != nil {
		return nil, err
	}
	out := make([]versionSummary, 0, len(versions))
	for i, v := range versions {
		out = append(out, versionSummary{
			Number:  v.VersionNumber,
			Note:    v.Note,
			Creator: v.Creator,
			// The latest snapshot is the current HEAD. ListByPostID is
			// ordered ascending, so that's the last element. Content
			// equality would wrongly flag every snapshot that shares the
			// restored content (e.g. after restoring v1, both v1 and the
			// appended restore version match the live content).
			IsCurrent: i == len(versions)-1,
		})
	}
	return out, nil
}

// platformOption is a platform the user can clone a post onto, with its
// available post-type slugs — surfaced to the model for the clonePost tool.
type platformOption struct {
	Name      string
	PostTypes []string
}

func toPlatformOptions(platforms []models.Platform) []platformOption {
	opts := make([]platformOption, 0, len(platforms))
	for _, p := range platforms {
		types := make([]string, 0, len(p.PostTypes))
		for slug := range p.PostTypes {
			types = append(types, slug)
		}
		sort.Strings(types)
		opts = append(opts, platformOption{Name: p.Name, PostTypes: types})
	}
	return opts
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
