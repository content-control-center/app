package server

import (
	"context"

	"github.com/firebase/genkit/go/ai"

	"github.com/ogen-app/ogen/src/embedding"
	"github.com/ogen-app/ogen/src/kernel/config"
	"github.com/ogen-app/ogen/src/kernel/usage"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/secrets"
	"github.com/ogen-app/ogen/src/storage"
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
	secretStore secrets.Store,
	recorder *usage.Recorder,
) (embedding.Callbacks, ai.Embedder, error) {
	return embedding.Init(ctx, cfg, chunksRepo, assetRepo, fileRepo, store, secretStore, recorder)
}
