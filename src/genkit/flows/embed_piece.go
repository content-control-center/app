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

// EmbedPieceInput is the typed input for the embedPiece flow.
type EmbedPieceInput struct {
	PieceID string `json:"piece_id"`
	Title   string `json:"title"`
	Content string `json:"content"` // raw BlockNote JSON
}

// EmbedPieceFlow is the singleton flow for embedding a piece.
// It is set by Init and ready for use after server startup.
var EmbedPieceFlow *core.Flow[EmbedPieceInput, struct{}, struct{}]

// NewPieceOnSaveCallback returns a fire-and-forget callback that runs EmbedPieceFlow
// asynchronously. It is intended to be passed as the onSave argument to
// NewPiecesHandler.
func NewPieceOnSaveCallback() func(pieceID, title, content string) {
	return func(pieceID, title, content string) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := EmbedPieceFlow.Run(ctx, EmbedPieceInput{
			PieceID: pieceID,
			Title:   title,
			Content: content,
		}); err != nil {
			log.Printf("embed piece %s: %v", pieceID, err)
		}
	}
}

// Init registers all Genkit flows. It must be called once during server
// startup, after the Genkit instance and embedder have been initialised.
func Init(g *genkit.Genkit, embedder ai.Embedder, repo repository.EmbeddingRepository) {
	EmbedPieceFlow = genkit.DefineFlow(g, "embedPiece",
		func(ctx context.Context, in EmbedPieceInput) (struct{}, error) {
			plainText, err := extractText(in.Content)
			if err != nil {
				return struct{}{}, fmt.Errorf("extract text from piece %s: %w", in.PieceID, err)
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
				return struct{}{}, fmt.Errorf("embed piece %s: %w", in.PieceID, err)
			}
			if len(resp.Embeddings) == 0 {
				return struct{}{}, fmt.Errorf("embed piece %s: no embeddings returned", in.PieceID)
			}

			if err := repo.Upsert(ctx, in.PieceID, resp.Embeddings[0].Embedding, embedder.Name()); err != nil {
				return struct{}{}, fmt.Errorf("store embedding for piece %s: %w", in.PieceID, err)
			}

			return struct{}{}, nil
		},
	)
}
