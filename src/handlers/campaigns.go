package handlers

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/content-control-center/app/src/genkit/flows/content_plan"
	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

var validStatuses = map[models.CampaignStatus]bool{
	models.StatusDraft:     true,
	models.StatusScheduled: true,
	models.StatusActive:    true,
	models.StatusPaused:    true,
	models.StatusCompleted: true,
	models.StatusArchived:  true,
}

type CampaignsHandler struct {
	repo             repository.CampaignRepository
	campaignTypeRepo repository.CampaignTypeRepository
	auth             fiber.Handler
	generateDraft    func(ctx context.Context, campaignID string, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error)
}

func NewCampaignsHandler(repo repository.CampaignRepository, campaignTypeRepo repository.CampaignTypeRepository, auth fiber.Handler, generateDraft func(ctx context.Context, campaignID string, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error)) *CampaignsHandler {
	return &CampaignsHandler{repo: repo, campaignTypeRepo: campaignTypeRepo, auth: auth, generateDraft: generateDraft}
}

func (h *CampaignsHandler) Register(app *fiber.App) {
	g := app.Group("/api/campaigns")
	g.Get("/", h.auth, h.List)
	g.Post("/", h.auth, h.Create)
	g.Get("/:id", h.auth, h.Get)
	g.Put("/:id", h.auth, h.Update)
	g.Delete("/:id", h.auth, h.Delete)
	g.Post("/:id/generate-draft", h.auth, h.GenerateDraft)
}

type campaignRequest struct {
	Name               string                   `json:"name"                validate:"required"`
	Description        string                   `json:"description"`
	TargetPersona      string                   `json:"target_persona"`
	KeyMessages        string                   `json:"key_messages"`
	ToneGuidelines     string                   `json:"tone_guidelines"`
	UseAssets          bool                     `json:"use_assets"`
	AssetIDs          models.StringSlice       `json:"asset_ids"`
	TargetPlatforms    models.CampaignPlatforms `json:"target_platforms"`
	CampaignTypeID     string                   `json:"campaign_type_id"    validate:"required"`
	Status             models.CampaignStatus    `json:"status"`
	StartDate          *time.Time               `json:"start_date"`
	EndDate            *time.Time               `json:"end_date"`
	EstimatedPostCount *int                     `json:"estimated_post_count"`
	Budget             *float64                 `json:"budget"`
	Currency           string                   `json:"currency"`
	Language           string                   `json:"language"`
	TagIDs             models.StringSlice       `json:"tag_ids"`
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
	status := req.toStatus()
	if !validStatuses[status] {
		return fiber.NewError(fiber.StatusBadRequest, "invalid status")
	}
	if _, err := h.campaignTypeRepo.GetByID(c.Context(), req.CampaignTypeID); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid campaign_type_id")
	}

	session := c.Locals("session").(*models.Session)

	id, err := models.NewID()
	if err != nil {
		return err
	}

	campaign := &models.Campaign{
		ID:                 id,
		Name:               req.Name,
		Description:        req.Description,
		TargetPersona:      req.TargetPersona,
		KeyMessages:        req.KeyMessages,
		ToneGuidelines:     req.ToneGuidelines,
		UseAssets:          req.UseAssets,
		AssetIDs:          nullSlice(req.AssetIDs),
		TargetPlatforms:    nullCampaignPlatforms(req.TargetPlatforms),
		CampaignTypeID:     req.CampaignTypeID,
		Status:             status,
		EstimatedPostCount: req.EstimatedPostCount,
		StartDate:          req.StartDate,
		EndDate:            req.EndDate,
		Budget:             req.Budget,
		Currency:           req.Currency,
		Language:           req.Language,
		TagIDs:             nullSlice(req.TagIDs),
		Tags:               []models.Tag{},
		CreatedBy:          session.UserID,
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
	status := req.toStatus()
	if !validStatuses[status] {
		return fiber.NewError(fiber.StatusBadRequest, "invalid status")
	}
	if _, err := h.campaignTypeRepo.GetByID(c.Context(), req.CampaignTypeID); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid campaign_type_id")
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
	campaign.UseAssets = req.UseAssets
	campaign.AssetIDs = nullSlice(req.AssetIDs)
	campaign.TargetPlatforms = nullCampaignPlatforms(req.TargetPlatforms)
	campaign.CampaignTypeID = req.CampaignTypeID
	campaign.Status = status
	campaign.EstimatedPostCount = req.EstimatedPostCount
	campaign.StartDate = req.StartDate
	campaign.EndDate = req.EndDate
	campaign.Budget = req.Budget
	campaign.Currency = req.Currency
	campaign.Language = req.Language
	campaign.TagIDs = nullSlice(req.TagIDs)
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

// GenerateDraft godoc
// @Summary      Generate draft posts (SSE)
// @Description  Calls the AI content plan flow and streams progress via Server-Sent Events.
// @Description  Each step emits an SSE event of type "step" with payload {"step":"<name>","status":"done"}.
// @Description  On success a final "complete" event carries the full ContentPlanResponse payload.
// @Description  On failure an "error" event carries {"message":"<text>","code":<http_code>}.
// @Tags         campaigns
// @Produce      text/event-stream
// @Security     CookieAuth
// @Param        id   path  string  true  "Campaign Sqid"
// @Success      200  "SSE stream: step / complete / error events"
// @Failure      401  {object}  map[string]string
// @Failure      403  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/campaigns/{id}/generate-draft [post]
func (h *CampaignsHandler) GenerateDraft(c *fiber.Ctx) error {
	if h.generateDraft == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "content plan feature is not enabled")
	}

	campaign, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "campaign not found")
		}
		return err
	}

	session := c.Locals("session").(*models.Session)
	if campaign.CreatedBy != session.UserID {
		return fiber.NewError(fiber.StatusForbidden, "forbidden")
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	campaignID := campaign.ID
	generateDraft := h.generateDraft

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		writeEvent := func(event string, data any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
			_ = w.Flush()
		}

		onEvent := content_plan.OnEventFunc(func(name content_plan.SSEEventKind, data any) {
			writeEvent(string(name), data)
		})

		resp, err := generateDraft(context.Background(), campaignID, onEvent)
		if err != nil {
			code := fiber.StatusInternalServerError
			msg := err.Error()
			var ve *content_plan.ValidationError
			var ae *content_plan.AIError
			switch {
			case errors.As(err, &ve):
				code = fiber.StatusBadRequest
				msg = ve.Msg
			case errors.As(err, &ae):
				code = fiber.StatusBadGateway
				msg = ae.Msg
			}
			writeEvent(string(content_plan.SSEEventError), content_plan.ErrorEventPayload{Message: msg, Code: code})
			return
		}

		writeEvent(string(content_plan.SSEEventComplete), resp)
	}))

	return nil
}

// nullSlice returns an empty StringSlice instead of nil so the JSON column
// always stores "[]" rather than null.
func nullSlice(s models.StringSlice) models.StringSlice {
	if s == nil {
		return models.StringSlice{}
	}
	return s
}

// nullCampaignPlatforms returns an empty CampaignPlatforms instead of nil.
func nullCampaignPlatforms(p models.CampaignPlatforms) models.CampaignPlatforms {
	if p == nil {
		return models.CampaignPlatforms{}
	}
	return p
}

// nullMap returns an empty PostTypeMap instead of nil so the JSON column
// always stores "{}" rather than null.
func nullMap(m models.PostTypeMap) models.PostTypeMap {
	if m == nil {
		return models.PostTypeMap{}
	}
	return m
}
