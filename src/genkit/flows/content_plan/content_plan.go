package content_plan

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"text/template"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"

	"github.com/content-control-center/app/src/genkit/flows"
	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

//go:embed prompts/content_plan.tmpl
var promptFS embed.FS

// ContentPlanFlow is the singleton flow for generating a content plan.
// Set by InitContentPlan; nil until then.
var ContentPlanFlow *core.Flow[ContentPlanRequest, *ContentPlanResponse, struct{}]

// ContentPlanFlowConfig holds the settings for the content plan flow.
type ContentPlanFlowConfig struct {
	ModelID    string
	MaxPieces  int
	Embedder   ai.Embedder // nil = skip semantic ranking, fall back to creation order
	systemTmpl *template.Template
	userTmpl   *template.Template
}

// ContentPlanRepos bundles all repository dependencies for the flow.
type ContentPlanRepos struct {
	Campaigns  repository.CampaignRepository
	Pieces     repository.PieceRepository
	Embeddings repository.PiecesEmbeddingsRepository
	Platforms  repository.PlatformRepository
	Posts      repository.PostRepository
}

// InitContentPlan registers the generateContentPlan Genkit flow. It must be
// called after the Genkit instance has been initialised with the Anthropic
// plugin.
func InitContentPlan(g *genkit.Genkit, cfg ContentPlanFlowConfig, repos ContentPlanRepos) error {
	raw, err := promptFS.ReadFile("prompts/content_plan.tmpl")
	if err != nil {
		return fmt.Errorf("load content_plan.tmpl: %w", err)
	}
	tmpl, err := template.New("content_plan").Parse(string(raw))
	if err != nil {
		return fmt.Errorf("parse content_plan.tmpl: %w", err)
	}
	cfg.systemTmpl = tmpl.Lookup("system")
	cfg.userTmpl = tmpl.Lookup("user")
	if cfg.systemTmpl == nil || cfg.userTmpl == nil {
		return fmt.Errorf("content_plan.tmpl must define both {{define \"system\"}} and {{define \"user\"}} blocks")
	}

	ContentPlanFlow = genkit.DefineFlow(g, "generateContentPlan",
		func(ctx context.Context, req ContentPlanRequest) (*ContentPlanResponse, error) {
			return runContentPlan(ctx, g, req, cfg, repos)
		},
	)
	return nil
}

// NewContentPlanCallback returns a synchronous callback that invokes
// ContentPlanFlow and is suitable for passing to the handler.
func NewContentPlanCallback() func(ctx context.Context, campaignID string) (*ContentPlanResponse, error) {
	return func(ctx context.Context, campaignID string) (*ContentPlanResponse, error) {
		resp, err := ContentPlanFlow.Run(ctx, ContentPlanRequest{CampaignID: campaignID})
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

// runContentPlan executes the six traced steps of the flow.
func runContentPlan(ctx context.Context, g *genkit.Genkit, req ContentPlanRequest, cfg ContentPlanFlowConfig, repos ContentPlanRepos) (*ContentPlanResponse, error) {
	// ── Step 1: validateInput ─────────────────────────────────────────────────
	campaign, err := genkit.Run(ctx, "validateInput", func() (*models.Campaign, error) {
		return validateInput(ctx, req.CampaignID, repos.Campaigns, repos.Pieces)
	})
	if err != nil {
		return nil, err
	}

	// ── Step 2: resolvePieces ─────────────────────────────────────────────────
	type piecesResult struct {
		Pieces   []resolvedPiece
		Warnings []string
	}
	pr, err := genkit.Run(ctx, "resolvePieces", func() (piecesResult, error) {
		pieces, warnings, err := resolvePieces(ctx, campaign, cfg, repos)
		return piecesResult{Pieces: pieces, Warnings: warnings}, err
	})
	if err != nil {
		return nil, err
	}
	warnings := pr.Warnings

	// ── Step 3: resolvePlatforms ──────────────────────────────────────────────
	platforms, err := genkit.Run(ctx, "resolvePlatforms", func() ([]resolvedPlatform, error) {
		return resolvePlatforms(ctx, campaign.TargetPlatformIDs, repos.Platforms)
	})
	if err != nil {
		return nil, err
	}

	// ── Step 4: generatePosts ─────────────────────────────────────────────────
	type genResult struct {
		Posts    []DraftPost
		Warnings []string
	}
	gr, err := genkit.Run(ctx, "generatePosts", func() (genResult, error) {
		posts, genWarnings, err := generatePosts(ctx, g, campaign, platforms, pr.Pieces, cfg)
		return genResult{Posts: posts, Warnings: genWarnings}, err
	})
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, gr.Warnings...)

	// ── Step 5: validateOutput ────────────────────────────────────────────────
	type validResult struct {
		Posts    []DraftPost
		Warnings []string
	}
	vr, err := genkit.Run(ctx, "validateOutput", func() (validResult, error) {
		posts, valWarnings := validateOutput(gr.Posts, campaign, platforms)
		return validResult{Posts: posts, Warnings: valWarnings}, nil
	})
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, vr.Warnings...)

	// ── Step 6: persistDraftPosts ─────────────────────────────────────────────
	_, err = genkit.Run(ctx, "persistDraftPosts", func() (struct{}, error) {
		return struct{}{}, persistDraftPosts(ctx, vr.Posts, campaign, repos.Posts)
	})
	if err != nil {
		return nil, err
	}

	return &ContentPlanResponse{
		CampaignID:  campaign.ID,
		GeneratedAt: time.Now().UTC(),
		Posts:       vr.Posts,
		Warnings:    warnings,
	}, nil
}

