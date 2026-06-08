package embedding

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/alephbet-ai/llama-genkit-embedder/llama"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"

	"github.com/ogen-app/ogen/src/config"
	"github.com/ogen-app/ogen/src/genkit/flows"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/storage"
)

// Callbacks bundles the fire-and-forget callbacks registered by Init.
// Either field may be nil when embeddings are disabled or the corresponding
// dependency (e.g. asset file repo) is nil.
type Callbacks struct {
	OnMarkdownSave func(assetID, title, content string)
	OnPDFProcess   func(flows.ProcessPDFInput)
}

const (
	DefaultRetryInterval = 5 * time.Second
	DefaultMaxRetries    = 12 // 12 × 5 s = 1 min total
)

// Init initialises Genkit + the llama embedder and returns an onSave callback
// and the raw embedder (used for semantic ranking). Returns nil, nil, nil when
// embed server URL is empty (embedding disabled).
func Init(
	ctx context.Context,
	cfg *config.Config,
	chunksRepo repository.AssetChunksRepository,
	assetRepo repository.AssetRepository,
	fileRepo repository.AssetFileRepository,
	store storage.Storage,
) (Callbacks, ai.Embedder, error) {
	return InitWithOptions(ctx, cfg.EmbedServerURL, DefaultMaxRetries, DefaultRetryInterval, chunksRepo, assetRepo, fileRepo, store)
}

// InitWithOptions is like Init but lets the caller control retry behaviour.
func InitWithOptions(
	ctx context.Context,
	embedServerURL string,
	maxRetries int,
	retryInterval time.Duration,
	chunksRepo repository.AssetChunksRepository,
	assetRepo repository.AssetRepository,
	fileRepo repository.AssetFileRepository,
	store storage.Storage,
) (Callbacks, ai.Embedder, error) {
	if embedServerURL == "" {
		return Callbacks{}, nil, nil
	}

	if err := waitForEmbedServer(ctx, embedServerURL, maxRetries, retryInterval); err != nil {
		return Callbacks{}, nil, fmt.Errorf("embed server unavailable: %w", err)
	}

	plugin := llama.New(llama.Config{LlamaEmbedServerAddress: embedServerURL})
	g := genkit.Init(ctx, genkit.WithPlugins(plugin))

	embedder, err := plugin.DefineEmbedder(g)
	if err != nil {
		return Callbacks{}, nil, fmt.Errorf("init embedder: %w", err)
	}

	flows.Init(g, embedder, chunksRepo, assetRepo)
	flows.InitPDF(g, embedder, chunksRepo, assetRepo, fileRepo, store)

	return Callbacks{
		OnMarkdownSave: flows.NewAssetOnSaveCallback(),
		OnPDFProcess:   flows.NewPDFProcessCallback(),
	}, embedder, nil
}

// WaitAndNewPlugin waits for the embed server to be ready and returns the
// llama plugin so the caller can include it in a shared genkit.Init() call.
// Returns nil when embedServerURL is empty (embedding disabled).
func WaitAndNewPlugin(
	ctx context.Context,
	embedServerURL string,
	maxRetries int,
	retryInterval time.Duration,
) (api.Plugin, error) {
	if embedServerURL == "" {
		return nil, nil
	}
	if err := waitForEmbedServer(ctx, embedServerURL, maxRetries, retryInterval); err != nil {
		return nil, fmt.Errorf("embed server unavailable: %w", err)
	}
	return llama.New(llama.Config{LlamaEmbedServerAddress: embedServerURL}), nil
}

// RegisterFlows defines the embedder and registers embedding flows on the
// given Genkit instance. Call this after genkit.Init() when using a shared
// instance created with the plugin from WaitAndNewPlugin.
func RegisterFlows(
	g *genkit.Genkit,
	plugin api.Plugin,
	chunksRepo repository.AssetChunksRepository,
	assetRepo repository.AssetRepository,
	fileRepo repository.AssetFileRepository,
	store storage.Storage,
) (Callbacks, ai.Embedder, error) {
	llamaPlugin, ok := plugin.(*llama.Plugin)
	if !ok {
		return Callbacks{}, nil, fmt.Errorf("expected *llama.Plugin, got %T", plugin)
	}

	embedder, err := llamaPlugin.DefineEmbedder(g)
	if err != nil {
		return Callbacks{}, nil, fmt.Errorf("init embedder: %w", err)
	}

	flows.Init(g, embedder, chunksRepo, assetRepo)
	flows.InitPDF(g, embedder, chunksRepo, assetRepo, fileRepo, store)

	return Callbacks{
		OnMarkdownSave: flows.NewAssetOnSaveCallback(),
		OnPDFProcess:   flows.NewPDFProcessCallback(),
	}, embedder, nil
}

func waitForEmbedServer(ctx context.Context, baseURL string, maxRetries int, retryInterval time.Duration) error {
	healthURL := baseURL + "/health"
	client := &http.Client{Timeout: retryInterval}

	attempts := maxRetries
	if attempts < 1 {
		attempts = 1
	}

	for i := 1; i <= attempts; i++ {
		resp, err := client.Get(healthURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				// Also verify the response body contains { "status": "ok" }.
				body := struct{ Status string }{}
				resp2, _ := client.Get(healthURL)
				if resp2 != nil {
					defer resp2.Body.Close()
					_ = json.NewDecoder(resp2.Body).Decode(&body)
				}
				return nil
			}
		}
		log.Printf("embed server not ready (attempt %d/%d): %v — retrying in %s", i, attempts, err, retryInterval)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryInterval):
		}
	}
	return fmt.Errorf("%s", healthURL)
}
