package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/storage"
	"github.com/ogen-app/ogen/src/tenantctx"
)

const (
	maxMarkdownUploadSize = 10 << 20 // 10 MB
	maxPDFUploadSize      = 50 << 20 // 50 MB
)

// PDFIngestEnqueuer enqueues a PDF-ingestion job in the caller's transaction
// (CON-103). Implemented by *queues.Enqueuer; a narrow interface here keeps the
// handler off the jobs package.
type PDFIngestEnqueuer interface {
	EnqueueProcessPDFTx(ctx context.Context, tx *sql.Tx, assetID, tenantID, originalName, mimeType string) error
}

type AssetsHandler struct {
	repo     repository.AssetRepository
	fileRepo repository.AssetFileRepository
	storage  storage.Storage
	db       *bun.DB
	auth     fiber.Handler

	// onSave triggers async embedding for text-based Asset saves (JSON create/update + MD upload).
	onSave func(assetID, title, content, tenantID string)
	// pdfJobs enqueues PDF ingestion (CON-103). Nil disables it (the asset is
	// created but left pending).
	pdfJobs PDFIngestEnqueuer
}

func NewAssetsHandler(
	repo repository.AssetRepository,
	fileRepo repository.AssetFileRepository,
	store storage.Storage,
	db *bun.DB,
	pdfJobs PDFIngestEnqueuer,
	auth fiber.Handler,
	onSave func(assetID, title, content, tenantID string),
) *AssetsHandler {
	return &AssetsHandler{
		repo:     repo,
		fileRepo: fileRepo,
		storage:  store,
		db:       db,
		auth:     auth,
		onSave:   onSave,
		pdfJobs:  pdfJobs,
	}
}

func (h *AssetsHandler) Register(app *fiber.App) {
	g := app.Group("/api/content-bank/assets")
	g.Get("/", h.auth, h.List)
	g.Post("/", h.auth, h.Create)
	g.Post("/upload", h.auth, h.Upload)
	g.Get("/:id", h.auth, h.Get)
	g.Put("/:id", h.auth, h.Update)
	g.Delete("/:id", h.auth, h.Delete)
}

type createAssetRequest struct {
	Title   string             `json:"title"   validate:"required"`
	Content string             `json:"content" validate:"required"`
	TagIDs  models.StringSlice `json:"tag_ids"`
}

type updateAssetRequest struct {
	Title   string             `json:"title"   validate:"required"`
	Content string             `json:"content" validate:"required"`
	TagIDs  models.StringSlice `json:"tag_ids"`
}

// List godoc
// @Summary      List assets
// @Description  Returns all content bank assets ordered by creation date.
// @Tags         content-bank
// @Produce      json
// @Security     CookieAuth
// @Success      200  {array}   models.Asset
// @Failure      401  {object}  map[string]string
// @Router       /api/content-bank/assets [get]
func (h *AssetsHandler) List(c *fiber.Ctx) error {
	assets, err := h.repo.List(c.Context())
	if err != nil {
		return err
	}
	for i := range assets {
		h.decorateFile(&assets[i])
	}
	return c.JSON(assets)
}

// decorateFile fills File.ThumbnailURL from its s3_key using the public
// storage URL, when both are present.
func (h *AssetsHandler) decorateFile(asset *models.Asset) {
	if asset == nil || asset.File == nil || h.storage == nil {
		return
	}
	if asset.File.ThumbnailS3Key == nil || *asset.File.ThumbnailS3Key == "" {
		return
	}
	u := h.storage.PublicURL(*asset.File.ThumbnailS3Key)
	asset.File.ThumbnailURL = &u
}

// Create godoc
// @Summary      Create asset
// @Description  Creates a new content bank asset. The created_by field is set from the authenticated session.
// @Tags         content-bank
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body      createAssetRequest  true  "Asset payload"
// @Success      201   {object}  models.Asset
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Router       /api/content-bank/assets [post]
func (h *AssetsHandler) Create(c *fiber.Ctx) error {
	var req createAssetRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	session := c.Locals("session").(*models.Session)

	id, err := models.NewID()
	if err != nil {
		return err
	}

	asset := &models.Asset{
		ID:        id,
		Title:     req.Title,
		Content:   req.Content,
		Status:    models.AssetStatusPending,
		TagIDs:    nullSlice(req.TagIDs),
		Tags:      []models.Tag{},
		CreatedBy: session.UserID,
	}
	if err := h.repo.Create(c.Context(), asset); err != nil {
		return err
	}

	if h.onSave != nil {
		tid, _ := tenantctx.From(c.Context())
		go h.onSave(asset.ID, asset.Title, asset.Content, tid)
	}

	return c.Status(fiber.StatusCreated).JSON(asset)
}

// uploadResult is the per-file outcome in a batch markdown upload.
type uploadResult struct {
	Filename string        `json:"filename"`
	AssetID  string        `json:"asset_id,omitempty"`
	Status   string        `json:"status"` // "created" | "failed"
	Error    string        `json:"error,omitempty"`
	Asset    *models.Asset `json:"asset,omitempty"`
}