// ── Step implementations ──────────────────────────────────────────────────────

func validateInput(ctx context.Context, campaignID string, campaignRepo repository.CampaignRepository, pieceRepo repository.PieceRepository) (*models.Campaign, error) {
	c, err := campaignRepo.GetByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	var missing []string
	if c.Name == ""           { missing = append(missing, "name") }
	if c.Description == ""    { missing = append(missing, "description") }
	if c.TargetPersona == ""  { missing = append(missing, "target_persona") }
	if c.KeyMessages == ""    { missing = append(missing, "key_messages") }
	if c.ToneGuidelines == "" { missing = append(missing, "tone_guidelines") }
	if c.Objective == ""      { missing = append(missing, "objective") }
	if c.StartDate == nil     { missing = append(missing, "start_date") }
	if c.EndDate == nil       { missing = append(missing, "end_date") }
	if len(c.TargetPlatformIDs) == 0 {
		missing = append(missing, "target_platform_ids")
	}
	if len(missing) > 0 {
		return nil, &ValidationError{Msg: "missing required campaign fields: " + strings.Join(missing, ", ")}
	}

	if !c.StartDate.Before(*c.EndDate) {
		return nil, &ValidationError{Msg: "start_date must be before end_date"}
	}
	if c.EndDate.Sub(*c.StartDate) < 24*time.Hour {
		return nil, &ValidationError{Msg: "date range must be at least 1 day"}
	}

	// Verify specific piece IDs exist when UsePieces is true and IDs are given.
	if c.UsePieces && len(c.PiecesIDs) > 0 {
		for _, id := range c.PiecesIDs {
			if _, err := pieceRepo.GetByID(ctx, id); err != nil {
				return nil, &ValidationError{Msg: fmt.Sprintf("piece %q not found", id)}
			}
		}
	}

	return c, nil
}

func resolvePieces(ctx context.Context, campaign *models.Campaign, cfg ContentPlanFlowConfig, repos ContentPlanRepos) ([]resolvedPiece, []string, error) {
	if !campaign.UsePieces {
		return nil, nil, nil
	}

	var warnings []string
	var candidateIDs []string

	if len(campaign.PiecesIDs) > 0 {
		candidateIDs = campaign.PiecesIDs
	} else {
		all, err := repos.Pieces.List(ctx)
		if err != nil {
			return nil, nil, err
		}
		for _, p := range all {
			candidateIDs = append(candidateIDs, p.ID)
		}
	}

	// Semantic ranking when we have more candidates than the context budget.
	if len(candidateIDs) > cfg.MaxPieces && cfg.Embedder != nil {
		ranked, skipped, err := semanticRank(ctx, campaign, candidateIDs, cfg, repos.Embeddings)
		if err != nil {
			log.Printf("content_plan: semantic ranking failed (%v) — using creation order", err)
			warnings = append(warnings, "semantic ranking unavailable; using creation order for piece selection")
		} else {
			if len(skipped) > 0 {
				log.Printf("content_plan: excluded %d pieces due to context budget: %v", len(skipped), skipped)
				warnings = append(warnings, fmt.Sprintf("%d pieces excluded due to context budget", len(skipped)))
			}
			candidateIDs = ranked
		}
	}

	// Truncate to budget.
	if len(candidateIDs) > cfg.MaxPieces {
		warnings = append(warnings, fmt.Sprintf("%d pieces excluded due to context budget", len(candidateIDs)-cfg.MaxPieces))
		candidateIDs = candidateIDs[:cfg.MaxPieces]
	}

	// Build resolved pieces with text excerpts.
	pieces := make([]resolvedPiece, 0, len(candidateIDs))
	for _, id := range candidateIDs {
		p, err := repos.Pieces.GetByID(ctx, id)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("piece %q could not be loaded — skipped", id))
			continue
		}
		excerpt, err := buildExcerpt(p.Content)
		if err != nil {
			excerpt = "(content unavailable)"
		}
		pieces = append(pieces, resolvedPiece{ID: p.ID, Title: p.Title, Excerpt: excerpt})
	}

	return pieces, warnings, nil
}

