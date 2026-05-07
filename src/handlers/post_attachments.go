package handlers

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/platforms"
	"github.com/content-control-center/app/src/repository"
	"github.com/content-control-center/app/src/storage"
	"github.com/content-control-center/app/src/storage/imageprobe"
)

const (
	// maxAttachmentUploadBytes is the hard upper bound enforced by the
	// upload endpoint regardless of any platform's smaller per-image
	// cap. 50 MB matches the spec default in CON-73 §2.2.
	maxAttachmentUploadBytes int64 = 50 << 20
)

// PresignedURLTTL controls how long pre-signed GET URLs returned in
// API responses stay valid. Short window keeps stale URLs out of
// caches; long enough that the editor UI has plenty of time to load
// the image once. Exposed as a var so integration tests can shrink
// the window to assert expiry behaviour without sleeping for minutes.
var PresignedURLTTL = 15 * time.Minute

// PostAttachmentsHandler exposes the upload/list/reorder/delete API
// for image attachments bound to a Post (CON-73). All mutations are
// blocked once the parent post is `published`.
type PostAttachmentsHandler struct {
	repo     repository.PostAttachmentRepository
	postRepo repository.PostRepository
	storage  storage.Storage
	auth     fiber.Handler
}

func NewPostAttachmentsHandler(
	repo repository.PostAttachmentRepository,
	postRepo repository.PostRepository,
	store storage.Storage,
	auth fiber.Handler,
) *PostAttachmentsHandler {
	return &PostAttachmentsHandler{repo: repo, postRepo: postRepo, storage: store, auth: auth}
}

func (h *PostAttachmentsHandler) Register(app *fiber.App) {
	g := app.Group("/api/posts/:post_id/attachments", h.auth)
	g.Get("/", h.List)
	g.Post("/", h.Upload)
	g.Get("/:id", h.Get)
	g.Patch("/:id", h.Reorder)
	g.Delete("/:id", h.Delete)
}

// attachmentResponse wraps a single attachment with the soft-validation
// block per CON-73 §2.4. ValidationErrors is empty when the attachment
// passes every rule for the post's currently-selected platform.
type attachmentResponse struct {
	*models.PostAttachment
	PlatformValidation []platforms.ValidationError `json:"platform_validation"`
}

// listResponse mirrors attachmentResponse but for list endpoints, with
// the post-level rules (e.g. count cap) surfaced once at the top.
type listResponse struct {
	Attachments        []attachmentResponse        `json:"attachments"`
	PlatformValidation []platforms.ValidationError `json:"platform_validation"`
}

// loadPostOrErr fetches the post and returns 404 if missing. Mutating
// callers should additionally check terminalForMutations.
func (h *PostAttachmentsHandler) loadPostOrErr(c *fiber.Ctx) (*models.Post, error) {
	postID := c.Params("post_id")
	post, err := h.postRepo.GetByID(c.Context(), postID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fiber.NewError(fiber.StatusNotFound, "post not found")
		}
		return nil, err
	}
	return post, nil
}

// terminalForMutations reports whether the post is in a state that
// freezes its attachments. Per CON-73 §2.1 / §2.7, mutations are
// blocked once the post is published.
func terminalForMutations(s models.PostStatus) bool {
	return s == models.PostStatusPublished
}

// hydratePresigned fills att.PresignedURL when storage is configured.
// Errors are swallowed — a missing presigned URL still leaves callers
// with the metadata, and the binary is never the source of truth.
func (h *PostAttachmentsHandler) hydratePresigned(c *fiber.Ctx, att *models.PostAttachment) {
	if h.storage == nil || att == nil || att.S3Key == "" {
		return
	}
	url, err := h.storage.PresignedGetURL(c.Context(), att.S3Key, PresignedURLTTL)
	if err == nil {
		att.PresignedURL = url
	}
}

// List godoc
// @Summary      List post attachments
// @Description  Returns the post's attachments ordered by position, with a
// @Description  soft pre-check (`platform_validation`) per CON-73 §2.4 that
// @Description  surfaces any rule failure for the post's current target
// @Description  platform. Each attachment carries a short-lived presigned
// @Description  GET URL when object storage is configured.
// @Tags         post-attachments
// @Produce      json
// @Security     CookieAuth
// @Param        post_id  path      string  true  "Post Sqid"
// @Success      200      {object}  listResponse
// @Failure      401      {object}  map[string]string
// @Failure      404      {object}  map[string]string
// @Router       /api/posts/{post_id}/attachments [get]
func (h *PostAttachmentsHandler) List(c *fiber.Ctx) error {
	post, err := h.loadPostOrErr(c)
	if err != nil {
		return err
	}
	atts, err := h.repo.ListByPostID(c.Context(), post.ID)
	if err != nil {
		return err
	}

	out := listResponse{
		Attachments:        make([]attachmentResponse, 0, len(atts)),
		PlatformValidation: platforms.ValidatePostAttachments(atts, post.Platform),
	}
	for i := range atts {
		h.hydratePresigned(c, &atts[i])
		out.Attachments = append(out.Attachments, attachmentResponse{
			PostAttachment:     &atts[i],
			PlatformValidation: platforms.ValidateAttachment(&atts[i], post.Platform),
		})
	}
	return c.JSON(out)
}

