package flows

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"

	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/pdf"
	"github.com/content-control-center/app/src/repository"
	"github.com/content-control-center/app/src/storage"
)

// ProcessPDFInput is the typed input for the processPDF flow.
type ProcessPDFInput struct {
	AssetID      string `json:"asset_id"`
	Data         []byte `json:"-"`
	OriginalName string `json:"original_name"`
	MimeType     string `json:"mime_type"`
}

// ProcessPDFFlow is the singleton flow for ingesting a PDF into an Asset.
// Set by InitPDF; ready after server startup.
var ProcessPDFFlow *core.Flow[ProcessPDFInput, struct{}, struct{}]

type pdfDeps struct {
	embedder ai.Embedder
	chunks   repository.AssetChunksRepository
	assets   repository.AssetRepository
	files    repository.AssetFileRepository
	storage  storage.Storage
}

var defaultPDFDeps *pdfDeps

// InitPDF registers the processPDF flow. Call once during server startup,
// after the embedder has been initialised. Storage may be nil, in which case
// S3 uploads (of both the PDF and its thumbnail) are skipped — useful for
// test and dev environments without object storage configured.
func InitPDF(
	g *genkit.Genkit,
	embedder ai.Embedder,
	chunksRepo repository.AssetChunksRepository,
	assetRepo repository.AssetRepository,
	fileRepo repository.AssetFileRepository,
	store storage.Storage,
) {
	defaultPDFDeps = &pdfDeps{
		embedder: embedder,
		chunks:   chunksRepo,
		assets:   assetRepo,
		files:    fileRepo,
		storage:  store,
	}
	ProcessPDFFlow = genkit.DefineFlow(g, "processPDF",
		func(ctx context.Context, in ProcessPDFInput) (struct{}, error) {
			return struct{}{}, processPDF(ctx, defaultPDFDeps, in)
		},
	)
}

// NewPDFProcessCallback returns a fire-and-forget callback that runs the
// processPDF flow in a background goroutine. One PDF per asset → no
// coalescing needed.
func NewPDFProcessCallback() func(ProcessPDFInput) {
	return func(in ProcessPDFInput) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			if ProcessPDFFlow == nil {
				log.Printf("process PDF asset %s: flow not initialised", in.AssetID)
				return
			}
			if _, err := ProcessPDFFlow.Run(ctx, in); err != nil {
				log.Printf("process PDF asset %s: %v", in.AssetID, err)
			}
		}()
	}
}

func processPDF(ctx context.Context, d *pdfDeps, in ProcessPDFInput) error {
	if d == nil {
		return fmt.Errorf("pdf deps not initialised")
	}

	setStatus := func(status string) {
		if d.assets == nil {
			return
		}
		if err := d.assets.UpdateStatus(ctx, in.AssetID, status); err != nil {
			log.Printf("pdf %s: set status %s: %v", in.AssetID, status, err)
		}
	}

	setStatus(models.AssetStatusProcessing)

	// 1. Upload PDF to S3. If storage is disabled, skip but keep processing.
	s3Key := fmt.Sprintf("assets/%s/original.pdf", in.AssetID)
	if d.storage != nil {
		if _, err := d.storage.Upload(ctx, s3Key, bytes.NewReader(in.Data), int64(len(in.Data)), "application/pdf"); err != nil {
			setStatus(models.AssetStatusFailed)
			return fmt.Errorf("upload pdf: %w", err)
		}
	}

	// 2. Extract text (ledongthuc primary, pdftotext fallback).
	pages, err := pdf.ExtractText(ctx, in.Data)
	if err != nil {
		setStatus(models.AssetStatusFailed)
		return fmt.Errorf("extract pdf: %w", err)
	}
	pageCount, _ := pdf.PageCount(in.Data)

	// 3. Page-aware chunk + embed.
	pagedChunks := pdf.ChunkPages(pages)
	log.Printf("pdf %s: %d page(s), %d chunk(s)", in.AssetID, pageCount, len(pagedChunks))

	chunks := make([]models.AssetChunk, 0, len(pagedChunks))
	var embedFailures int
	for i, pc := range pagedChunks {
		if !hasWords(pc.Text) {
			continue
		}
		resp, err := d.embedder.Embed(ctx, &ai.EmbedRequest{
			Input: []*ai.Document{ai.DocumentFromText(pc.Text, nil)},
		})
		if err != nil {
			log.Printf("pdf %s chunk %d: embed failed: %v", in.AssetID, i, err)
			embedFailures++
			continue
		}
		if len(resp.Embeddings) != 1 {
			embedFailures++
			continue
		}
		ps, pe := pc.PageStart, pc.PageEnd
		chunks = append(chunks, models.AssetChunk{
			ID:         fmt.Sprintf("%s:%d", in.AssetID, i),
			AssetID:    in.AssetID,
			ChunkIndex: i,
			PageStart:  &ps,
			PageEnd:    &pe,
			Content:    pc.Text,
			TokenCount: EstimateTokens(pc.Text),
			Embedding:  encodeVector(resp.Embeddings[0].Embedding),
			Model:      d.embedder.Name(),
		})
	}

	if len(chunks) > 0 {
		if err := d.chunks.UpsertChunks(ctx, in.AssetID, chunks); err != nil {
			setStatus(models.AssetStatusFailed)
			return fmt.Errorf("store chunks: %w", err)
		}
	}

	// 4. Thumbnail (non-blocking). Errors here must not fail the asset.
	var thumbKey *string
	if d.storage != nil {
		if img, tErr := pdf.RenderThumbnail(ctx, in.Data, 96); tErr == nil {
			k := fmt.Sprintf("assets/%s/thumbnail.png", in.AssetID)
			if _, upErr := d.storage.Upload(ctx, k, bytes.NewReader(img), int64(len(img)), "image/png"); upErr == nil {
				thumbKey = &k
			} else {
				log.Printf("pdf %s: thumbnail upload failed: %v", in.AssetID, upErr)
			}
		} else {
			log.Printf("pdf %s: thumbnail render failed: %v", in.AssetID, tErr)
		}
	}

	// 5. Persist file metadata.
	if d.files != nil {
		fileID, idErr := models.NewID()
		if idErr == nil {
			var pcPtr *int
			if pageCount > 0 {
				pcPtr = &pageCount
			}
			if err := d.files.Upsert(ctx, &models.AssetFile{
				ID:             fileID,
				AssetID:        in.AssetID,
				OriginalName:   in.OriginalName,
				MimeType:       in.MimeType,
				SizeBytes:      int64(len(in.Data)),
				S3Key:          s3Key,
				ThumbnailS3Key: thumbKey,
				PageCount:      pcPtr,
			}); err != nil {
				log.Printf("pdf %s: upsert asset_files: %v", in.AssetID, err)
			}
		}
	}

	// 6. Final status. partial = some chunks embedded, some failed.
	switch {
	case len(pagedChunks) == 0:
		// Nothing to embed — treat as ready (empty/image-only PDF).
		setStatus(models.AssetStatusReady)
	case embedFailures > 0 && len(chunks) > 0:
		setStatus(models.AssetStatusPartial)
	case len(chunks) == 0:
		setStatus(models.AssetStatusFailed)
	default:
		setStatus(models.AssetStatusReady)
	}
	return nil
}
