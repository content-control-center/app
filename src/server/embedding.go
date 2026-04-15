package server

import (
	"context"

	"github.com/firebase/genkit/go/ai"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/embedding"
	"github.com/content-control-center/app/src/repository"
)

// initEmbedding delegates to the embedding package so the same setup can be
// reused by both the server and the seed command.
func initEmbedding(ctx context.Context, cfg *config.Config, repo repository.AssetChunksRepository) (func(assetID, title, content string), ai.Embedder, error) {
	return embedding.Init(ctx, cfg, repo)
}
