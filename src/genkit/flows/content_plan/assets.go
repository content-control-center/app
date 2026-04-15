package content_plan

import (
	"context"
	"fmt"
	"log"
	"sort"

	"github.com/firebase/genkit/go/ai"

	"github.com/content-control-center/app/src/genkit/flows"
	"github.com/content-control-center/app/src/models"
)

// minAssetSimilarity is the minimum cosine similarity a chunk must score
// against the campaign query to be included in the prompt context.
const minAssetSimilarity = 0.7

func resolveAssets(ctx context.Context, campaign *models.Campaign, cfg ContentPlanFlowConfig, repos ContentPlanRepos) ([]resolvedPiece, []string, error) {
	if !campaign.UseAssets {
		return nil, nil, nil
	}

	var warnings []string

	// Determine the candidate asset IDs.
	var candidateIDs []string
	if len(campaign.AssetIDs) > 0 {
		candidateIDs = campaign.AssetIDs
	} else {
		all, err := repos.Assets.List(ctx)
		if err != nil {
			return nil, nil, err
		}
		for _, a := range all {
			candidateIDs = append(candidateIDs, a.ID)
		}
	}

	if cfg.MaxContextChars == 0 {
		cfg.MaxContextChars = 10000
	}

	// With an embedder: rank chunks by semantic similarity and greedily pack.
	if cfg.Embedder != nil {
		candidateSet := make(map[string]bool, len(candidateIDs))
		for _, id := range candidateIDs {
			candidateSet[id] = true
		}

		ranked, w, err := rankAndPackChunks(ctx, campaign, candidateSet, cfg, repos)
		if err != nil {
			log.Printf("content_plan: chunk ranking failed (%v) — falling back to creation order", err)
			warnings = append(warnings, "chunk ranking unavailable; using creation order for asset selection")
		} else {
			warnings = append(warnings, w...)
			return ranked, warnings, nil
		}
	}

	// Fallback: no embedder — load assets in creation order up to MaxContextAssets.
	if len(candidateIDs) > cfg.MaxContextAssets {
		warnings = append(warnings, fmt.Sprintf("%d assets excluded due to context budget", len(candidateIDs)-cfg.MaxContextAssets))
		candidateIDs = candidateIDs[:cfg.MaxContextAssets]
	}

	result := make([]resolvedPiece, 0, len(candidateIDs))
	for _, id := range candidateIDs {
		a, err := repos.Assets.GetByID(ctx, id)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("asset %q could not be loaded — skipped", id))
			continue
		}
		excerpt, err := buildExcerpt(a.Content)
		if err != nil {
			excerpt = "(content unavailable)"
		}
		result = append(result, resolvedPiece{ID: a.ID, Title: a.Title, Excerpt: excerpt})
	}
	return result, warnings, nil
}

// rankAndPackChunks fetches all embedded chunks, scores them against the
// campaign query, and greedily packs top-scoring chunks into the context budget.
// Multiple chunks from the same asset are merged in chunk_index order.
func rankAndPackChunks(
	ctx context.Context,
	campaign *models.Campaign,
	candidateSet map[string]bool,
	cfg ContentPlanFlowConfig,
	repos ContentPlanRepos,
) ([]resolvedPiece, []string, error) {
	var warnings []string

	// Embed the campaign query.
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

	// Fetch all embedded chunks and filter to candidates.
	allChunks, err := repos.Chunks.GetAllEmbedded(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch embedded chunks: %w", err)
	}

	type scoredChunk struct {
		chunk models.AssetChunk
		score float32
	}

	scored := make([]scoredChunk, 0, len(allChunks))
	for _, c := range allChunks {
		if !candidateSet[c.AssetID] {
			continue
		}
		vec := flows.DecodeVector(c.Embedding)
		s := cosineSimilarity(queryVec, vec)
		log.Printf("content_plan: chunk %s (asset %s idx %d) similarity=%.4f (pass=%v)",
			c.ID, c.AssetID, c.ChunkIndex, s, s >= minAssetSimilarity)
		if s >= minAssetSimilarity {
			scored = append(scored, scoredChunk{chunk: c, score: s})
		}
	}

	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })

	// Greedy packing: accumulate chunks until the context budget is exhausted.
	type assetEntry struct {
		title  string
		chunks []models.AssetChunk
	}
	selected := make(map[string]*assetEntry)
	order := make([]string, 0)
	totalChars := 0

	for _, sc := range scored {
		if totalChars >= cfg.MaxContextChars {
			break
		}
		if _, exists := selected[sc.chunk.AssetID]; !exists {
			selected[sc.chunk.AssetID] = &assetEntry{}
			order = append(order, sc.chunk.AssetID)
		}
		selected[sc.chunk.AssetID].chunks = append(selected[sc.chunk.AssetID].chunks, sc.chunk)
		totalChars += len(sc.chunk.Content)
	}

	if len(selected) == 0 {
		warnings = append(warnings, "no chunks met the similarity threshold; no asset context will be provided")
		return nil, warnings, nil
	}

	// Populate asset titles.
	for assetID, entry := range selected {
		a, err := repos.Assets.GetByID(ctx, assetID)
		if err != nil {
			entry.title = assetID // fallback to ID if asset can't be loaded
			continue
		}
		entry.title = a.Title
	}

	// Build resolved pieces: one entry per asset, chunks concatenated in order.
	result := make([]resolvedPiece, 0, len(order))
	for _, assetID := range order {
		entry := selected[assetID]

		sort.Slice(entry.chunks, func(i, j int) bool {
			return entry.chunks[i].ChunkIndex < entry.chunks[j].ChunkIndex
		})

		var parts []string
		for _, c := range entry.chunks {
			if c.Content != "" {
				parts = append(parts, c.Content)
			}
		}
		result = append(result, resolvedPiece{
			ID:      assetID,
			Title:   entry.title,
			Excerpt: joinChunks(parts),
		})
	}

	return result, warnings, nil
}

// buildExcerpt extracts plain text from BlockNote JSON and truncates to ~800
// characters for the no-embedder fallback path.
func buildExcerpt(content string) (string, error) {
	text, err := flows.ExtractText(content)
	if err != nil {
		return "", err
	}
	const maxChars = 800
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text, nil
	}
	return string(runes[:maxChars]) + "…", nil
}

// joinChunks concatenates chunk texts with a visual separator.
func joinChunks(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += "\n\n[...]\n\n" + p
	}
	return result
}
