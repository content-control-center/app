package handlers

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/jobs/queues"
	"github.com/ogen-app/ogen/src/models"
)

// ImageGenEnqueuer enqueues the image_generate job in a transaction so it
// commits atomically with the pending Asset insert (CON-105).
type ImageGenEnqueuer interface {
	EnqueueImageGenerateTx(ctx context.Context, tx *sql.Tx, task queues.ImageGenerateTask) error
}

// SetImageGenerator wires image generation (CON-105). enq enqueues the job;
// ready reports whether a gemini_api_key is configured (nil = always ready).
// The pending-asset transaction runs on the db handle set by SetSchedulingDeps.
func (h *PostsHandler) SetImageGenerator(enq ImageGenEnqueuer, ready func() bool) {
	h.imageEnq = enq
	h.imageReady = ready
}

type imagineRequest struct {
	Instruction      string   `json:"instruction"`
	SubjectDesc      string   `json:"subjectDesc"`
	AspectRatio      string   `json:"aspectRatio"` // optional override; else resolved from the platform
	Resolution       string   `json:"resolution"`  // optional; "" -> 1K
	Premium          bool     `json:"premium"`
	BrandRefAssetIDs []string `json:"brandRefAssetIds"`
	SubjectAssetID   string   `json:"subjectAssetId"`
}

// Imagine godoc
// @Summary      Generate a branded image for a post (async)
// @Description  Creates a pending image Asset and enqueues async generation
// @Description  (Nano Banana, CON-105). Poll the returned asset until its
// @Description  status is "ready" (or "failed"). The default aspect ratio is
// @Description  derived from the post's platform and may be overridden.
// @Tags         posts
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string          true  "Post Sqid"
// @Param        body  body      imagineRequest  true  "Generation request"
// @Success      202  {object}  models.Asset  "pending asset"
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/posts/{id}/imagine [post]
func (h *PostsHandler) Imagine(c *fiber.Ctx) error {
	if h.imageEnq == nil || h.db == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "image generation is not available")
	}
	if h.imageReady != nil && !h.imageReady() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "image generation is not configured")
	}

	session, ok := c.Locals("session").(*models.Session)
	if !ok || session == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
	}

	var req imagineRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	postID := c.Params("id")
	post, err := h.repo.GetByID(c.Context(), postID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "post not found")
		}
		return err
	}

	// Resolve the default aspect ratio from the post's platform (overridable).
	var platform *models.Platform
	if post.PlatformID != "" && h.platformRepo != nil {
		platform, _ = h.platformRepo.GetByID(c.Context(), post.PlatformID)
	}
	aspectRatio := resolveAspectRatio(platform, post.PlatformPostType, req.AspectRatio)

	// Build the pending image Asset. The worker fills in the storage key +
	// generation provenance and flips it to ready.
	id, err := models.NewID()
	if err != nil {
		return err
	}
	imgType := models.AssetTypeImage
	asset := &models.Asset{
		ID:          id,
		Title:       imagineTitle(req, post),
		Content:     "[]",
		Status:      models.AssetStatusPending,
		Type:        &imgType,
		AIGenerated: true,
		TagIDs:      models.StringSlice{},
		Tags:        []models.Tag{},
		CreatedBy:   session.UserID,
	}

	task := queues.ImageGenerateTask{
		AssetID:          asset.ID,
		TenantID:         session.TenantID,
		PostID:           postID,
		Platform:         platformName(platform),
		SubjectDesc:      req.SubjectDesc,
		Instruction:      req.Instruction,
		BrandRefAssetIDs: req.BrandRefAssetIDs,
		SubjectAssetID:   req.SubjectAssetID,
		AspectRatio:      aspectRatio,
		Resolution:       req.Resolution,
		Premium:          req.Premium,
	}

	// Insert the pending asset and enqueue generation atomically (transactional
	// outbox): a committed asset always has a job, a rolled-back one never does.
	if err := h.db.RunInTx(c.Context(), nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(asset).Exec(ctx); err != nil {
			return err
		}
		return h.imageEnq.EnqueueImageGenerateTx(ctx, tx.Tx, task)
	}); err != nil {
		return err
	}

	return c.Status(fiber.StatusAccepted).JSON(asset)
}

func platformName(p *models.Platform) string {
	if p == nil {
		return ""
	}
	return p.Name
}

func imagineTitle(req imagineRequest, post *models.Post) string {
	if s := strings.TrimSpace(req.SubjectDesc); s != "" {
		return "Image: " + truncateTitle(s, 60)
	}
	if t := strings.TrimSpace(post.Title); t != "" {
		return "Image for " + truncateTitle(t, 60)
	}
	return "Generated image"
}

func truncateTitle(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// resolveAspectRatio picks the output aspect ratio for a generation: an explicit
// request override wins; otherwise a per-platform default is used (PRD §7). The
// mapping is a conservative v1 default pending the confirmed Platform→ratio
// table (PRD §11) — vertical post types (story/reel/short) go 9:16, a few
// platforms carry portrait/landscape defaults, and everything else falls back
// to 1:1 (always a supported ratio). imagegen re-validates the result per model.
func resolveAspectRatio(platform *models.Platform, postType, override string) string {
	if r := strings.TrimSpace(override); r != "" {
		return r
	}
	pt := strings.ToLower(postType)
	if strings.Contains(pt, "story") || strings.Contains(pt, "reel") || strings.Contains(pt, "short") {
		return "9:16"
	}
	name := ""
	if platform != nil {
		name = strings.ToLower(platform.Name)
	}
	switch {
	case strings.Contains(name, "tiktok"):
		return "9:16"
	case strings.Contains(name, "youtube"):
		return "16:9"
	case strings.Contains(name, "pinterest"):
		return "2:3"
	case strings.Contains(name, "instagram"):
		return "4:5"
	default:
		return "1:1"
	}
}
