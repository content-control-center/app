package embedding

import (
	"context"
	"fmt"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"

	"github.com/ogen-app/ogen/src/config"
	"github.com/ogen-app/ogen/src/genkit/embedopts"
	"github.com/ogen-app/ogen/src/genkit/flows"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/storage"
)

// Callbacks bundles the fire-and-forget callbacks registered by Init.
// OnMarkdownSave may be nil when embeddings are disabled.
//
// PDF ingestion no longer rides a callback here — it goes through the
// process_pdf River job (CON-103), enqueued by the asset upload handler.
type Callbacks struct {
	OnMarkdownSave func(assetID, title, content, tenantID string)
}

// embeddingDimensions is the fixed embedding width baked into the
// assets_chunks.embedding halfvec(3072) column (CON-101 migration). The Gemini
// embedder must emit exactly this many dimensions; any other value produces
// vectors that fail to insert. Changing it requires a schema migration, so it
// is a compile-time constant rather than config-derived.
const embeddingDimensions = 3072

// Init builds a dedicated Genkit instance backed by the hosted Gemini Embedding
// 2 model (CON-101), registers the embedding flows, and returns the
// fire-and-forget callbacks plus the raw embedder (used for query-time semantic
// ranking). The embedding instance is deliberately separate from the Anthropic
// flow runtime so an Anthropic key rotation never disturbs the embedder bound
// here.
//
// Returns zero values (embedding disabled) when GeminiAPIKey is empty: asset
// saves still succeed, but no vectors are written and semantic search returns
// nothing — mirroring the previous empty-EMBED_SERVER_URL behaviour. Because
// the embedder is a hosted API there is no readiness poll at boot.
func Init(
	ctx context.Context,
	cfg *config.Config,
	chunksRepo repository.AssetChunksRepository,
	assetRepo repository.AssetRepository,
	fileRepo repository.AssetFileRepository,
	store storage.Storage,
) (Callbacks, ai.Embedder, error) {
	if cfg.GeminiAPIKey == "" {
		return Callbacks{}, nil, nil
	}

	// Fail fast at boot if the configured dimension diverges from the fixed
	// halfvec column contract, rather than surfacing as opaque per-chunk insert
	// errors deep in the async embed flow.
	if cfg.EmbedDimensions != embeddingDimensions {
		return Callbacks{}, nil, fmt.Errorf(
			"EMBED_DIMENSIONS=%d does not match the assets_chunks.embedding halfvec(%d) column; changing the embedding dimension requires a schema migration",
			cfg.EmbedDimensions, embeddingDimensions)
	}

	// Output dimensionality drives every embed request and must match the
	// assets_chunks.embedding halfvec(N) column; set it before any flow fires.
	embedopts.Dimensions = int32(cfg.EmbedDimensions)

	plugin := &googlegenai.GoogleAI{APIKey: cfg.GeminiAPIKey}
	g := genkit.Init(ctx, genkit.WithPlugins(plugin))

	embedder, err := plugin.DefineEmbedder(g, cfg.EmbedModel, &ai.EmbedderOptions{
		Label:      "Gemini Embedding 2",
		Dimensions: cfg.EmbedDimensions,
	})
	if err != nil {
		return Callbacks{}, nil, fmt.Errorf("init gemini embedder %q: %w", cfg.EmbedModel, err)
	}

	flows.Init(g, embedder, chunksRepo, assetRepo)

	return Callbacks{
		OnMarkdownSave: flows.NewAssetOnSaveCallback(),
	}, embedder, nil
}
