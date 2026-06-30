package queues

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/pgvector/pgvector-go"
	"github.com/riverqueue/river"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/ogen-app/ogen/src/genkit/embedopts"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/pdfclient"
	"github.com/ogen-app/ogen/src/storage"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// ProcessPDFQueue ingests an uploaded PDF (CON-103): download original.pdf from
// object storage, parse it via pdf-service over gRPC (text -> page-aware chunks
// + thumbnail), embed the chunks, and persist chunks + file metadata. Replaces
// the old fire-and-forget goroutine with a durable, retried River job, so a
// crash or transient failure no longer strands an asset in "processing".
const ProcessPDFQueue = "process_pdf"

// thumbnailDPIDefault is used when PDFDeps.ThumbnailDPI is 0.
const thumbnailDPIDefault = 96

// Narrow dependency interfaces — the real client/embedder/storage/repos satisfy
// these structurally; tests provide small fakes.

type pdfParser interface {
	Parse(ctx context.Context, r io.Reader, opts pdfclient.Options) (*pdfclient.Result, error)
}

type chunkEmbedder interface {
	Embed(ctx context.Context, req *ai.EmbedRequest) (*ai.EmbedResponse, error)
	Name() string
}

type blobStore interface {
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error)
}

type assetStatusUpdater interface {
	UpdateStatus(ctx context.Context, id, status string) error
}

type chunkUpserter interface {
	UpsertChunks(ctx context.Context, assetID string, chunks []models.AssetChunk) error
}

type fileUpserter interface {
	Upsert(ctx context.Context, file *models.AssetFile) error
}

// PDFDeps bundles the process_pdf worker's dependencies (built in server.go). A
// nil Client (no PDF_SERVICE_ADDR configured) disables the job — it no-ops.
type PDFDeps struct {
	Client       pdfParser
	Embedder     chunkEmbedder
	Storage      blobStore
	Assets       assetStatusUpdater
	Chunks       chunkUpserter
	Files        fileUpserter
	ThumbnailDPI int
}

// ProcessPDFTask carries the asset to ingest. The PDF bytes are NOT in the args
// (River args are JSON in Postgres, far too small for a 50 MB PDF); the worker
// re-reads original.pdf from storage on each attempt.
type ProcessPDFTask struct {
	AssetID      string `json:"asset_id"`
	TenantID     string `json:"tenant_id"`
	OriginalName string `json:"original_name"`
	MimeType     string `json:"mime_type"`
}

func (ProcessPDFTask) Kind() string { return ProcessPDFQueue }

// InsertOpts bounds retries. Transient failures (storage, gRPC Unavailable /
// DeadlineExceeded, embedder outage) retry with backoff; terminal ones (corrupt
// PDF) short-circuit to "failed" inside Work.
func (ProcessPDFTask) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5}
}

type ProcessPDFProcessor struct {
	river.WorkerDefaults[ProcessPDFTask]
	Deps PDFDeps
}

func init() {
	register(func(w *river.Workers, d Deps) {
		river.AddWorker(w, &ProcessPDFProcessor{Deps: d.PDF})
	})
}

// Work scopes the job to the asset's tenant and processes it. lastAttempt lets
// process() flip an otherwise-retryable embedder outage to a terminal "failed"
// instead of abandoning the asset in "processing".
func (p *ProcessPDFProcessor) Work(ctx context.Context, job *river.Job[ProcessPDFTask]) error {
	ctx = tenantctx.With(ctx, job.Args.TenantID)
	return p.process(ctx, job.Args, job.Attempt >= job.MaxAttempts)
}

// Timeout covers a worst-case parse (large PDF) plus per-chunk embedding.
func (p *ProcessPDFProcessor) Timeout(*river.Job[ProcessPDFTask]) time.Duration {
	return 16 * time.Minute
}

