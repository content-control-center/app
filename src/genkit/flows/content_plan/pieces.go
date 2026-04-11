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

	// Semantic ranking: filter by relevance and trim to context budget.
	if cfg.Embedder != nil {
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

// minPieceSimilarity is the minimum cosine similarity a piece must score
// against the campaign query to be included in the prompt context.
const minPieceSimilarity = 0.4

// semanticRank ranks candidateIDs by cosine similarity to the campaign's key
// messages + description, filters out pieces below minPieceSimilarity, and
// returns the top-N IDs plus the excluded ones.
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

	ranked := rankByCosineSimilarity(queryVec, embeddings, cfg.MaxPieces, minPieceSimilarity)

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
