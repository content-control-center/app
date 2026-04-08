package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/alephbet-ai/llama-genkit-embedder/llama"
	"github.com/firebase/genkit/go/genkit"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/genkit/flows"
	"github.com/content-control-center/app/src/repository"
)

// initEmbedding initialises Genkit + the llama embedder and returns an onSave
// callback suitable for passing to NewPiecesHandler. Returns nil, nil when the
// embed server URL is empty (embedding disabled).
func initEmbedding(ctx context.Context, cfg *config.Config, repo repository.EmbeddingRepository) (func(pieceID, title, content string), error) {
	if cfg.EmbedServerURL == "" {
		return nil, nil
	}

	plugin := llama.New(llama.Config{LlamaEmbedServerAddress: cfg.EmbedServerURL})
	// genkit.Init panics on bad options; errors from network calls surface via DefineEmbedder.
	g := genkit.Init(ctx, genkit.WithPlugins(plugin))

	embedder, err := plugin.DefineEmbedder(g)
	if err != nil {
		return nil, fmt.Errorf("init embedder: %w", err)
	}

	flows.Init(g, embedder, repo)

	onSave := func(pieceID, title, content string) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := flows.EmbedPieceFlow.Run(ctx, flows.EmbedPieceInput{
			PieceID: pieceID,
			Title:   title,
			Content: content,
		}); err != nil {
			log.Printf("embed piece %s: %v", pieceID, err)
		}
	}

	return onSave, nil
}
