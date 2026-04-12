package handlers

import (
	"database/sql"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

type PiecesHandler struct {
	repo   repository.PieceRepository
	auth   fiber.Handler
	onSave func(pieceID, title, content string) // async embedding trigger; nil = disabled
}

func NewPiecesHandler(repo repository.PieceRepository, auth fiber.Handler, onSave func(pieceID, title, content string)) *PiecesHandler {
	return &PiecesHandler{repo: repo, auth: auth, onSave: onSave}
}

func (h *PiecesHandler) Register(app *fiber.App) {
	g := app.Group("/api/content-bank/pieces")
	g.Get("/", h.auth, h.List)
	g.Post("/", h.auth, h.Create)
	g.Get("/:id", h.auth, h.Get)
	g.Put("/:id", h.auth, h.Update)
	g.Delete("/:id", h.auth, h.Delete)
}

type createPieceRequest struct {
	Title   string             `json:"title"   validate:"required"`
	Content string             `json:"content" validate:"required"`
	TagIDs  models.StringSlice `json:"tag_ids"`
}

type updatePieceRequest struct {
	Title   string             `json:"title"   validate:"required"`
	Content string             `json:"content" validate:"required"`
	TagIDs  models.StringSlice `json:"tag_ids"`
}

// List godoc
// @Summary      List pieces
// @Description  Returns all content bank pieces ordered by creation date.
// @Tags         content-bank
// @Produce      json
// @Security     CookieAuth
// @Success      200  {array}   models.Piece
// @Failure      401  {object}  map[string]string
// @Router       /api/content-bank/pieces [get]
func (h *PiecesHandler) List(c *fiber.Ctx) error {
	pieces, err := h.repo.List(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(pieces)
}

// Create godoc
// @Summary      Create piece
// @Description  Creates a new content bank piece. The created_by field is set from the authenticated session.
// @Tags         content-bank
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body      createPieceRequest  true  "Piece payload"
// @Success      201   {object}  models.Piece
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Router       /api/content-bank/pieces [post]
func (h *PiecesHandler) Create(c *fiber.Ctx) error {
	var req createPieceRequest
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

	piece := &models.Piece{
		ID:        id,
		Title:     req.Title,
		Content:   req.Content,
		TagIDs:    nullSlice(req.TagIDs),
		Tags:      []models.Tag{},
		CreatedBy: session.UserID,
	}
	if err := h.repo.Create(c.Context(), piece); err != nil {
		return err
	}

	if h.onSave != nil {
		go h.onSave(piece.ID, piece.Title, piece.Content)
	}

	return c.Status(fiber.StatusCreated).JSON(piece)
}

// Get godoc
// @Summary      Get piece
// @Description  Returns a single content bank piece by Sqid.
// @Tags         content-bank
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "Piece Sqid"
// @Success      200  {object}  models.Piece
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/content-bank/pieces/{id} [get]
func (h *PiecesHandler) Get(c *fiber.Ctx) error {
	piece, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "piece not found")
		}
		return err
	}
	return c.JSON(piece)
}

// Update godoc
// @Summary      Update piece
// @Description  Updates title and content of an existing piece.
// @Tags         content-bank
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string             true  "Piece Sqid"
// @Param        body  body      updatePieceRequest true  "Piece payload"
// @Success      200   {object}  models.Piece
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Router       /api/content-bank/pieces/{id} [put]
func (h *PiecesHandler) Update(c *fiber.Ctx) error {
	var req updatePieceRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	piece, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "piece not found")
		}
		return err
	}

	piece.Title = req.Title
	piece.Content = req.Content
	piece.TagIDs = nullSlice(req.TagIDs)
	piece.UpdatedAt = time.Now().UTC()

	if err := h.repo.Update(c.Context(), piece); err != nil {
		return err
	}

	if h.onSave != nil {
		go h.onSave(piece.ID, piece.Title, piece.Content)
	}

	return c.JSON(piece)
}

// Delete godoc
// @Summary      Delete piece
// @Description  Deletes a content bank piece by Sqid.
// @Tags         content-bank
// @Security     CookieAuth
// @Param        id   path  string  true  "Piece Sqid"
// @Success      204
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/content-bank/pieces/{id} [delete]
func (h *PiecesHandler) Delete(c *fiber.Ctx) error {
	deleted, err := h.repo.Delete(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	if !deleted {
		return fiber.NewError(fiber.StatusNotFound, "piece not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
