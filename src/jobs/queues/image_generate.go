package queues

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/ogen-app/ogen/src/genkit/flows/imagegen"
	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/storage"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// ImageGenerateQueue produces a branded image for a Post via Nano Banana
// (CON-105): load the brand-kit + subject reference bytes from object storage,
// call the imagine flow, store the result, and flip the pending Asset to ready
// with its generation provenance. Runs async so slow (esp. premium) generations
// never block the request path (§5/§8). The pending Asset + enqueue are created
// transactionally by the handler; this worker fills it in.
const ImageGenerateQueue = "image_generate"

// Narrow dependency interfaces — the real repos satisfy these structurally so
// the worker is unit-testable with small fakes (mirrors process_pdf). blobStore
// is shared with the process_pdf worker in this package.

type assetGenerator interface {
	GetByID(ctx context.Context, id string) (*models.Asset, error)
	Update(ctx context.Context, asset *models.Asset) error
	UpdateStatus(ctx context.Context, id, status string) error
}

type assetFileStore interface {
	GetByAssetID(ctx context.Context, assetID string) (*models.AssetFile, error)
	ListByAssetIDs(ctx context.Context, assetIDs []string) (map[string]*models.AssetFile, error)
	Upsert(ctx context.Context, file *models.AssetFile) error
}

type postLinker interface {
	GetByID(ctx context.Context, id string) (*models.Post, error)
	Update(ctx context.Context, post *models.Post) error
}

// imagineFunc is the imagegen flow callback (imagegen.NewImagineCallback).
type imagineFunc func(ctx context.Context, req imagegen.ImagineRequest) (*imagegen.ImagineResponse, error)

// ImageGenDeps bundles the image_generate worker's dependencies (built in
// server.go). A nil Generate or Storage makes the job a no-op / hard-fail,
// mirroring the process_pdf nil-dep handling, so the worker is safe to register
// before it is fully wired.
type ImageGenDeps struct {
	Generate imagineFunc
	Storage  blobStore
	Assets   assetGenerator
	Files    assetFileStore
	Posts    postLinker
}

// ImageGenerateTask carries the pending Asset to fill plus the generation
// request. Reference bytes are NOT in the args (River args are small JSON in
// Postgres) — the worker re-reads them from storage on each attempt.
type ImageGenerateTask struct {
	AssetID          string   `json:"asset_id"`  // the pending Asset to fill
	TenantID         string   `json:"tenant_id"` // scopes every read/write (RLS)
	PostID           string   `json:"post_id"`   // Post to link the result to (optional)
	Platform         string   `json:"platform"`
	SubjectDesc      string   `json:"subject_desc"`
	Instruction      string   `json:"instruction"`
	BrandRefAssetIDs []string `json:"brand_ref_asset_ids"`
	SubjectAssetID   string   `json:"subject_asset_id"` // optional
	AspectRatio      string   `json:"aspect_ratio"`
	Resolution       string   `json:"resolution"`
	Premium          bool     `json:"premium"`
}

func (ImageGenerateTask) Kind() string { return ImageGenerateQueue }

// InsertOpts bounds retries. Transient failures (storage, a model 5xx/429, the
// key not yet configured) retry with backoff; terminal ones (an unsupported
// size) short-circuit to "failed" inside process.
func (ImageGenerateTask) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

type ImageGenerateProcessor struct {
	river.WorkerDefaults[ImageGenerateTask]
	Deps ImageGenDeps
}

func init() {
	register(func(w *river.Workers, d Deps) {
		river.AddWorker(w, &ImageGenerateProcessor{Deps: d.ImageGen})
	})
}

// Work scopes the job to the asset's tenant and processes it. lastAttempt lets
// process() flip an otherwise-retryable outage to a terminal "failed" instead
// of abandoning the asset in "processing".
func (p *ImageGenerateProcessor) Work(ctx context.Context, job *river.Job[ImageGenerateTask]) error {
	ctx = WithJobRequestID(ctx, job.JobRow)
	ctx = tenantctx.With(ctx, job.Args.TenantID)
	return p.process(ctx, job.Args, job.Attempt >= job.MaxAttempts)
}

// Timeout is generous — a premium (Thinking) generation can take many seconds
// (§10) — and longer for the premium tier than the default.
func (p *ImageGenerateProcessor) Timeout(job *river.Job[ImageGenerateTask]) time.Duration {
	if job.Args.Premium {
		return 5 * time.Minute
	}
	return 3 * time.Minute
}

