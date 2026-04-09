package server

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
	embedRetryInterval = 5 * time.Second
	embedMaxRetries    = 12 // 12 × 5 s = 1 min total
)

// initEmbedding initialises Genkit + the llama embedder and returns an onSave
// callback suitable for passing to NewPiecesHandler. Returns nil, nil when the
// embed server URL is empty (embedding disabled).
func initEmbedding(ctx context.Context, cfg *config.Config, repo repository.PiecesEmbeddingsRepository) (func(pieceID, title, content string), error) {
	if cfg.EmbedServerURL == "" {
		return nil, nil
	}

	if err := waitForEmbedServer(ctx, cfg.EmbedServerURL); err != nil {
		return nil, fmt.Errorf("embed server unavailable: %w", err)
	}

	plugin := llama.New(llama.Config{LlamaEmbedServerAddress: cfg.EmbedServerURL})
	// genkit.Init panics on bad options; errors from network calls surface via DefineEmbedder.
	g := genkit.Init(ctx, genkit.WithPlugins(plugin))

	embedder, err := plugin.DefineEmbedder(g)
	if err != nil {
		return nil, fmt.Errorf("init embedder: %w", err)
	}

	flows.Init(g, embedder, repo)

	return flows.NewPieceOnSaveCallback(), nil
}

// waitForEmbedServer polls GET /health on the embed server until it returns
// {"status":"ok"} or the retry budget is exhausted.
func waitForEmbedServer(ctx context.Context, baseURL string) error {
	healthURL := baseURL + "/health"
	client := &http.Client{Timeout: embedRetryInterval}

	for attempt := 1; attempt <= embedMaxRetries; attempt++ {
		ok, err := checkEmbedHealth(client, healthURL)
		if ok {
			log.Printf("embed server ready after %d attempt(s)", attempt)
			return nil
		}
		if attempt == embedMaxRetries {
			if err != nil {
				return err
			}
			return fmt.Errorf("embed server did not become healthy after %d attempts", embedMaxRetries)
		}
		log.Printf("embed server not ready (attempt %d/%d): %v — retrying in %s",
			attempt, embedMaxRetries, err, embedRetryInterval)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(embedRetryInterval):
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
