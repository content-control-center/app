package flows

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"

	"github.com/content-control-center/app/src/repository"
)

// EmbedAssetInput is the typed input for the embedPiece flow.
type EmbedAssetInput struct {
	AssetID string `json:"asset_id"`
	Title   string `json:"title"`
	Content string `json:"content"` // raw BlockNote JSON
}

// EmbedAssetFlow is the singleton flow for embedding a asset.
// It is set by Init and ready for use after server startup.
var EmbedAssetFlow *core.Flow[EmbedAssetInput, struct{}, struct{}]

// NewAssetOnSaveCallback returns a fire-and-forget callback that runs EmbedAssetFlow
// asynchronously. It is intended to be passed as the onSave argument to
// NewAssetsHandler.
func NewAssetOnSaveCallback() func(assetID, title, content string) {
	return func(assetID, title, content string) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := EmbedAssetFlow.Run(ctx, EmbedAssetInput{
			AssetID: assetID,
			Title:   title,
			Content: content,
		}); err != nil {
			log.Printf("embed asset %s: %v", assetID, err)
		}
	}
}

// Init registers all Genkit flows. It must be called once during server
// startup, after the Genkit instance and embedder have been initialised.
func Init(g *genkit.Genkit, embedder ai.Embedder, repo repository.AssetsEmbeddingsRepository) {
	EmbedAssetFlow = genkit.DefineFlow(g, "embedPiece",
		func(ctx context.Context, in EmbedAssetInput) (struct{}, error) {
			plainText, err := ExtractText(in.Content)
			if err != nil {
				return struct{}{}, fmt.Errorf("extract text from asset %s: %w", in.AssetID, err)
			}

			input := in.Title
			if plainText != "" {
				input += "\n" + plainText
			}

			resp, err := embedder.Embed(ctx, &ai.EmbedRequest{
				Input: []*ai.Document{
					ai.DocumentFromText(input, nil),
				},
			})
			if err != nil {
				return struct{}{}, fmt.Errorf("embed asset %s: %w", in.AssetID, err)
			}
			if len(resp.Embeddings) == 0 {
				return struct{}{}, fmt.Errorf("embed asset %s: no embeddings returned", in.AssetID)
			}

			if err := repo.Upsert(ctx, in.AssetID, resp.Embeddings[0].Embedding, embedder.Name()); err != nil {
				return struct{}{}, fmt.Errorf("store embedding for asset %s: %w", in.AssetID, err)
			}

			return struct{}{}, nil
		},
	)
}