func (p *ImageGenerateProcessor) process(ctx context.Context, in ImageGenerateTask, lastAttempt bool) error {
	if p.Deps.Generate == nil || p.Deps.Storage == nil || p.Deps.Assets == nil || p.Deps.Files == nil {
		slog.WarnContext(ctx, "image generation not configured", logging.AttrComponent, "jobs.image_generate", "asset_id", in.AssetID)
		return nil
	}

	// Idempotency guard: a prior attempt that already produced the image left the
	// asset "ready". Re-running would regenerate (new cost + new bytes), so stop.
	asset, err := p.Deps.Assets.GetByID(ctx, in.AssetID)
	if err != nil {
		return fmt.Errorf("image_generate %s: load asset: %w", in.AssetID, err)
	}
	if asset == nil {
		slog.WarnContext(ctx, "asset gone; nothing to generate", logging.AttrComponent, "jobs.image_generate", "asset_id", in.AssetID)
		return nil
	}
	if asset.Status == models.AssetStatusReady {
		return nil
	}

	if err := p.Deps.Assets.UpdateStatus(ctx, in.AssetID, models.AssetStatusProcessing); err != nil {
		return fmt.Errorf("image_generate %s: set processing: %w", in.AssetID, err)
	}

	// 1. Load reference bytes from storage (brand-first order preserved).
	brandRefs, err := p.loadBrandRefs(ctx, in.BrandRefAssetIDs)
	if err != nil {
		return fmt.Errorf("image_generate %s: load brand refs: %w", in.AssetID, err)
	}
	var subject *imagegen.ReferenceImage
	if in.SubjectAssetID != "" {
		subject, err = p.loadRef(ctx, in.SubjectAssetID)
		if err != nil {
			return fmt.Errorf("image_generate %s: load subject: %w", in.AssetID, err)
		}
	}

	// 2. Generate. Classify the error: a bad request is terminal; an outage
	//    retries (and fails only once attempts are exhausted).
	res, err := p.Deps.Generate(ctx, imagegen.ImagineRequest{
		Platform:    in.Platform,
		SubjectDesc: in.SubjectDesc,
		Instruction: in.Instruction,
		BrandRefs:   brandRefs,
		Subject:     subject,
		AspectRatio: in.AspectRatio,
		Resolution:  in.Resolution,
		Premium:     in.Premium,
	})
	if err != nil {
		var ve *imagegen.ValidationError
		if errors.As(err, &ve) {
			// Bad request (e.g. unsupported size) — never fixes on retry.
			return p.fail(ctx, in.AssetID, "invalid request: "+ve.Msg)
		}
		if lastAttempt {
			return p.fail(ctx, in.AssetID, "generation failed: "+err.Error())
		}
		return fmt.Errorf("image_generate %s: generate: %w", in.AssetID, err)
	}

	// 3. Store the image + its asset_file row.
	ext := extForImageMIME(res.MIMEType)
	key := storage.TenantKey(ctx, fmt.Sprintf("assets/%s/generated.%s", in.AssetID, ext))
	if _, err := p.Deps.Storage.Upload(ctx, key, bytes.NewReader(res.Image), int64(len(res.Image)), res.MIMEType); err != nil {
		return fmt.Errorf("image_generate %s: upload image: %w", in.AssetID, err)
	}
	if err := p.persistFile(ctx, in.AssetID, key, res.MIMEType, len(res.Image), ext); err != nil {
		return err
	}

	// 4. Flip the Asset to ready with its generation provenance (§5/§7/§10).
	imgType := models.AssetTypeImage
	asset.Status = models.AssetStatusReady
	asset.Type = &imgType
	asset.AIGenerated = true
	asset.Generation = &models.AssetGeneration{
		Model:       res.Model,
		AspectRatio: res.AspectRatio,
		Resolution:  res.Resolution,
		// Nano Banana outputs always carry SynthID + C2PA content credentials.
		SynthID: true,
		C2PA:    true,
	}
	if err := p.Deps.Assets.Update(ctx, asset); err != nil {
		return fmt.Errorf("image_generate %s: mark ready: %w", in.AssetID, err)
	}

	// 5. Link the result to the Post (best-effort — the asset is already ready
	//    and usable; a link failure must not trigger a regeneration).
	p.linkPost(ctx, in.PostID, in.AssetID)

	slog.InfoContext(ctx, "image generated", logging.AttrComponent, "jobs.image_generate",
		"asset_id", in.AssetID, "model", res.Model, "size", res.AspectRatio+"@"+res.Resolution, "bytes", len(res.Image))
	return nil
}

