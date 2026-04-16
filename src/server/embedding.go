package server

import (
	"context"

	"github.com/firebase/genkit/go/ai"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/embedding"
	"github.com/content-control-center/app/src/repository"
	"github.com/content-control-center/app/src/storage"
)

// initEmbedding delegates to the embedding package so the same setup can be
// reused by both the server and the seed command.
func initEmbedding(
	ctx context.Context,
	cfg *config.Config,
	chunksRepo repository.AssetChunksRepository,
	assetRepo repository.AssetRepository,
	fileRepo repository.AssetFileRepository,
	store storage.Storage,
) (embedding.Callbacks, ai.Embedder, error) {
	return embedding.Init(ctx, cfg, chunksRepo, assetRepo, fileRepo, store)
}