// Get godoc
// @Summary      Get post attachment
// @Description  Returns metadata for a single attachment plus its soft
// @Description  validation block and a short-lived presigned GET URL.
// @Tags         post-attachments
// @Produce      json
// @Security     CookieAuth
// @Param        post_id  path      string  true  "Post Sqid"
// @Param        id       path      string  true  "Attachment id"
// @Success      200      {object}  attachmentResponse
// @Failure      401      {object}  map[string]string
// @Failure      404      {object}  map[string]string
// @Router       /api/posts/{post_id}/attachments/{id} [get]
func (h *PostAttachmentsHandler) Get(c *fiber.Ctx) error {
	post, err := h.loadPostOrErr(c)
	if err != nil {
		return err
	}
	att, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "attachment not found")
		}
		return err
	}
	if att.PostID != post.ID {
		return fiber.NewError(fiber.StatusNotFound, "attachment not found")
	}
	h.hydratePresigned(c, att)
	return c.JSON(attachmentResponse{
		PostAttachment:     att,
		PlatformValidation: platforms.ValidateAttachment(att, post.Platform),
	})
}

// Upload godoc
// @Summary      Upload a post attachment
// @Description  Accepts a single image file via multipart/form-data under
// @Description  the field `file`. The file is decoded server-side to validate
// @Description  the MIME (JPEG/PNG/WebP/GIF), then streamed to object storage.
// @Description  Hard cap is 50 MB regardless of any platform's smaller per-image
// @Description  limit; per-platform caps are surfaced as soft warnings in the
// @Description  response.
// @Tags         post-attachments
// @Accept       multipart/form-data
// @Produce      json
// @Security     CookieAuth
// @Param        post_id  path      string  true  "Post Sqid"
// @Param        file     formData  file    true  "Image file"
// @Success      201      {object}  attachmentResponse
// @Failure      400      {object}  map[string]string
// @Failure      401      {object}  map[string]string
// @Failure      404      {object}  map[string]string
// @Failure      409      {object}  map[string]string  "post is in a terminal publishing state"
// @Failure      415      {object}  map[string]string
// @Failure      503      {object}  map[string]string
// @Router       /api/posts/{post_id}/attachments [post]
func (h *PostAttachmentsHandler) Upload(c *fiber.Ctx) error {
	if h.storage == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "storage not configured")
	}

	post, err := h.loadPostOrErr(c)
	if err != nil {
		return err
	}
	if terminalForMutations(post.Status) {
		return fiber.NewError(fiber.StatusConflict, "post is in a terminal publishing state and its attachments are immutable")
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "file is required")
	}
	if fh.Size > maxAttachmentUploadBytes {
		return fiber.NewError(
			fiber.StatusBadRequest,
			fmt.Sprintf("file exceeds upload limit of %d MB", maxAttachmentUploadBytes>>20),
		)
	}

	f, err := fh.Open()
	if err != nil {
		return fmt.Errorf("post_attachments: open upload: %w", err)
	}
	defer f.Close()

	probe, data, err := imageprobe.Probe(f, maxAttachmentUploadBytes)
	if err != nil {
		if errors.Is(err, imageprobe.ErrUnsupportedMIME) {
			return fiber.NewError(fiber.StatusUnsupportedMediaType, err.Error())
		}
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	session := c.Locals("session").(*models.Session)

	id, err := models.NewID()
	if err != nil {
		return err
	}
	key := filepath.Base(fmt.Sprintf("post-attachments/%s/%s%s", post.ID, id, probe.Extension))
	// Strip post-attachments/ prefix from filepath.Base; reattach.
	key = "post-attachments/" + post.ID + "/" + id + probe.Extension

	if _, err := h.storage.Upload(c.Context(), key, bytes.NewReader(data), probe.Size, probe.MIME); err != nil {
		return fmt.Errorf("post_attachments: storage upload: %w", err)
	}

	att := &models.PostAttachment{
		ID:             id,
		PostID:         post.ID,
		MimeType:       probe.MIME,
		SizeBytes:      probe.Size,
		Width:          probe.Width,
		Height:         probe.Height,
		IsAnimated:     probe.IsAnimated,
		ChecksumSHA256: probe.SHA256,
		S3Key:          key,
		CreatedBy:      session.UserID,
	}
	// CreateAtNextPosition assigns att.Position atomically, eliminating
	// the read-then-insert race that NextPosition+Create exposed under
	// concurrent uploads to the same post.
	if err := h.repo.CreateAtNextPosition(c.Context(), att); err != nil {
		// Clean up the orphan object — the metadata row is what makes
		// the attachment discoverable; without it, the bytes are dead
		// weight in the bucket (CON-73 §2.2 transactional rollback).
		_ = h.storage.Delete(c.Context(), key)
		return err
	}

	h.hydratePresigned(c, att)
	return c.Status(fiber.StatusCreated).JSON(attachmentResponse{
		PostAttachment:     att,
		PlatformValidation: platforms.ValidateAttachment(att, post.Platform),
	})
}

