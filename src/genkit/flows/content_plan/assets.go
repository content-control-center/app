package content_plan

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/firebase/genkit/go/ai"

	"github.com/content-control-center/app/src/genkit/flows"
	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

func resolveAssets(ctx context.Context, campaign *models.Campaign, cfg ContentPlanFlowConfig, repos ContentPlanRepos) ([]resolvedPiece, []string, error) {
	if !campaign.UseAssets {
		return nil, nil, nil
	}

	var warnings []string
	var candidateIDs []string

	if len(campaign.AssetIDs) > 0 {
		candidateIDs = campaign.AssetIDs
	} else {
		all, err := repos.Assets.List(ctx)
		if err != nil {
			return nil, nil, err
		}
		for _, p := range all {
			candidateIDs = append(candidateIDs, p.ID)
		}
	}

	// Semantic ranking: filter by relevance and trim to context budget.
	if cfg.Embedder != nil {
		ranked, skipped, err := semanticRank(ctx, campaign, candidateIDs, cfg, repos.Embeddings)
		if err != nil {
			log.Printf("content_plan: semantic ranking failed (%v) — using creation order", err)
			warnings = append(warnings, "semantic ranking unavailable; using creation order for asset selection")
		} else {
			if len(skipped) > 0 {
				log.Printf("content_plan: excluded %d assets due to context budget: %v", len(skipped), skipped)
				warnings = append(warnings, fmt.Sprintf("%d assets excluded due to context budget", len(skipped)))
			}
			candidateIDs = ranked
		}
	}

	// Truncate to budget.
	if len(candidateIDs) > cfg.MaxAssets {
		warnings = append(warnings, fmt.Sprintf("%d assets excluded due to context budget", len(candidateIDs)-cfg.MaxAssets))
		candidateIDs = candidateIDs[:cfg.MaxAssets]
	}

	// Build resolved assets with text excerpts.
	assets := make([]resolvedPiece, 0, len(candidateIDs))
	for _, id := range candidateIDs {
		p, err := repos.Assets.GetByID(ctx, id)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("asset %q could not be loaded — skipped", id))
			continue
		}
		excerpt, err := buildExcerpt(p.Content)
		if err != nil {
			excerpt = "(content unavailable)"
		}
		assets = append(assets, resolvedPiece{ID: p.ID, Title: p.Title, Excerpt: excerpt})
	}

	return assets, warnings, nil
}

// minAssetSimilarity is the minimum cosine similarity a asset must score
// against the campaign query to be included in the prompt context.
// Calibrated for the embeddinggemma-300m model: related content typically
// scores 0.70–0.85; loosely related falls in 0.50–0.65.
const minAssetSimilarity = 0.7

// semanticRank ranks candidateIDs by cosine similarity to the campaign's key
// messages + description, filters out assets below minAssetSimilarity, and
// returns the top-N IDs plus the excluded ones.
func semanticRank(ctx context.Context, campaign *models.Campaign, candidateIDs []string, cfg ContentPlanFlowConfig, embeddingRepo repository.AssetsEmbeddingsRepository) (top []string, excluded []string, err error) {
	query := campaign.Name + "\n" + campaign.KeyMessages + "\n" + campaign.Description
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
		vec, _, err := embeddingRepo.GetByAssetID(ctx, id)
		if err != nil {
			continue // asset has no embedding yet — it will sort last
		}
		embeddings[id] = vec
	}

	ranked := rankByCosineSimilarity(queryVec, embeddings, cfg.MaxAssets, minAssetSimilarity)

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
