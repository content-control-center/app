package handlers

import (
	"database/sql"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

var validObjectives = map[models.CampaignObjective]bool{
	models.ObjectiveAwareness:  true,
	models.ObjectiveEngagement: true,
	models.ObjectiveConversion: true,
	models.ObjectiveRetention:  true,
}

var validStatuses = map[models.CampaignStatus]bool{
	models.StatusDraft:     true,
	models.StatusScheduled: true,
	models.StatusActive:    true,
	models.StatusPaused:    true,
	models.StatusCompleted: true,
	models.StatusArchived:  true,
}

type CampaignsHandler struct {
	repo repository.CampaignRepository
	auth fiber.Handler
}

func NewCampaignsHandler(repo repository.CampaignRepository, auth fiber.Handler) *CampaignsHandler {
	return &CampaignsHandler{repo: repo, auth: auth}
}

func (h *CampaignsHandler) Register(app *fiber.App) {
	g := app.Group("/api/campaigns")
	g.Get("/", h.auth, h.List)
	g.Post("/", h.auth, h.Create)
	g.Get("/:id", h.auth, h.Get)
	g.Put("/:id", h.auth, h.Update)
	g.Delete("/:id", h.auth, h.Delete)
}

type campaignRequest struct {
	Name              string                  `json:"name"                validate:"required"`
	Description       string                  `json:"description"`
	TargetPersona     string                  `json:"target_persona"`
	KeyMessages       string                  `json:"key_messages"`
	ToneGuidelines    string                  `json:"tone_guidelines"`
	UsePieces         bool                    `json:"use_pieces"`
	PiecesIDs         models.StringSlice      `json:"pieces_ids"`
	TargetPlatformIDs models.StringSlice      `json:"target_platform_ids"`
	Objective         models.CampaignObjective `json:"objective"           validate:"required"`
	Status            models.CampaignStatus   `json:"status"`
	StartDate         *time.Time              `json:"start_date"`
	EndDate           *time.Time              `json:"end_date"`
	Budget            *float64                `json:"budget"`
	Currency          string                  `json:"currency"`
	Tags              models.StringSlice      `json:"tags"`
}

func (r *campaignRequest) toStatus() models.CampaignStatus {
	if r.Status == "" {
		return models.StatusDraft
	}
	return r.Status
}

// List godoc
// @Summary      List campaigns
// @Description  Returns all campaigns ordered by creation date.
// @Tags         campaigns
// @Produce      json
// @Security     CookieAuth
// @Success      200  {array}   models.Campaign
// @Failure      401  {object}  map[string]string
// @Router       /api/campaigns [get]
func (h *CampaignsHandler) List(c *fiber.Ctx) error {
	campaigns, err := h.repo.List(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(campaigns)
}

// Create godoc
// @Summary      Create campaign
// @Description  Creates a new campaign. The created_by field is set from the authenticated session.
// @Tags         campaigns
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body      campaignRequest  true  "Campaign payload"
// @Success      201   {object}  models.Campaign
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Router       /api/campaigns [post]
func (h *CampaignsHandler) Create(c *fiber.Ctx) error {
	var req campaignRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}
	if !validObjectives[req.Objective] {
		return fiber.NewError(fiber.StatusBadRequest, "invalid objective")
	}
	status := req.toStatus()
	if !validStatuses[status] {
		return fiber.NewError(fiber.StatusBadRequest, "invalid status")
	}

	session := c.Locals("session").(*models.Session)

	id, err := models.NewID()
	if err != nil {
		return err
	}

	campaign := &models.Campaign{
		ID:                id,
		Name:              req.Name,
		Description:       req.Description,
		TargetPersona:     req.TargetPersona,
		KeyMessages:       req.KeyMessages,
		ToneGuidelines:    req.ToneGuidelines,
		UsePieces:         req.UsePieces,
		PiecesIDs:         nullSlice(req.PiecesIDs),
		TargetPlatformIDs: nullSlice(req.TargetPlatformIDs),
		Objective:         req.Objective,
		Status:            status,
		StartDate:         req.StartDate,
		EndDate:           req.EndDate,
		Budget:            req.Budget,
		Currency:          req.Currency,
		Tags:              nullSlice(req.Tags),
		CreatedBy:         session.UserID,
	}
	if err := h.repo.Create(c.Context(), campaign); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(campaign)
}

// Get godoc
// @Summary      Get campaign
// @Description  Returns a single campaign by Sqid.
// @Tags         campaigns
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "Campaign Sqid"
// @Success      200  {object}  models.Campaign
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/campaigns/{id} [get]
func (h *CampaignsHandler) Get(c *fiber.Ctx) error {
	campaign, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "campaign not found")
		}
		return err
	}
	return c.JSON(campaign)
}

// Update godoc
// @Summary      Update campaign
// @Description  Replaces all mutable fields of an existing campaign.
// @Tags         campaigns
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string          true  "Campaign Sqid"
// @Param        body  body      campaignRequest true  "Campaign payload"
// @Success      200   {object}  models.Campaign
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Router       /api/campaigns/{id} [put]
func (h *CampaignsHandler) Update(c *fiber.Ctx) error {
	var req campaignRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}
	if !validObjectives[req.Objective] {
		return fiber.NewError(fiber.StatusBadRequest, "invalid objective")
	}
	status := req.toStatus()
	if !validStatuses[status] {
		return fiber.NewError(fiber.StatusBadRequest, "invalid status")
	}

	campaign, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "campaign not found")
		}
		return err
	}

	campaign.Name = req.Name
	campaign.Description = req.Description
	campaign.TargetPersona = req.TargetPersona
	campaign.KeyMessages = req.KeyMessages
	campaign.ToneGuidelines = req.ToneGuidelines
	campaign.UsePieces = req.UsePieces
	campaign.PiecesIDs = nullSlice(req.PiecesIDs)
	campaign.TargetPlatformIDs = nullSlice(req.TargetPlatformIDs)
	campaign.Objective = req.Objective
	campaign.Status = status
	campaign.StartDate = req.StartDate
	campaign.EndDate = req.EndDate
	campaign.Budget = req.Budget
	campaign.Currency = req.Currency
	campaign.Tags = nullSlice(req.Tags)
	campaign.UpdatedAt = time.Now().UTC()

	if err := h.repo.Update(c.Context(), campaign); err != nil {
		return err
	}
	return c.JSON(campaign)
}

// Delete godoc
// @Summary      Delete campaign
// @Description  Deletes a campaign by Sqid.
// @Tags         campaigns
// @Security     CookieAuth
// @Param        id   path  string  true  "Campaign Sqid"
// @Success      204
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/campaigns/{id} [delete]
func (h *CampaignsHandler) Delete(c *fiber.Ctx) error {
	deleted, err := h.repo.Delete(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	if !deleted {
		return fiber.NewError(fiber.StatusNotFound, "campaign not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// nullSlice returns an empty StringSlice instead of nil so the JSON column
// always stores "[]" rather than null.
func nullSlice(s models.StringSlice) models.StringSlice {
	if s == nil {
		return models.StringSlice{}
	}
	return s
}

// nullMap returns an empty PostTypeMap instead of nil so the JSON column
// always stores "{}" rather than null.
func nullMap(m models.PostTypeMap) models.PostTypeMap {
	if m == nil {
		return models.PostTypeMap{}
	}
	return m
}