type reorderRequest struct {
	Position *int `json:"position" validate:"required"`
}

// Reorder godoc
// @Summary      Reorder a post attachment
// @Description  Updates the `position` of an attachment. Last-write-wins;
// @Description  no optimistic concurrency token (CON-73 §6 MVP decision).
// @Tags         post-attachments
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        post_id  path      string          true  "Post Sqid"
// @Param        id       path      string          true  "Attachment id"
// @Param        body     body      reorderRequest  true  "New position"
// @Success      200      {object}  attachmentResponse
// @Failure      400      {object}  map[string]string
// @Failure      401      {object}  map[string]string
// @Failure      404      {object}  map[string]string
// @Failure      409      {object}  map[string]string
// @Router       /api/posts/{post_id}/attachments/{id} [patch]
func (h *PostAttachmentsHandler) Reorder(c *fiber.Ctx) error {
	post, err := h.loadPostOrErr(c)
	if err != nil {
		return err
	}
	if terminalForMutations(post.Status) {
		return fiber.NewError(fiber.StatusConflict, "post is in a terminal publishing state and its attachments are immutable")
	}

	att, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "attachment not found")
		}
		return err
	}
	if att.PostID != post.ID {
		return fiber.NewError(fiber.StatusNotFound, "attachment not found")
	}

	var req reorderRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}
	if *req.Position < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "position must be non-negative")
	}

	if err := h.repo.UpdatePosition(c.Context(), att.ID, *req.Position); err != nil {
		return err
	}

	updated, err := h.repo.GetByID(c.Context(), att.ID)
	if err != nil {
		return err
	}
	h.hydratePresigned(c, updated)
	return c.JSON(attachmentResponse{
		PostAttachment:     updated,
		PlatformValidation: platforms.ValidateAttachment(updated, post.Platform),
	})
}

// Delete godoc
// @Summary      Delete a post attachment
// @Description  Removes an attachment row and its S3 object. If the S3
// @Description  delete fails, the metadata row is retained and 502 is
// @Description  returned so the caller can retry (CON-73 §2.7).
// @Tags         post-attachments
// @Security     CookieAuth
// @Param        post_id  path  string  true  "Post Sqid"
// @Param        id       path  string  true  "Attachment id"
// @Success      204
// @Failure      401      {object}  map[string]string
// @Failure      404      {object}  map[string]string
// @Failure      409      {object}  map[string]string
// @Failure      502      {object}  map[string]string
// @Router       /api/posts/{post_id}/attachments/{id} [delete]
func (h *PostAttachmentsHandler) Delete(c *fiber.Ctx) error {
	post, err := h.loadPostOrErr(c)
	if err != nil {
		return err
	}
	if terminalForMutations(post.Status) {
		return fiber.NewError(fiber.StatusConflict, "post is in a terminal publishing state and its attachments are immutable")
	}

	att, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "attachment not found")
		}
		return err
	}
	if att.PostID != post.ID {
		return fiber.NewError(fiber.StatusNotFound, "attachment not found")
	}

	// Delete S3 first so we never end up with a row pointing at a
	// missing object. If S3 delete fails, the row stays and the caller
	// retries.
	if h.storage != nil && att.S3Key != "" {
		if err := h.storage.Delete(c.Context(), att.S3Key); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "failed to delete object from storage; please retry")
		}
	}

	deleted, err := h.repo.Delete(c.Context(), att.ID)
	if err != nil {
		return err
	}
	if !deleted {
		return fiber.NewError(fiber.StatusNotFound, "attachment not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
