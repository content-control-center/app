package handlers

import (
	"database/sql"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

type AssetsHandler struct {
	repo   repository.AssetRepository
	auth   fiber.Handler
	onSave func(assetID, title, content string) // async embedding trigger; nil = disabled
}

func NewAssetsHandler(repo repository.AssetRepository, auth fiber.Handler, onSave func(assetID, title, content string)) *AssetsHandler {
	return &AssetsHandler{repo: repo, auth: auth, onSave: onSave}
}

func (h *AssetsHandler) Register(app *fiber.App) {
	g := app.Group("/api/content-bank/assets")
	g.Get("/", h.auth, h.List)
	g.Post("/", h.auth, h.Create)
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
	return c.JSON(assets)
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
		TagIDs:    nullSlice(req.TagIDs),
		Tags:      []models.Tag{},
		CreatedBy: session.UserID,
	}
	if err := h.repo.Create(c.Context(), asset); err != nil {
		return err
	}

	if h.onSave != nil {
		go h.onSave(asset.ID, asset.Title, asset.Content)
	}

	return c.Status(fiber.StatusCreated).JSON(asset)
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

	asset.Title = req.Title
	asset.Content = req.Content
	asset.TagIDs = nullSlice(req.TagIDs)
	asset.UpdatedAt = time.Now().UTC()

	if err := h.repo.Update(c.Context(), asset); err != nil {
		return err
	}

	if h.onSave != nil {
		go h.onSave(asset.ID, asset.Title, asset.Content)
	}

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
	deleted, err := h.repo.Delete(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	if !deleted {
		return fiber.NewError(fiber.StatusNotFound, "asset not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
