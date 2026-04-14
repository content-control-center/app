package handlers

import (
	"database/sql"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

type PlatformsHandler struct {
	repo repository.PlatformRepository
	auth fiber.Handler
}

func NewPlatformsHandler(repo repository.PlatformRepository, auth fiber.Handler) *PlatformsHandler {
	return &PlatformsHandler{repo: repo, auth: auth}
}

func (h *PlatformsHandler) Register(app *fiber.App) {
	g := app.Group("/api/platforms")
	g.Get("/", h.auth, h.List)
	g.Post("/", h.auth, h.Create)
	g.Get("/:id", h.auth, h.Get)
	g.Put("/:id", h.auth, h.Update)
	g.Delete("/:id", h.auth, h.Delete)
}

type platformRequest struct {
	Name        string             `json:"name"        validate:"required"`
	PostTypes   models.PostTypeMap `json:"post_types"`
	Cadence     string             `json:"cadence"`
	Constraints string             `json:"constraints"`
}

// List godoc
// @Summary      List platforms
// @Description  Returns all platforms ordered by creation date.
// @Tags         platforms
// @Produce      json
// @Security     CookieAuth
// @Success      200  {array}   models.Platform
// @Failure      401  {object}  map[string]string
// @Router       /api/platforms [get]
func (h *PlatformsHandler) List(c *fiber.Ctx) error {
	platforms, err := h.repo.List(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(platforms)
}

// Create godoc
// @Summary      Create platform
// @Description  Creates a new platform.
// @Tags         platforms
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body      platformRequest  true  "Platform payload"
// @Success      201   {object}  models.Platform
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Router       /api/platforms [post]
func (h *PlatformsHandler) Create(c *fiber.Ctx) error {
	var req platformRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	id, err := models.NewID()
	if err != nil {
		return err
	}

	platform := &models.Platform{
		ID:          id,
		Name:        req.Name,
		PostTypes:   nullMap(req.PostTypes),
		Cadence:     req.Cadence,
		Constraints: req.Constraints,
	}
	if err := h.repo.Create(c.Context(), platform); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(platform)
}

// Get godoc
// @Summary      Get platform
// @Description  Returns a single platform by Sqid.
// @Tags         platforms
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "Platform Sqid"
// @Success      200  {object}  models.Platform
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/platforms/{id} [get]
func (h *PlatformsHandler) Get(c *fiber.Ctx) error {
	platform, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "platform not found")
		}
		return err
	}
	return c.JSON(platform)
}

// Update godoc
// @Summary      Update platform
// @Description  Replaces all mutable fields of an existing platform.
// @Tags         platforms
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string          true  "Platform Sqid"
// @Param        body  body      platformRequest true  "Platform payload"
// @Success      200   {object}  models.Platform
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Router       /api/platforms/{id} [put]
func (h *PlatformsHandler) Update(c *fiber.Ctx) error {
	var req platformRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	platform, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "platform not found")
		}
		return err
	}

	platform.Name = req.Name
	platform.PostTypes = nullMap(req.PostTypes)
	platform.Cadence = req.Cadence
	platform.Constraints = req.Constraints
	platform.UpdatedAt = time.Now().UTC()

	if err := h.repo.Update(c.Context(), platform); err != nil {
		return err
	}
	return c.JSON(platform)
}

// Delete godoc
// @Summary      Delete platform
// @Description  Deletes a platform by Sqid.
// @Tags         platforms
// @Security     CookieAuth
// @Param        id   path  string  true  "Platform Sqid"
// @Success      204
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/platforms/{id} [delete]
func (h *PlatformsHandler) Delete(c *fiber.Ctx) error {
	deleted, err := h.repo.Delete(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	if !deleted {
		return fiber.NewError(fiber.StatusNotFound, "platform not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
