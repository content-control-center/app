package handlers

import (
	"database/sql"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

type TagsHandler struct {
	repo repository.TagRepository
	auth fiber.Handler
}

func NewTagsHandler(repo repository.TagRepository, auth fiber.Handler) *TagsHandler {
	return &TagsHandler{repo: repo, auth: auth}
}

func (h *TagsHandler) Register(app *fiber.App) {
	g := app.Group("/api/tags")
	g.Get("/", h.auth, h.List)
	g.Post("/", h.auth, h.Create)
	g.Get("/:id", h.auth, h.Get)
	g.Put("/:id", h.auth, h.Update)
	g.Delete("/:id", h.auth, h.Delete)
}

type tagRequest struct {
	Name  string `json:"name"  validate:"required"`
	Color string `json:"color"`
}

// List godoc
// @Summary      List tags
// @Description  Returns all tags ordered by creation date.
// @Tags         tags
// @Produce      json
// @Security     CookieAuth
// @Success      200  {array}   models.Tag
// @Failure      401  {object}  map[string]string
// @Router       /api/tags [get]
func (h *TagsHandler) List(c *fiber.Ctx) error {
	tags, err := h.repo.List(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(tags)
}

// Create godoc
// @Summary      Create tag
// @Description  Creates a new tag. The created_by field is set from the authenticated session.
// @Tags         tags
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body      tagRequest  true  "Tag payload"
// @Success      201   {object}  models.Tag
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Router       /api/tags [post]
func (h *TagsHandler) Create(c *fiber.Ctx) error {
	var req tagRequest
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

	tag := &models.Tag{
		ID:        id,
		Name:      req.Name,
		Color:     req.Color,
		CreatedBy: session.UserID,
	}
	if err := h.repo.Create(c.Context(), tag); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(tag)
}

// Get godoc
// @Summary      Get tag
// @Description  Returns a single tag by Sqid.
// @Tags         tags
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "Tag Sqid"
// @Success      200  {object}  models.Tag
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/tags/{id} [get]
func (h *TagsHandler) Get(c *fiber.Ctx) error {
	tag, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "tag not found")
		}
		return err
	}
	return c.JSON(tag)
}

// Update godoc
// @Summary      Update tag
// @Description  Updates name and color of an existing tag.
// @Tags         tags
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string     true  "Tag Sqid"
// @Param        body  body      tagRequest true  "Tag payload"
// @Success      200   {object}  models.Tag
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Router       /api/tags/{id} [put]
func (h *TagsHandler) Update(c *fiber.Ctx) error {
	var req tagRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	tag, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "tag not found")
		}
		return err
	}

	tag.Name = req.Name
	tag.Color = req.Color
	tag.UpdatedAt = time.Now().UTC()

	if err := h.repo.Update(c.Context(), tag); err != nil {
		return err
	}
	return c.JSON(tag)
}

// Delete godoc
// @Summary      Delete tag
// @Description  Deletes a tag by Sqid.
// @Tags         tags
// @Security     CookieAuth
// @Param        id   path  string  true  "Tag Sqid"
// @Success      204
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/tags/{id} [delete]
func (h *TagsHandler) Delete(c *fiber.Ctx) error {
	deleted, err := h.repo.Delete(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	if !deleted {
		return fiber.NewError(fiber.StatusNotFound, "tag not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