// Upload godoc
// @Summary      Upload Markdown or PDF file(s)
// @Description  Accepts one or more `.md` (max 10 MB) or `.pdf` (max 50 MB) files. Markdown files are converted to BlockNote JSON synchronously; PDFs are uploaded to object storage and processed asynchronously (text extraction, page-aware chunking, embedding, thumbnail). Files are processed independently — one failure does not block others.
// @Tags         content-bank
// @Accept       multipart/form-data
// @Produce      json
// @Security     CookieAuth
// @Param        files  formData  file  true  "Markdown or PDF file(s)"
// @Success      201    {object}  map[string]any
// @Failure      400    {object}  map[string]string
// @Failure      401    {object}  map[string]string
// @Router       /api/content-bank/assets/upload [post]
func (h *AssetsHandler) Upload(c *fiber.Ctx) error {
	form, err := c.MultipartForm()
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "expected multipart/form-data")
	}
	files := form.File["files"]
	if len(files) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "no files provided (use field 'files')")
	}

	session := c.Locals("session").(*models.Session)

	results := make([]uploadResult, 0, len(files))
	for _, fh := range files {
		res := uploadResult{Filename: fh.Filename}

		switch detectUploadKind(fh.Filename) {
		case uploadKindMarkdown:
			res = h.processMarkdownUpload(c, fh, session)
		case uploadKindPDF:
			res = h.processPDFUpload(c, fh, session)
		default:
			res.Status = "failed"
			res.Error = "only .md and .pdf files are accepted"
		}
		results = append(results, res)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"results": results})
}

type uploadKind int

const (
	uploadKindUnknown uploadKind = iota
	uploadKindMarkdown
	uploadKindPDF
)

func detectUploadKind(filename string) uploadKind {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".md":
		return uploadKindMarkdown
	case ".pdf":
		return uploadKindPDF
	}
	return uploadKindUnknown
}

func (h *AssetsHandler) processMarkdownUpload(c *fiber.Ctx, fh *multipart.FileHeader, session *models.Session) uploadResult {
	res := uploadResult{Filename: fh.Filename}

	if fh.Size > maxMarkdownUploadSize {
		res.Status = "failed"
		res.Error = fmt.Sprintf("file exceeds maximum size of %d MB", maxMarkdownUploadSize>>20)
		return res
	}

	raw, err := readFormFile(fh, maxMarkdownUploadSize)
	if err != nil {
		res.Status = "failed"
		res.Error = "could not read file"
		return res
	}

	content := string(raw)
	title := strings.TrimSuffix(filepath.Base(fh.Filename), filepath.Ext(fh.Filename))

	id, err := models.NewID()
	if err != nil {
		res.Status = "failed"
		res.Error = "could not generate id"
		return res
	}
	mdType := models.AssetTypeMarkdown
	asset := &models.Asset{
		ID:        id,
		Title:     title,
		Content:   content,
		Status:    models.AssetStatusPending,
		Type:      &mdType,
		TagIDs:    models.StringSlice{},
		Tags:      []models.Tag{},
		CreatedBy: session.UserID,
	}
	if err := h.repo.Create(c.Context(), asset); err != nil {
		res.Status = "failed"
		res.Error = "could not create asset"
		return res
	}

	if h.onSave != nil {
		tid, _ := tenantctx.From(c.Context())
		go h.onSave(asset.ID, asset.Title, asset.Content, tid)
	}

	res.AssetID = asset.ID
	res.Status = "created"
	res.Asset = asset
	return res
}

