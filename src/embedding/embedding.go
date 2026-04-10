package embedding

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/alephbet-ai/llama-genkit-embedder/llama"
	"github.com/firebase/genkit/go/genkit"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/genkit/flows"
	"github.com/content-control-center/app/src/repository"
)

const (
	DefaultRetryInterval = 5 * time.Second
	DefaultMaxRetries    = 12 // 12 × 5 s = 1 min total
)

// Init initialises Genkit + the llama embedder and returns an onSave callback.
// Returns nil, nil when embed server URL is empty (embedding disabled).
func Init(ctx context.Context, cfg *config.Config, repo repository.PiecesEmbeddingsRepository) (func(pieceID, title, content string), error) {
	return InitWithOptions(ctx, cfg.EmbedServerURL, DefaultMaxRetries, DefaultRetryInterval, repo)
}

// InitWithOptions is like Init but lets the caller control retry behaviour.
func InitWithOptions(ctx context.Context, embedServerURL string, maxRetries int, retryInterval time.Duration, repo repository.PiecesEmbeddingsRepository) (func(pieceID, title, content string), error) {
	if embedServerURL == "" {
		return nil, nil
	}

	if err := waitForEmbedServer(ctx, embedServerURL, maxRetries, retryInterval); err != nil {
		return nil, fmt.Errorf("embed server unavailable: %w", err)
	}

	plugin := llama.New(llama.Config{LlamaEmbedServerAddress: embedServerURL})
	g := genkit.Init(ctx, genkit.WithPlugins(plugin))

	embedder, err := plugin.DefineEmbedder(g)
	if err != nil {
		return nil, fmt.Errorf("init embedder: %w", err)
	}

	flows.Init(g, embedder, repo)

	return flows.NewPieceOnSaveCallback(), nil
}

func waitForEmbedServer(ctx context.Context, baseURL string, maxRetries int, retryInterval time.Duration) error {
	healthURL := baseURL + "/health"
	client := &http.Client{Timeout: retryInterval}

	attempts := maxRetries
	if attempts < 1 {
		attempts = 1
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		ok, err := checkEmbedHealth(client, healthURL)
		if ok {
			log.Printf("embed server ready after %d attempt(s)", attempt)
			return nil
		}
		if attempt == attempts {
			if err != nil {
				return err
			}
			return fmt.Errorf("embed server did not become healthy after %d attempts", attempts)
		}
		log.Printf("embed server not ready (attempt %d/%d): %v — retrying in %s",
			attempt, attempts, err, retryInterval)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryInterval):
		}
	}
	return nil
}

func checkEmbedHealth(client *http.Client, url string) (bool, error) {
	resp, err := client.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, fmt.Errorf("decode response: %w", err)
	}

	return body.Status == "ok", fmt.Errorf("unexpected status %q", body.Status)
}
