package post_assistant

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/content-control-center/app/src/genkit/flows"
	"github.com/content-control-center/app/src/models"
)

// Per-turn token budget for tool-retrieved chunks.
const chunkTokenBudget = 3000

// ── Context key for per-request state ────────────────────────────────────────

type ctxKey int

const requestStateKey ctxKey = iota

type requestState struct {
	postID   string
	assetIDs []string
	repos    PostAssistantRepos
	embedder ai.Embedder
}

func withRequestState(ctx context.Context, s *requestState) context.Context {
	return context.WithValue(ctx, requestStateKey, s)
}

func getRequestState(ctx context.Context) *requestState {
	return ctx.Value(requestStateKey).(*requestState)
}

// ���─ Tool input/output types ──────────────────────────────────────────────────

// AssetInfo is the output element for the listAssets tool.
type AssetInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	ChunkCount int    `json:"chunkCount"`
}

// GetChunksInput is the input for the getAssetChunks tool.
type GetChunksInput struct {
	AssetID  string   `json:"assetId"            jsonschema:"description=The asset ID to retrieve chunks from"`
	ChunkIDs []string `json:"chunkIds,omitempty"  jsonschema:"description=Specific chunk IDs to retrieve; omit to get all chunks for the asset"`
}

// ChunkContent is a single chunk returned by chunk-retrieval tools.
type ChunkContent struct {
	ID      string `json:"id"`
	Index   int    `json:"index"`
	Content string `json:"content"`
}

// ChunksOutput wraps chunk results with a truncation indicator.
type ChunksOutput struct {
	Chunks    []ChunkContent `json:"chunks"`
	Truncated bool           `json:"truncated"`
}

// SearchChunksInput is the input for the searchAssetChunks tool.
type SearchChunksInput struct {
	AssetID string `json:"assetId" jsonschema:"description=The asset ID to search within"`
	Query   string `json:"query"   jsonschema:"description=Natural-language search query"`
}

// ── Tool registration ────────────────────────────────────────────────────────

type toolSet struct {
	listAssets        ai.ToolRef
	getAssetChunks    ai.ToolRef
	searchAssetChunks ai.ToolRef
	getCurrentDesc    ai.ToolRef
}

func defineTools(g *genkit.Genkit) *toolSet {
	list := genkit.DefineTool(g, "listAssets",
		"Returns the list of assets attached to the current post with their ID, name, type, and chunk count.",
		func(ctx *ai.ToolContext, _ struct{}) ([]AssetInfo, error) {
			return toolListAssets(ctx)
		},
	)

	getChunks := genkit.DefineTool(g, "getAssetChunks",
		"Retrieves text chunks from a specific asset. Omit chunkIds to get all chunks (subject to token budget).",
		func(ctx *ai.ToolContext, in GetChunksInput) (*ChunksOutput, error) {
			return toolGetAssetChunks(ctx, in)
		},
	)

	searchChunks := genkit.DefineTool(g, "searchAssetChunks",
		"Semantic search over an asset's chunks. Returns the most relevant chunks for the given query.",
		func(ctx *ai.ToolContext, in SearchChunksInput) (*ChunksOutput, error) {
			return toolSearchAssetChunks(ctx, in)
		},
	)

	getCurrentDesc := genkit.DefineTool(g, "getCurrentDescription",
		"Returns the latest post description text.",
		func(ctx *ai.ToolContext, _ struct{}) (string, error) {
			return toolGetCurrentDescription(ctx)
		},
	)

	return &toolSet{
		listAssets:        list,
		getAssetChunks:    getChunks,
		searchAssetChunks: searchChunks,
		getCurrentDesc:    getCurrentDesc,
	}
}

// ── Tool implementations ─────────────────────────────────────────────────────

func toolListAssets(ctx context.Context) ([]AssetInfo, error) {
	st := getRequestState(ctx)
	result := make([]AssetInfo, 0, len(st.assetIDs))
	for _, id := range st.assetIDs {
		asset, err := st.repos.Assets.GetByID(ctx, id)
		if err != nil {
			continue
		}
		chunks, err := st.repos.Chunks.GetByAssetID(ctx, id)
		if err != nil {
			continue
		}
		assetType := ""
		if asset.Type != nil {
			assetType = *asset.Type
		}
		result = append(result, AssetInfo{
			ID:         asset.ID,
			Name:       asset.Title,
			Type:       assetType,
			ChunkCount: len(chunks),
		})
	}
	return result, nil
}

func toolGetAssetChunks(ctx context.Context, in GetChunksInput) (*ChunksOutput, error) {
	st := getRequestState(ctx)

	var chunks []models.AssetChunk
	var err error
	if len(in.ChunkIDs) > 0 {
		chunks, err = st.repos.Chunks.GetByIDs(ctx, in.ChunkIDs)
	} else {
		chunks, err = st.repos.Chunks.GetByAssetID(ctx, in.AssetID)
	}
	if err != nil {
		return nil, fmt.Errorf("fetch chunks: %w", err)
	}

	return packChunks(chunks), nil
}

func toolSearchAssetChunks(ctx context.Context, in SearchChunksInput) (*ChunksOutput, error) {
	st := getRequestState(ctx)
	if st.embedder == nil {
		return nil, fmt.Errorf("semantic search is unavailable (no embedder configured); use getAssetChunks to retrieve chunks by ID instead")
	}

	allChunks, err := st.repos.Chunks.GetByAssetID(ctx, in.AssetID)
	if err != nil {
		return nil, fmt.Errorf("fetch chunks: %w", err)
	}

	qResp, err := st.embedder.Embed(ctx, &ai.EmbedRequest{
		Input: []*ai.Document{ai.DocumentFromText(in.Query, nil)},
	})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(qResp.Embeddings) == 0 {
		return &ChunksOutput{}, nil
	}
	queryVec := qResp.Embeddings[0].Embedding

	type scored struct {
		chunk models.AssetChunk
		score float32
	}
	var ranked []scored
	for _, c := range allChunks {
		if len(c.Embedding) == 0 {
			continue
		}
		vec := flows.DecodeVector(c.Embedding)
		s := cosineSimilarity(queryVec, vec)
		if s >= 0.5 {
			ranked = append(ranked, scored{chunk: c, score: s})
		}
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })

	result := make([]models.AssetChunk, 0, len(ranked))
	for _, r := range ranked {
		result = append(result, r.chunk)
	}

	return packChunks(result), nil
}

func toolGetCurrentDescription(ctx context.Context) (string, error) {
	st := getRequestState(ctx)
	post, err := st.repos.Posts.GetByID(ctx, st.postID)
	if err != nil {
		return "", fmt.Errorf("fetch post: %w", err)
	}
	// Return plain text so the model can read the content.
	if plainText, err := flows.ExtractText(post.Content); err == nil && plainText != "" {
		return plainText, nil
	}
	return post.Content, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func packChunks(chunks []models.AssetChunk) *ChunksOutput {
	var out ChunksOutput
	tokens := 0
	for _, c := range chunks {
		if tokens+c.TokenCount > chunkTokenBudget && len(out.Chunks) > 0 {
			out.Truncated = true
			break
		}
		out.Chunks = append(out.Chunks, ChunkContent{
			ID:      c.ID,
			Index:   c.ChunkIndex,
			Content: c.Content,
		})
		tokens += c.TokenCount
	}
	return &out
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return float32(dot / denom)
}