// loadBrandRefs loads the brand-kit reference bytes, preserving the requested
// order (brand-first is the cacheable prefix, §6). A missing/fileless asset is
// skipped with a warning rather than failing the whole generation.
func (p *ImageGenerateProcessor) loadBrandRefs(ctx context.Context, assetIDs []string) ([]imagegen.ReferenceImage, error) {
	if len(assetIDs) == 0 {
		return nil, nil
	}
	files, err := p.Deps.Files.ListByAssetIDs(ctx, assetIDs)
	if err != nil {
		return nil, err
	}
	refs := make([]imagegen.ReferenceImage, 0, len(assetIDs))
	for _, id := range assetIDs {
		f := files[id]
		if f == nil {
			slog.WarnContext(ctx, "brand ref has no file; skipping", logging.AttrComponent, "jobs.image_generate", "asset_id", id)
			continue
		}
		data, err := p.download(ctx, f.S3Key)
		if err != nil {
			return nil, fmt.Errorf("brand ref %s: %w", id, err)
		}
		refs = append(refs, imagegen.ReferenceImage{MIMEType: f.MimeType, Data: data})
	}
	return refs, nil
}

func (p *ImageGenerateProcessor) loadRef(ctx context.Context, assetID string) (*imagegen.ReferenceImage, error) {
	f, err := p.Deps.Files.GetByAssetID(ctx, assetID)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, fmt.Errorf("asset %s has no file", assetID)
	}
	data, err := p.download(ctx, f.S3Key)
	if err != nil {
		return nil, err
	}
	return &imagegen.ReferenceImage{MIMEType: f.MimeType, Data: data}, nil
}

func (p *ImageGenerateProcessor) download(ctx context.Context, key string) ([]byte, error) {
	rc, err := p.Deps.Storage.Download(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", key, err)
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(rc)
}

// persistFile upserts the generated image's asset_file row. Upsert conflicts on
// asset_id, so it is idempotent across retries.
func (p *ImageGenerateProcessor) persistFile(ctx context.Context, assetID, s3Key, mime string, size int, ext string) error {
	fileID, err := models.NewID()
	if err != nil {
		return fmt.Errorf("image_generate %s: new file id: %w", assetID, err)
	}
	if err := p.Deps.Files.Upsert(ctx, &models.AssetFile{
		ID:           fileID,
		AssetID:      assetID,
		OriginalName: "generated." + ext,
		MimeType:     mime,
		SizeBytes:    int64(size),
		S3Key:        s3Key,
	}); err != nil {
		return fmt.Errorf("image_generate %s: upsert asset_files: %w", assetID, err)
	}
	return nil
}

// linkPost appends the generated asset to the Post's used_asset_ids. Best-effort:
// a failure is logged, not returned, so it never triggers a regeneration.
func (p *ImageGenerateProcessor) linkPost(ctx context.Context, postID, assetID string) {
	if postID == "" || p.Deps.Posts == nil {
		return
	}
	post, err := p.Deps.Posts.GetByID(ctx, postID)
	if err != nil || post == nil {
		slog.WarnContext(ctx, "link post: load failed", logging.AttrComponent, "jobs.image_generate", "post_id", postID, logging.AttrError, err)
		return
	}
	for _, id := range post.UsedAssetIDs {
		if id == assetID {
			return // already linked
		}
	}
	post.UsedAssetIDs = append(post.UsedAssetIDs, assetID)
	if err := p.Deps.Posts.Update(ctx, post); err != nil {
		slog.WarnContext(ctx, "link post: update failed", logging.AttrComponent, "jobs.image_generate", "post_id", postID, logging.AttrError, err)
	}
}

// fail marks the asset failed (reason logged; the Asset has no error column yet,
// so surfacing it to the UI is a follow-up) and returns nil so the terminal
// outcome is not itself retried.
func (p *ImageGenerateProcessor) fail(ctx context.Context, assetID, reason string) error {
	slog.WarnContext(ctx, "image generation failed", logging.AttrComponent, "jobs.image_generate", "asset_id", assetID, "reason", reason)
	if err := p.Deps.Assets.UpdateStatus(ctx, assetID, models.AssetStatusFailed); err != nil {
		return fmt.Errorf("image_generate %s: set failed: %w", assetID, err)
	}
	return nil
}

func extForImageMIME(mime string) string {
	switch mime {
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/webp":
		return "webp"
	default:
		return "png"
	}
}