func (p *ProcessPDFProcessor) process(ctx context.Context, in ProcessPDFTask, lastAttempt bool) error {
	if p.Deps.Client == nil {
		log.Printf("process_pdf %s: pdf-service not configured; leaving asset pending", in.AssetID)
		return nil
	}
	if p.Deps.Storage == nil {
		// Best-effort status write; we return the more descriptive error below.
		_ = p.setStatus(ctx, in.AssetID, models.AssetStatusFailed)
		return fmt.Errorf("process_pdf %s: storage not configured", in.AssetID)
	}

	if err := p.setStatus(ctx, in.AssetID, models.AssetStatusProcessing); err != nil {
		return err
	}

	// 1. Re-read original.pdf (the upload handler stored it before enqueue).
	//    Transient read errors retry.
	key := storage.TenantKey(ctx, fmt.Sprintf("assets/%s/original.pdf", in.AssetID))
	rc, err := p.Deps.Storage.Download(ctx, key)
	if err != nil {
		return fmt.Errorf("process_pdf %s: download %s: %w", in.AssetID, key, err)
	}
	data, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		return fmt.Errorf("process_pdf %s: read pdf: %w", in.AssetID, err)
	}

	// 2. Parse via pdf-service. Corrupt/unsupported PDFs are terminal (no retry).
	res, err := p.Deps.Client.Parse(ctx, bytes.NewReader(data), pdfclient.Options{
		Filename:        in.OriginalName,
		RenderThumbnail: true,
		ThumbnailDPI:    p.thumbnailDPI(),
	})
	if err != nil {
		if isTerminalParseErr(err) {
			log.Printf("process_pdf %s: unparseable pdf: %v", in.AssetID, err)
			return p.setStatus(ctx, in.AssetID, models.AssetStatusFailed)
		}
		return fmt.Errorf("process_pdf %s: parse: %w", in.AssetID, err)
	}

	// 3. Embed each chunk that has words.
	chunks := make([]models.AssetChunk, 0, len(res.Chunks))
	var embedAttempts, embedFailures int
	for _, ch := range res.Chunks {
		if !hasWords(ch.Text) {
			continue
		}
		embedAttempts++
		emb, eErr := p.Deps.Embedder.Embed(ctx, &ai.EmbedRequest{
			Input:   []*ai.Document{ai.DocumentFromText(ch.Text, nil)},
			Options: embedopts.Document(),
		})
		if eErr != nil || len(emb.Embeddings) != 1 {
			embedFailures++
			continue
		}
		chunk := models.AssetChunk{
			ID:         fmt.Sprintf("%s:%d", in.AssetID, ch.Index),
			AssetID:    in.AssetID,
			ChunkIndex: ch.Index,
			Content:    ch.Text,
			TokenCount: estimateTokens(ch.Text),
			Embedding:  pgvector.NewHalfVector(emb.Embeddings[0].Embedding),
			Model:      p.Deps.Embedder.Name(),
		}
		// Page bounds are optional: pdfium pages are 1-based, so a zero value
		// means "unknown" — leave the pointers nil rather than store page 0.
		if ch.PageStart > 0 {
			ps := ch.PageStart
			chunk.PageStart = &ps
		}
		if ch.PageEnd > 0 {
			pe := ch.PageEnd
			chunk.PageEnd = &pe
		}
		chunks = append(chunks, chunk)
	}

	if len(chunks) > 0 && p.Deps.Chunks != nil {
		if err := p.Deps.Chunks.UpsertChunks(ctx, in.AssetID, chunks); err != nil {
			return fmt.Errorf("process_pdf %s: store chunks: %w", in.AssetID, err)
		}
	}

	// 4. Thumbnail (non-fatal) + 5. file metadata (best-effort).
	thumbKey := p.uploadThumbnail(ctx, in.AssetID, res.ThumbnailPNG)
	p.persistFile(ctx, in, key, thumbKey, len(data), res.PageCount)

	// 6. Final status. Propagate a write failure so the worker retries rather
	//    than reporting success with the asset stuck in "processing".
	switch {
	case embedAttempts == 0:
		// No embeddable text (empty or image-only PDF) — ready with 0 chunks.
		return p.setStatus(ctx, in.AssetID, models.AssetStatusReady)
	case len(chunks) == 0:
		// Every chunk failed to embed — almost always a transient embedder
		// outage. Retry; give up (failed) only once attempts are exhausted, so
		// the asset never stays stuck in "processing".
		if lastAttempt {
			return p.setStatus(ctx, in.AssetID, models.AssetStatusFailed)
		}
		return fmt.Errorf("process_pdf %s: all %d chunk(s) failed to embed", in.AssetID, embedAttempts)
	case embedFailures > 0:
		return p.setStatus(ctx, in.AssetID, models.AssetStatusPartial)
	default:
		return p.setStatus(ctx, in.AssetID, models.AssetStatusReady)
	}
}