func (h *AssetsHandler) processPDFUpload(c *fiber.Ctx, fh *multipart.FileHeader, session *models.Session) uploadResult {
	res := uploadResult{Filename: fh.Filename}

	if fh.Size > maxPDFUploadSize {
		res.Status = "failed"
		res.Error = fmt.Sprintf("file exceeds maximum size of %d MB", maxPDFUploadSize>>20)
		return res
	}

	raw, err := readFormFile(fh, maxPDFUploadSize)
	if err != nil {
		res.Status = "failed"
		res.Error = "could not read file"
		return res
	}
	if len(raw) < 4 || string(raw[:4]) != "%PDF" {
		res.Status = "failed"
		res.Error = "file is not a valid PDF"
		return res
	}

	title := strings.TrimSuffix(filepath.Base(fh.Filename), filepath.Ext(fh.Filename))
	id, err := models.NewID()
	if err != nil {
		res.Status = "failed"
		res.Error = "could not generate id"
		return res
	}
	pdfType := models.AssetTypePDF
	asset := &models.Asset{
		ID:        id,
		Title:     title,
		Content:   "[]",
		Status:    models.AssetStatusPending,
		Type:      &pdfType,
		TagIDs:    models.StringSlice{},
		Tags:      []models.Tag{},
		CreatedBy: session.UserID,
	}

	ctx := c.Context()

	// PDF ingestion (CON-103) needs object storage — the worker re-reads the PDF
	// from it on each attempt — plus the job enqueuer. Without them, create the
	// asset but skip processing (it stays pending).
	if h.storage == nil || h.pdfJobs == nil || h.db == nil {
		if err := h.repo.Create(ctx, asset); err != nil {
			res.Status = "failed"
			res.Error = "could not create asset"
			return res
		}
		slog.WarnContext(c.Context(), "pdf ingestion disabled; asset left pending", logging.AttrComponent, "assets", "asset_id", asset.ID)
		res.AssetID = asset.ID
		res.Status = "created"
		res.Asset = asset
		return res
	}

	// 1. Store original.pdf BEFORE enqueue so the worker can re-read it on each
	//    attempt (the 50 MB bytes can't ride in the River job args).
	key := storage.TenantKey(ctx, fmt.Sprintf("assets/%s/original.pdf", asset.ID))
	if _, err := h.storage.Upload(ctx, key, bytes.NewReader(raw), int64(len(raw)), "application/pdf"); err != nil {
		res.Status = "failed"
		res.Error = "could not store pdf"
		return res
	}

	// 2. Insert the asset and enqueue the ingestion job atomically (transactional
	//    outbox): a committed asset always has a job, a rolled-back one never does.
	if err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(asset).Exec(ctx); err != nil {
			return err
		}
		return h.pdfJobs.EnqueueProcessPDFTx(ctx, tx.Tx, asset.ID, session.TenantID, fh.Filename, "application/pdf")
	}); err != nil {
		// The transaction rolled back, so the asset/job never persisted — delete
		// the original.pdf uploaded above (same key) so it isn't left orphaned in
		// the bucket. Best-effort: the failed upload is what the caller acts on.
		_ = h.storage.Delete(ctx, key)
		res.Status = "failed"
		res.Error = "could not create asset"
		return res
	}

	res.AssetID = asset.ID
	res.Status = "created"
	res.Asset = asset
	return res
}

func readFormFile(fh *multipart.FileHeader, limit int64) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit+1))
}

// Get godoc
// @Summary      Get asset
// @Description  Returns a single content bank asset by Sqid.
// @Tags         content-bank
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "Asset Sqid"
// @Success      200  {object}  models.Asset
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/content-bank/assets/{id} [get]
func (h *AssetsHandler) Get(c *fiber.Ctx) error {
	asset, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "asset not found")
		}
		return err
	}
	h.decorateFile(asset)
	return c.JSON(asset)
}

// Update godoc
// @Summary      Update asset
// @Description  Updates title and content of an existing asset.
// @Tags         content-bank
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string             true  "Asset Sqid"
// @Param        body  body      updateAssetRequest true  "Asset payload"
// @Success      200   {object}  models.Asset
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Router       /api/content-bank/assets/{id} [put]
func (h *AssetsHandler) Update(c *fiber.Ctx) error {
	var req updateAssetRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	asset, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "asset not found")
		}
		return err
	}

	// Detect whether the embedding inputs (title or content) actually changed,
	// so we don't re-embed an asset when only a tag was toggled.
	embedInputChanged := asset.Title != req.Title || asset.Content != req.Content

	asset.Title = req.Title
	asset.Content = req.Content
	asset.TagIDs = nullSlice(req.TagIDs)
	asset.UpdatedAt = time.Now().UTC()

	if err := h.repo.Update(c.Context(), asset); err != nil {
		return err
	}

	if h.onSave != nil && embedInputChanged {
		tid, _ := tenantctx.From(c.Context())
		go h.onSave(asset.ID, asset.Title, asset.Content, tid)
	}

	h.decorateFile(asset)
	return c.JSON(asset)
}

// Delete godoc
// @Summary      Delete asset
// @Description  Deletes a content bank asset by Sqid.
// @Tags         content-bank
// @Security     CookieAuth
// @Param        id   path  string  true  "Asset Sqid"
// @Success      204
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/content-bank/assets/{id} [delete]
func (h *AssetsHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")

	// Collect S3 keys to clean up before deleting the row; DB cascade will
	// drop the asset_files row when we delete the asset, so capture now.
	var keysToDelete []string
	if h.fileRepo != nil {
		if f, err := h.fileRepo.GetByAssetID(c.Context(), id); err == nil && f != nil {
			if f.S3Key != "" {
				keysToDelete = append(keysToDelete, f.S3Key)
			}
			if f.ThumbnailS3Key != nil && *f.ThumbnailS3Key != "" {
				keysToDelete = append(keysToDelete, *f.ThumbnailS3Key)
			}
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}

	deleted, err := h.repo.Delete(c.Context(), id)
	if err != nil {
		return err
	}
	if !deleted {
		return fiber.NewError(fiber.StatusNotFound, "asset not found")
	}

	// Best-effort S3 cleanup. Logged but not surfaced — the row is gone
	// and orphaned objects are tolerable.
	if h.storage != nil {
		for _, k := range keysToDelete {
			if err := h.storage.Delete(c.Context(), k); err != nil {
				// Fiber has no handler-level logger dependency here; swallow.
				_ = err
			}
		}
	}

	return c.SendStatus(fiber.StatusNoContent)
}
