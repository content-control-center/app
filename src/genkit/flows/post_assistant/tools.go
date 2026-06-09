package post_assistant

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/ogen-app/ogen/src/genkit/flows"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/postclone"
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

	// Clone support (CON-59). cloneSvc/actor drive the clonePost tool;
	// platforms lets it resolve a platform name → ID; onEvent lets it
	// emit a clone_started SSE event mid-generation; cloneResult is set
	// by the tool and read by the runner after generation to finalise
	// the response.
	cloneSvc    *postclone.Service
	actor       string
	platforms   []models.Platform
	onEvent     OnEventFunc
	cloneResult *postclone.Result
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

// ClonePostInput is the input for the clonePost tool.
type ClonePostInput struct {
	TargetPlatform string `json:"targetPlatform,omitempty" jsonschema:"description=Platform name or ID for the clone (e.g. Threads). Omit to keep the source's platform."`
	TargetPostType string `json:"targetPostType,omitempty" jsonschema:"description=Post-type slug for the target platform (e.g. text-post). Omit to inherit or default."`
	Content        string `json:"content,omitempty"        jsonschema:"description=Full Markdown content for the clone. For a cross-platform clone provide content adapted to the target platform; omit to copy the source content verbatim."`
	Title          string `json:"title,omitempty"          jsonschema:"description=Title for the clone. Omit to apply the default naming."`
}

// ClonePostOutput is returned to the model after a clone is created.
type ClonePostOutput struct {
	NewPostID        string `json:"newPostId"`
	PlatformID       string `json:"platformId,omitempty"`
	PostType         string `json:"postType,omitempty"`
	Adapted          bool   `json:"adapted"`
	PostTypeFellBack bool   `json:"postTypeFellBack"`
}

// ── Tool registration ────────────────────────────────────────────────────────

type toolSet struct {
	listAssets        ai.ToolRef
	getAssetChunks    ai.ToolRef
	searchAssetChunks ai.ToolRef
	getCurrentContent ai.ToolRef
	clonePost         ai.ToolRef
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

	getCurrentContent := genkit.DefineTool(g, "getCurrentContent",
		"Returns the latest post content as plain text.",
		func(ctx *ai.ToolContext, _ struct{}) (string, error) {
			return toolGetCurrentContent(ctx)
		},
	)

	clonePost := genkit.DefineTool(g, "clonePost",
		"Duplicates the current post as a new draft in the same campaign. "+
			"To clone for another platform, set targetPlatform and provide content adapted to that platform. "+
			"Omit content for a verbatim copy. Returns the new draft's id.",
		func(ctx *ai.ToolContext, in ClonePostInput) (*ClonePostOutput, error) {
			return toolClonePost(ctx, in)
		},
	)

	return &toolSet{
		listAssets:        list,
		getAssetChunks:    getChunks,
		searchAssetChunks: searchChunks,
		getCurrentContent: getCurrentContent,
		clonePost:         clonePost,
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

func toolGetCurrentContent(ctx context.Context) (string, error) {
	st := getRequestState(ctx)
	post, err := st.repos.Posts.GetByID(ctx, st.postID)
	if err != nil {
		return "", fmt.Errorf("fetch post: %w", err)
	}
	return post.Content, nil
}

func toolClonePost(ctx context.Context, in ClonePostInput) (*ClonePostOutput, error) {
	st := getRequestState(ctx)
	if st.cloneSvc == nil {
		return nil, fmt.Errorf("cloning is not available")
	}

	opts := postclone.DefaultOptions(st.actor, postclone.TriggerAssistant)
	if in.TargetPlatform != "" {
		id, err := resolvePlatform(st.platforms, in.TargetPlatform)
		if err != nil {
			return nil, err
		}
		opts.TargetPlatformID = id
	}
	opts.TargetPostType = in.TargetPostType
	if in.Content != "" {
		opts.ContentOverride = &in.Content
	}
	if in.Title != "" {
		opts.TitleOverride = &in.Title
	}

	emit(st.onEvent, SSEEventCloneStarted, CloneStartedEventPayload{TargetPlatform: in.TargetPlatform})

	res, err := st.cloneSvc.Clone(ctx, st.postID, opts)
	if err != nil {
		return nil, err
	}
	st.cloneResult = res

	return &ClonePostOutput{
		NewPostID:        res.Post.ID,
		PlatformID:       res.Post.PlatformID,
		PostType:         res.ResolvedPostType,
		Adapted:          res.Adapted,
		PostTypeFellBack: res.PostTypeFellBack,
	}, nil
}

// resolvePlatform maps a platform identifier from the model (an exact ID
// or a case-insensitive name) to a platform ID. Returns an error listing
// the available platforms when nothing matches, so the model can ask the
// user to clarify rather than guess.
func resolvePlatform(platforms []models.Platform, ident string) (string, error) {
	for _, p := range platforms {
		if p.ID == ident {
			return p.ID, nil
		}
	}
	for _, p := range platforms {
		if strings.EqualFold(p.Name, ident) {
			return p.ID, nil
		}
	}
	names := make([]string, 0, len(platforms))
	for _, p := range platforms {
		names = append(names, p.Name)
	}
	return "", fmt.Errorf("unknown platform %q; available platforms: %s", ident, strings.Join(names, ", "))
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
