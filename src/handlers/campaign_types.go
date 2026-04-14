package handlers

import (
	"database/sql"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/content-control-center/app/src/repository"
)

type CampaignTypesHandler struct {
	repo repository.CampaignTypeRepository
	auth fiber.Handler
}

func NewCampaignTypesHandler(repo repository.CampaignTypeRepository, auth fiber.Handler) *CampaignTypesHandler {
	return &CampaignTypesHandler{repo: repo, auth: auth}
}

func (h *CampaignTypesHandler) Register(app *fiber.App) {
	g := app.Group("/api/campaign_types")
	g.Get("/", h.auth, h.List)
	g.Get("/:id", h.auth, h.Get)
}

// List godoc
// @Summary      List campaign types
// @Description  Returns all campaign types with their phases ordered by name.
// @Tags         campaign_types
// @Produce      json
// @Security     CookieAuth
// @Success      200  {array}   models.CampaignType
// @Failure      401  {object}  map[string]string
// @Router       /api/campaign_types [get]
func (h *CampaignTypesHandler) List(c *fiber.Ctx) error {
	types, err := h.repo.List(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(types)
}

// Get godoc
// @Summary      Get campaign type
// @Description  Returns a single campaign type with its phases by ID.
// @Tags         campaign_types
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "CampaignType ID"
// @Success      200  {object}  models.CampaignType
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/campaign_types/{id} [get]
func (h *CampaignTypesHandler) Get(c *fiber.Ctx) error {
	ct, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "campaign type not found")
		}
		return err
	}
	return c.JSON(ct)
}