// semanticRank ranks candidateIDs by cosine similarity to the campaign's key
// messages + description and returns the top-N IDs plus the excluded ones.
func semanticRank(ctx context.Context, campaign *models.Campaign, candidateIDs []string, cfg ContentPlanFlowConfig, embeddingRepo repository.PiecesEmbeddingsRepository) (top []string, excluded []string, err error) {
	query := campaign.KeyMessages + "\n" + campaign.Description
	qResp, err := cfg.Embedder.Embed(ctx, &ai.EmbedRequest{
		Input: []*ai.Document{ai.DocumentFromText(query, nil)},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("embed query: %w", err)
	}
	if len(qResp.Embeddings) == 0 {
		return nil, nil, fmt.Errorf("no embeddings returned for query")
	}
	queryVec := qResp.Embeddings[0].Embedding

	// Fetch stored embeddings for all candidates.
	embeddings := make(map[string][]float32, len(candidateIDs))
	for _, id := range candidateIDs {
		vec, _, err := embeddingRepo.GetByPieceID(ctx, id)
		if err != nil {
			continue // piece has no embedding yet — it will sort last
		}
		embeddings[id] = vec
	}

	ranked := rankByCosineSimilarity(queryVec, embeddings, cfg.MaxPieces)

	// Collect excluded IDs.
	rankedSet := make(map[string]bool, len(ranked))
	for _, id := range ranked {
		rankedSet[id] = true
	}
	for _, id := range candidateIDs {
		if !rankedSet[id] {
			excluded = append(excluded, id)
		}
	}

	return ranked, excluded, nil
}

// buildExcerpt extracts plain text from BlockNote JSON and truncates to ~800
// characters (~200 tokens).
func buildExcerpt(content string) (string, error) {
	text, err := flows.ExtractText(content)
	if err != nil {
		return "", err
	}
	const maxChars = 800
	if len(text) <= maxChars {
		return text, nil
	}
	// Truncate at a word boundary.
	truncated := text[:maxChars]
	if i := strings.LastIndexByte(truncated, ' '); i > 0 {
		truncated = truncated[:i]
	}
	return truncated + "…", nil
}

// platformCadence returns a human-readable cadence hint for a platform.
func platformCadence(platformID string) string {
	switch platformID {
	case "linkedin":
		return "1–2 posts per week"
	case "x-twitter":
		return "3–5 posts per week"
	case "facebook":
		return "3–5 posts per week"
	case "instagram":
		return "4–7 posts per week"
	case "threads":
		return "3–5 posts per week"
	case "youtube":
		return "1 video per week"
	default:
		return "2–3 posts per week"
	}
}

// platformConstraints returns brief content guidance for a platform.
func platformConstraints(platformID string) string {
	switch platformID {
	case "linkedin":
		return "professional tone; posts up to 3000 chars; articles up to 100k chars"
	case "x-twitter":
		return "posts max 280 chars; use threads for longer content"
	case "facebook":
		return "varied formats; posts up to 63k chars; balanced tone"
	case "instagram":
		return "visual-first; captions up to 2200 chars; strong opening line"
	case "threads":
		return "posts up to 500 chars; conversational tone"
	case "youtube":
		return "video-first; titles up to 100 chars; descriptions up to 5000 chars"
	default:
		return "adapt to platform norms"
	}
}

func resolvePlatforms(ctx context.Context, platformIDs models.StringSlice, platformRepo repository.PlatformRepository) ([]resolvedPlatform, error) {
	all, err := platformRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]models.Platform, len(all))
	for _, p := range all {
		byID[p.ID] = p
	}

	platforms := make([]resolvedPlatform, 0, len(platformIDs))
	for _, id := range platformIDs {
		p, ok := byID[id]
		if !ok {
			continue
		}
		typeNames := make([]string, 0, len(p.PostTypes))
		for _, name := range p.PostTypes {
			typeNames = append(typeNames, name)
		}
		platforms = append(platforms, resolvedPlatform{
			ID:          p.ID,
			Name:        p.Name,
			PostTypes:   strings.Join(typeNames, ", "),
			Cadence:     platformCadence(p.ID),
			Constraints: platformConstraints(p.ID),
		})
	}
	return platforms, nil
}

func generatePosts(ctx context.Context, g *genkit.Genkit, campaign *models.Campaign, platforms []resolvedPlatform, pieces []resolvedPiece, cfg ContentPlanFlowConfig) ([]DraftPost, []string, error) {
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
	return posts, nil, nil
}

func validateOutput(posts []DraftPost, campaign *models.Campaign, platforms []resolvedPlatform) ([]DraftPost, []string) {
	validPlatformIDs := make(map[string]bool, len(platforms))
	for _, p := range platforms {
		validPlatformIDs[p.ID] = true
	}

	startDate := campaign.StartDate.Format("2006-01-02")
	endDate := campaign.EndDate.Format("2006-01-02")

	valid := make([]DraftPost, 0, len(posts))
	var warnings []string

	for _, post := range posts {
		if !validPlatformIDs[post.PlatformID] {
			warnings = append(warnings, fmt.Sprintf("post %q dropped: unknown platformId %q", post.Title, post.PlatformID))
			continue
		}
		d := post.PublishDate
		if d < startDate || d > endDate {
			warnings = append(warnings, fmt.Sprintf("post %q dropped: publishDate %q is outside campaign range", post.Title, d))
			continue
		}
		valid = append(valid, post)
	}

	return valid, warnings
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
