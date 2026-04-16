package handlers

import (
	"database/sql"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

var validPostStatuses = map[models.PostStatus]bool{
	models.PostStatusDraft:                     true,
	models.PostStatusReadyForPublish:           true,
	models.PostStatusScheduled:                 true,
	models.PostStatusScheduledForManualPublish: true,
	models.PostStatusFailed:                    true,
	models.PostStatusPublished:                 true,
	models.PostStatusNotPublished:              true,
}

var validCTATypes = map[models.PostCTAType]bool{
	models.CTATypeLink:   true,
	models.CTATypeButton: true,
	models.CTATypeNone:   true,
}

type PostsHandler struct {
	repo repository.PostRepository
	auth fiber.Handler
}

func NewPostsHandler(repo repository.PostRepository, auth fiber.Handler) *PostsHandler {
	return &PostsHandler{repo: repo, auth: auth}
}

func (h *PostsHandler) Register(app *fiber.App) {
	g := app.Group("/api/posts")
	g.Get("/", h.auth, h.List)
	g.Post("/", h.auth, h.Create)
	g.Get("/:id", h.auth, h.Get)
	g.Put("/:id", h.auth, h.Update)
	g.Delete("/:id", h.auth, h.Delete)

	app.Get("/api/campaigns/:campaign_id/posts", h.auth, h.ListByCampaign)
}

type postRequest struct {
	CampaignID          string             `json:"campaign_id"             validate:"required"`
	PlatformID          string             `json:"platform_id"             validate:"required"`
	PlatformPostType    string             `json:"platform_post_type"      validate:"required"`
	Title               string             `json:"title"                   validate:"required"`
	Content             string             `json:"content"`
	MediaURLs           models.StringSlice `json:"media_urls"`
	ScheduledAt         *time.Time         `json:"scheduled_at"`
	PublishedAt         *time.Time         `json:"published_at"`
	Status              models.PostStatus  `json:"status"`
	CTAType             models.PostCTAType `json:"cta_type"`
	CTAUrl              string             `json:"cta_url"`
	TargetAudienceNotes string             `json:"target_audience_notes"`
	UsedAssetIDs       models.StringSlice `json:"used_asset_ids"`
	CampaignTypePhaseID *string            `json:"campaign_type_phase_id"`
}

func (r *postRequest) toStatus() models.PostStatus {
	if r.Status == "" {
		return models.PostStatusDraft
	}
	return r.Status
}

func (r *postRequest) toCTAType() models.PostCTAType {
	if r.CTAType == "" {
		return models.CTATypeNone
	}
	return r.CTAType
}

// List godoc
// @Summary      List posts
// @Description  Returns all posts ordered by creation date, with campaign, platform, and assets hydrated.
// @Tags         posts
// @Produce      json
// @Security     CookieAuth
// @Success      200  {array}   models.Post
// @Failure      401  {object}  map[string]string
// @Router       /api/posts [get]
func (h *PostsHandler) List(c *fiber.Ctx) error {
	posts, err := h.repo.List(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(posts)
}

// ListByCampaign godoc
// @Summary      List posts by campaign
// @Description  Returns all posts for the given campaign ID, with campaign, platform, and assets hydrated.
// @Tags         posts
// @Produce      json
// @Security     CookieAuth
// @Param        campaign_id  path      string  true  "Campaign Sqid"
// @Success      200          {array}   models.Post
// @Failure      401          {object}  map[string]string
// @Router       /api/campaigns/{campaign_id}/posts [get]
func (h *PostsHandler) ListByCampaign(c *fiber.Ctx) error {
	posts, err := h.repo.ListByCampaign(c.Context(), c.Params("campaign_id"))
	if err != nil {
		return err
	}
	return c.JSON(posts)
}

// Create godoc
// @Summary      Create post
// @Description  Creates a new post. The created_by field is set from the authenticated session.
// @Tags         posts
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body      postRequest  true  "Post payload"
// @Success      201   {object}  models.Post
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Router       /api/posts [post]
func (h *PostsHandler) Create(c *fiber.Ctx) error {
	var req postRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}
	status := req.toStatus()
	if !validPostStatuses[status] {
		return fiber.NewError(fiber.StatusBadRequest, "invalid status")
	}
	ctaType := req.toCTAType()
	if !validCTATypes[ctaType] {
		return fiber.NewError(fiber.StatusBadRequest, "invalid cta_type")
	}

	session := c.Locals("session").(*models.Session)

	id, err := models.NewID()
	if err != nil {
		return err
	}

	post := &models.Post{
		ID:                  id,
		CampaignID:          req.CampaignID,
		PlatformID:          req.PlatformID,
		PlatformPostType:    req.PlatformPostType,
		Title:               req.Title,
		Content:             req.Content,
		MediaURLs:           nullSlice(req.MediaURLs),
		ScheduledAt:         req.ScheduledAt,
		PublishedAt:         req.PublishedAt,
		Status:              status,
		CTAType:             ctaType,
		CTAUrl:              req.CTAUrl,
		TargetAudienceNotes: req.TargetAudienceNotes,
		UsedAssetIDs:       nullSlice(req.UsedAssetIDs),
		CampaignTypePhaseID: req.CampaignTypePhaseID,
		CreatedBy:           session.UserID,
		UsedAssets:          []models.Asset{},
	}
	if err := h.repo.Create(c.Context(), post); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(post)
}

// Get godoc
// @Summary      Get post
// @Description  Returns a single post by Sqid with campaign, platform, and assets hydrated.
// @Tags         posts
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "Post Sqid"
// @Success      200  {object}  models.Post
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/posts/{id} [get]
func (h *PostsHandler) Get(c *fiber.Ctx) error {
	post, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "post not found")
		}
		return err
	}
	return c.JSON(post)
}