func (p *ProcessPDFProcessor) thumbnailDPI() int {
	if p.Deps.ThumbnailDPI > 0 {
		return p.Deps.ThumbnailDPI
	}
	return thumbnailDPIDefault
}

// setStatus persists the asset status, returning the error so callers can fail
// the job rather than reporting success with an unpersisted status. A nil Assets
// dep (status updates disabled) is a no-op.
func (p *ProcessPDFProcessor) setStatus(ctx context.Context, assetID, status string) error {
	if p.Deps.Assets == nil {
		return nil
	}
	if err := p.Deps.Assets.UpdateStatus(ctx, assetID, status); err != nil {
		return fmt.Errorf("process_pdf %s: set status %s: %w", assetID, status, err)
	}
	return nil
}

func (p *ProcessPDFProcessor) uploadThumbnail(ctx context.Context, assetID string, png []byte) *string {
	if len(png) == 0 {
		return nil
	}
	k := storage.TenantKey(ctx, fmt.Sprintf("assets/%s/thumbnail.png", assetID))
	if _, err := p.Deps.Storage.Upload(ctx, k, bytes.NewReader(png), int64(len(png)), "image/png"); err != nil {
		log.Printf("process_pdf %s: thumbnail upload: %v", assetID, err)
		return nil
	}
	return &k
}

func (p *ProcessPDFProcessor) persistFile(ctx context.Context, in ProcessPDFTask, s3Key string, thumbKey *string, size, pageCount int) {
	if p.Deps.Files == nil {
		return
	}
	fileID, err := models.NewID()
	if err != nil {
		log.Printf("process_pdf %s: new file id: %v", in.AssetID, err)
		return
	}
	var pcPtr *int
	if pageCount > 0 {
		pc := pageCount
		pcPtr = &pc
	}
	if err := p.Deps.Files.Upsert(ctx, &models.AssetFile{
		ID:             fileID,
		AssetID:        in.AssetID,
		OriginalName:   in.OriginalName,
		MimeType:       in.MimeType,
		SizeBytes:      int64(size),
		S3Key:          s3Key,
		ThumbnailS3Key: thumbKey,
		PageCount:      pcPtr,
	}); err != nil {
		log.Printf("process_pdf %s: upsert asset_files: %v", in.AssetID, err)
	}
}

// isTerminalParseErr reports whether a pdf-service Parse error is terminal (the
// PDF is corrupt / encrypted / unsupported) and must not be retried. gRPC
// transport errors (Unavailable, DeadlineExceeded) and Internal are transient.
func isTerminalParseErr(err error) bool {
	st, ok := grpcstatus.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() {
	case codes.InvalidArgument, codes.FailedPrecondition, codes.Unimplemented:
		return true
	default:
		return false
	}
}

// estimateTokens mirrors flows.EstimateTokens (≈4 chars/token). Kept local so
// the queue package doesn't depend on genkit/flows.
func estimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	if t := len(text) / 4; t > 0 {
		return t
	}
	return 1
}

// hasWords mirrors the flows helper: true when text has a non-whitespace word
// character (catches zero-width / non-breaking spaces the embedder can't use).
func hasWords(text string) bool {
	for _, r := range text {
		if r > 32 && !strings.ContainsRune(" \t\n\r\v\f\u00a0\u200b\u200c\u200d\ufeff", r) {
			return true
		}
	}
	return false
}
