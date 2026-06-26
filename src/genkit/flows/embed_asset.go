package flows

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
	"github.com/pgvector/pgvector-go"

	"github.com/ogen-app/ogen/src/genkit/embedopts"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// EmbedAssetInput is the typed input for the embedAsset flow.
type EmbedAssetInput struct {
	AssetID string `json:"asset_id"`
	Title   string `json:"title"`
	Content string `json:"content"` // raw BlockNote JSON
}

// EmbedAssetFlow is the singleton flow for embedding an asset.
// It is set by Init and ready for use after server startup.
var EmbedAssetFlow *core.Flow[EmbedAssetInput, struct{}, struct{}]

// embedScheduler serialises embed flows per asset and coalesces overlapping
// saves. If a new save arrives while an embed is running for the same asset,
// only the latest input is kept as "pending" — intermediate saves are
// discarded. When the running embed finishes, the pending one runs with the
// latest content. This prevents concurrent embeds of the same asset from
// flooding the embedserver and contending for the same database rows.
type embedScheduler struct {
	mu        sync.Mutex
	pending   map[string]EmbedAssetInput
	running   map[string]bool
	assetRepo repository.AssetRepository // optional; when set, used to flip asset.status
}

func newEmbedScheduler() *embedScheduler {
	return &embedScheduler{
		pending: make(map[string]EmbedAssetInput),
		running: make(map[string]bool),
	}
}

// Schedule queues an embed for the asset. Safe to call concurrently. Returns
// immediately — the embed runs in a background goroutine.
func (s *embedScheduler) Schedule(in EmbedAssetInput) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pending[in.AssetID] = in
	if s.running[in.AssetID] {
		log.Printf("embed asset %s: coalesced with in-flight embed", in.AssetID)
		return
	}
	s.running[in.AssetID] = true
	go s.run(in.AssetID)
}

func (s *embedScheduler) run(assetID string) {
	for {
		s.mu.Lock()
		in, ok := s.pending[assetID]
		if !ok {
			s.running[assetID] = false
			s.mu.Unlock()
			return
		}
		delete(s.pending, assetID)
		s.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		s.setStatus(ctx, in.AssetID, models.AssetStatusProcessing)
		_, err := EmbedAssetFlow.Run(ctx, in)
		if err != nil {
			log.Printf("embed asset %s: %v", in.AssetID, err)
			s.setStatus(ctx, in.AssetID, models.AssetStatusFailed)
		} else {
			s.setStatus(ctx, in.AssetID, models.AssetStatusReady)
		}
		cancel()
	}
}

func (s *embedScheduler) setStatus(ctx context.Context, assetID, status string) {
	if s.assetRepo == nil {
		return
	}
	if err := s.assetRepo.UpdateStatus(ctx, assetID, status); err != nil {
		log.Printf("embed asset %s: update status %s: %v", assetID, status, err)
	}
}

var defaultEmbedScheduler = newEmbedScheduler()

// NewAssetOnSaveCallback returns a fire-and-forget callback that schedules an
// embed of the asset. Concurrent saves of the same asset are serialised — at
// most one embed per asset runs at a time, and intermediate saves are
// coalesced into the latest content.
func NewAssetOnSaveCallback() func(assetID, title, content string) {
	return func(assetID, title, content string) {
		defaultEmbedScheduler.Schedule(EmbedAssetInput{
			AssetID: assetID,
			Title:   title,
			Content: content,
		})
	}
}

// Init registers all Genkit flows. It must be called once during server
// startup, after the Genkit instance and embedder have been initialised.
// assetRepo is optional — when non-nil, the scheduler flips asset.status as
// embeds run.
func Init(g *genkit.Genkit, embedder ai.Embedder, repo repository.AssetChunksRepository, assetRepo repository.AssetRepository) {
	defaultEmbedScheduler.assetRepo = assetRepo
	EmbedAssetFlow = genkit.DefineFlow(g, "embedAsset",
		func(ctx context.Context, in EmbedAssetInput) (struct{}, error) {
			return struct{}{}, embedAsset(ctx, embedder, repo, in)
		},
	)
}

func embedAsset(ctx context.Context, embedder ai.Embedder, repo repository.AssetChunksRepository, in EmbedAssetInput) error {
	// Asset content is stored as Markdown — feed it directly to the
	// chunker/embedder. Markdown is readable enough for embeddings; stripping
	// syntax wasn't worth its complexity cost.

	// Prepend title so every chunk carries the asset's identity.
	fullText := in.Title
	if in.Content != "" {
		fullText += "\n" + in.Content
	}

	chunkTexts := ChunkText(fullText)
	if len(chunkTexts) == 0 {
		log.Printf("embed asset %s: no text content to embed — skipping", in.AssetID)
		return nil
	}
	log.Printf("embed asset %s: %d chunk(s) from %d chars", in.AssetID, len(chunkTexts), len(fullText))

	// Embed each chunk individually. Gemini's EmbedContent can batch multiple
	// documents per request, but per-chunk calls keep the scheduler simple and
	// the chunk count per asset is small.
	chunks := make([]models.AssetChunk, 0, len(chunkTexts))
	for i, text := range chunkTexts {
		if !hasWords(text) {
			log.Printf("embed asset %s: chunk %d/%d SKIPPED (no words) len=%d repr=%q",
				in.AssetID, i, len(chunkTexts)-1, len(text), truncate(text, 80))
			continue
		}
		log.Printf("embed asset %s: chunk %d/%d len=%d preview=%q",
			in.AssetID, i, len(chunkTexts)-1, len(text), truncate(text, 80))

		resp, err := embedder.Embed(ctx, &ai.EmbedRequest{
			Input:   []*ai.Document{ai.DocumentFromText(text, nil)},
			Options: embedopts.Document(),
		})
		if err != nil {
			return fmt.Errorf("embed asset %s chunk %d: %w", in.AssetID, i, err)
		}
		if len(resp.Embeddings) != 1 {
			return fmt.Errorf("embed asset %s chunk %d: expected 1 embedding, got %d", in.AssetID, i, len(resp.Embeddings))
		}

		chunks = append(chunks, models.AssetChunk{
			ID:         fmt.Sprintf("%s:%d", in.AssetID, i),
			AssetID:    in.AssetID,
			ChunkIndex: i,
			Content:    text,
			TokenCount: EstimateTokens(text),
			Embedding:  pgvector.NewHalfVector(resp.Embeddings[0].Embedding),
			Model:      embedder.Name(),
		})
	}

	if err := repo.UpsertChunks(ctx, in.AssetID, chunks); err != nil {
		return fmt.Errorf("store chunks for asset %s: %w", in.AssetID, err)
	}

	log.Printf("embed asset %s: stored %d chunk(s)", in.AssetID, len(chunks))
	return nil
}

// hasWords returns true when text contains at least one non-whitespace Unicode
// word character (letter or digit). This is stricter than TrimSpace != ""
// and catches zero-width spaces, non-breaking spaces, and other invisible
// Unicode characters that TrimSpace doesn't strip but that the embedding model
// cannot produce tokens from.
func hasWords(text string) bool {
	for _, r := range text {
		if r > 32 && !strings.ContainsRune(" \t\n\r\v\f\u00a0\u200b\u200c\u200d\ufeff", r) {
			return true
		}
	}
	return false
}

// truncate returns the first n runes of s, followed by "…" if truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