// Update godoc
// @Summary      Update post
// @Description  Replaces all mutable fields of an existing post.
// @Tags         posts
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string      true  "Post Sqid"
// @Param        body  body      postRequest true  "Post payload"
// @Success      200   {object}  models.Post
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Router       /api/posts/{id} [put]
func (h *PostsHandler) Update(c *fiber.Ctx) error {
	var req postRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}
	status := req.toStatus()
	if !validPostStatuses[status] {
		return fiber.NewError(fiber.StatusBadRequest, "invalid status")
	}
	ctaType := req.toCTAType()
	if !validCTATypes[ctaType] {
		return fiber.NewError(fiber.StatusBadRequest, "invalid cta_type")
	}

	post, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "post not found")
		}
		return err
	}

	if !post.Status.CanTransition(status) {
		return fiber.NewError(fiber.StatusBadRequest, "invalid status transition from "+string(post.Status)+" to "+string(status))
	}

	post.CampaignID = req.CampaignID
	post.PlatformID = req.PlatformID
	post.PlatformPostType = req.PlatformPostType
	post.Title = req.Title
	post.Content = req.Content
	post.MediaURLs = nullSlice(req.MediaURLs)
	post.ScheduledAt = req.ScheduledAt
	post.PublishedAt = req.PublishedAt
	post.Status = status
	post.CTAType = ctaType
	post.CTAUrl = req.CTAUrl
	post.CampaignTypePhaseID = req.CampaignTypePhaseID
	post.TargetAudienceNotes = req.TargetAudienceNotes
	post.UsedAssetIDs = nullSlice(req.UsedAssetIDs)
	post.UpdatedAt = time.Now().UTC()

	if err := h.repo.Update(c.Context(), post); err != nil {
		return err
	}

	// Re-fetch to return fully hydrated response.
	updated, err := h.repo.GetByID(c.Context(), post.ID)
	if err != nil {
		return err
	}
	return c.JSON(updated)
}

// Delete godoc
// @Summary      Delete post
// @Description  Deletes a post by Sqid.
// @Tags         posts
// @Security     CookieAuth
// @Param        id   path  string  true  "Post Sqid"
// @Success      204
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/posts/{id} [delete]
func (h *PostsHandler) Delete(c *fiber.Ctx) error {
	deleted, err := h.repo.Delete(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	if !deleted {
		return fiber.NewError(fiber.StatusNotFound, "post not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
