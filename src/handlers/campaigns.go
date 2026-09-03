package handlers

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/ogen-app/ogen/src/activity"
	"github.com/ogen-app/ogen/src/campaign_actions/overview"
	"github.com/ogen-app/ogen/src/campaign_actions/summaries"
	"github.com/ogen-app/ogen/src/campaigngoal"
	"github.com/ogen-app/ogen/src/genkit/flows/campaign_assistant"
	"github.com/ogen-app/ogen/src/genkit/flows/consistency"
	"github.com/ogen-app/ogen/src/genkit/flows/content_plan"
	"github.com/ogen-app/ogen/src/genkit/flows/enrich_brief"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/scheduling"
	"github.com/ogen-app/ogen/src/settings"
	"github.com/ogen-app/ogen/src/tenantctx"
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
	// brandRepo validates campaign brand_voice_id/brand_audience_id belong to
	// the tenant (CON-245). Optional (SetBrandRepo); nil skips validation.
	brandRepo     repository.BrandRepository
	auth          fiber.Handler
	generateDraft func(ctx context.Context, campaignID string, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error)
	// isContentPlanReady reports whether the underlying Anthropic key
	// is currently configured. Decoupled from generateDraft so we can
	// return 503 before opening the SSE stream rather than emitting
	// an error event mid-stream when the runtime is unavailable.
	// May be nil in tests; nil is treated as "always available" so
	// existing fixture wiring keeps working. The same readiness gate
	// covers enrichBrief — both flows live behind the one Anthropic key.
	isContentPlanReady func() bool
	// enrichBrief streams an AI-generated campaign brief (CON-56). nil
	// when the feature is unwired (e.g. tests) → the handler returns 503.
	enrichBrief func(ctx context.Context, req enrich_brief.EnrichBriefRequest, onEvent enrich_brief.OnEventFunc) (*enrich_brief.EnrichBriefResponse, error)
	// messageRepo persists the Campaign Assistant conversation (CON-112). nil
	// in tests that don't exercise the assistant.
	messageRepo repository.CampaignAssistantMessageRepository
	// assistant is the Campaign Assistant flow callback (CON-112). nil when
	// unwired (e.g. tests) → the assistant/messages endpoints return 503.
	// Readiness is gated by isContentPlanReady, the same Anthropic-key gate.
	assistant func(ctx context.Context, req campaign_assistant.CampaignAssistantRequest, onEvent campaign_assistant.OnEventFunc) (*campaign_assistant.CampaignAssistantResponse, error)
	// overview backs GET /:id/overview (CON-113). Set via SetOverviewService
	// (a late-bound optional dep, like PostsHandler's setters), so existing
	// NewCampaignsHandler call sites stay unchanged. nil → the endpoint 503s.
	// Not gated by the Anthropic key — it's a plain tenant-scoped DB read.
	overview *overview.Service
	// summaries backs GET /summaries (CON-152), the batched Campaigns-list
	// read that replaces the per-card GET /:id/posts N+1. Set via
	// SetSummariesService (late-bound optional dep, like overview). nil → the
	// endpoint 503s. A plain tenant-scoped DB read, not gated by any AI key.
	summaries *summaries.Service
	// generatePosts backs POST /:id/generate-posts (CON-114); generatePostsMax
	// caps the count. Set via SetGeneratePosts. nil → the endpoint 503s.
	generatePosts    func(ctx context.Context, req content_plan.GeneratePostsRequest, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error)
	generatePostsMax int
	// checkBrief / checkPosts back the read-only consistency reviews (CON-116):
	// POST /:id/brief-review and POST /:id/posts-review. Set via SetConsistency.
	// Gated by isContentPlanReady (same Anthropic key); nil → the endpoints 503.
	checkBrief func(ctx context.Context, campaignID string, onEvent consistency.OnEventFunc) (*consistency.BriefReview, error)
	checkPosts func(ctx context.Context, req consistency.PostsCheckRequest, onEvent consistency.OnEventFunc) (*consistency.PostsReview, error)
	// activity records CON-125 user-activity events (campaign_created,
	// content_generated, …). nil is a no-op (analytics disabled / fixtures).
	// Wired via SetActivityRecorder.
	activity *activity.Recorder
}

// SetBrandRepo wires the CON-245 brand repository so campaign brand refs can be
// tenant-validated. Optional; nil skips validation.
func (h *CampaignsHandler) SetBrandRepo(r repository.BrandRepository) {
	h.brandRepo = r
}

// SetActivityRecorder wires the CON-125 activity recorder. nil (analytics
// disabled) makes every activity emission a no-op.
func (h *CampaignsHandler) SetActivityRecorder(r *activity.Recorder) {
	h.activity = r
}

// recordActivity emits a best-effort CON-125 activity event. Tenant + user are
// resolved from the request context (set by the auth middleware); source
// defaults to api and can be overridden by a later option.
func (h *CampaignsHandler) recordActivity(c *fiber.Ctx, category, typ string, opts ...activity.Option) {
	h.activity.Record(c.Context(), category, typ,
		append([]activity.Option{activity.WithSource(activity.SourceAPI)}, opts...)...)
}

// timePtrEqual compares two optional timestamps by value (the fields are
// *time.Time, so == would compare pointers).
func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// SetOverviewService wires the campaign overview service (CON-113). Kept as a
// setter, not a constructor arg, so existing NewCampaignsHandler call sites and
// fixtures stay unchanged.
func (h *CampaignsHandler) SetOverviewService(svc *overview.Service) {
	h.overview = svc
}

// SetSummariesService wires the batched Campaigns-list summary service
// (CON-152). Setter, not a constructor arg, so existing NewCampaignsHandler
// call sites and fixtures stay unchanged.
func (h *CampaignsHandler) SetSummariesService(svc *summaries.Service) {
	h.summaries = svc
}

// SetGeneratePosts wires the CON-114 targeted generation callback + its per-call
// cap. Setter, not a constructor arg, so existing call sites stay unchanged.
func (h *CampaignsHandler) SetGeneratePosts(fn func(ctx context.Context, req content_plan.GeneratePostsRequest, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error), max int) {
	h.generatePosts = fn
	h.generatePostsMax = max
}

// SetConsistency wires the CON-116 read-only review callbacks. Setter, not a
// constructor arg, so existing call sites and fixtures stay unchanged.
func (h *CampaignsHandler) SetConsistency(
	checkBrief func(ctx context.Context, campaignID string, onEvent consistency.OnEventFunc) (*consistency.BriefReview, error),
	checkPosts func(ctx context.Context, req consistency.PostsCheckRequest, onEvent consistency.OnEventFunc) (*consistency.PostsReview, error),
) {
	h.checkBrief = checkBrief
	h.checkPosts = checkPosts
}

func NewCampaignsHandler(
	repo repository.CampaignRepository,
	campaignTypeRepo repository.CampaignTypeRepository,
	auth fiber.Handler,
	generateDraft func(ctx context.Context, campaignID string, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error),
	isContentPlanReady func() bool,
	enrichBrief func(ctx context.Context, req enrich_brief.EnrichBriefRequest, onEvent enrich_brief.OnEventFunc) (*enrich_brief.EnrichBriefResponse, error),
	messageRepo repository.CampaignAssistantMessageRepository,
	assistant func(ctx context.Context, req campaign_assistant.CampaignAssistantRequest, onEvent campaign_assistant.OnEventFunc) (*campaign_assistant.CampaignAssistantResponse, error),
) *CampaignsHandler {
	return &CampaignsHandler{
		repo:               repo,
		campaignTypeRepo:   campaignTypeRepo,
		auth:               auth,
		generateDraft:      generateDraft,
		isContentPlanReady: isContentPlanReady,
		enrichBrief:        enrichBrief,
		messageRepo:        messageRepo,
		assistant:          assistant,
	}
}

func (h *CampaignsHandler) Register(app *fiber.App) {
	g := app.Group("/api/campaigns")
	g.Get("/", h.auth, h.List)
	g.Post("/", h.auth, h.Create)
	// CON-152: batched Campaigns-list summaries. Registered before /:id so the
	// static "summaries" segment is not captured as a campaign id param.
	g.Get("/summaries", h.auth, h.Summaries)
	g.Get("/:id", h.auth, h.Get)
	g.Put("/:id", h.auth, h.Update)
	g.Delete("/:id", h.auth, h.Delete)
	// CON-156 (BE 6): campaign lifecycle. Archive removes a campaign from the
	// active list (reversible); unarchive returns it. Both tenant-scoped.
	g.Post("/:id/archive", h.auth, h.Archive)
	g.Post("/:id/unarchive", h.auth, h.Unarchive)
	g.Post("/:id/generate-draft", h.auth, h.GenerateDraft)
	g.Post("/:id/enrich-brief", h.auth, h.EnrichBrief)
	// CON-114: targeted generation — add a few posts for a platform subset,
	// phase, and date window. Tenant-scoped (no owner guard), mirroring the
	// assistant capability.
	g.Post("/:id/generate-posts", h.auth, h.GeneratePosts)
	// CON-112: Campaign Assistant chat. Tenant-scoped only — any user who can
	// see the campaign can use the assistant (no owner-only guard).
	g.Post("/:id/assistant", h.auth, h.Assistant)
	g.Get("/:id/messages", h.auth, h.ListMessages)
	// CON-113: quick campaign overview (brief recap + phases + content distribution).
	g.Get("/:id/overview", h.auth, h.Overview)
	// CON-116: read-only consistency reviews. Tenant-scoped, SSE.
	g.Post("/:id/brief-review", h.auth, h.BriefReview)
	g.Post("/:id/posts-review", h.auth, h.PostsReview)
}

type campaignRequest struct {
	Name           string `json:"name"                validate:"required"`
	Description    string `json:"description"`
	TargetPersona  string `json:"target_persona"`
	KeyMessages    string `json:"key_messages"`
	ToneGuidelines string `json:"tone_guidelines"`
	// Presence-aware (CON-245): omitting a brand ref on a full-replace save
	// leaves the stored value alone; an explicit null clears it. See Optional.
	BrandVoiceID       Optional[string]         `json:"brand_voice_id"`
	BrandAudienceID    Optional[string]         `json:"brand_audience_id"`
	UseAssets          bool                     `json:"use_assets"`
	AssetIDs           models.StringSlice       `json:"asset_ids"`
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
	// Scheduling settings (CON-181). All optional; omitted fields fall back to
	// defaults (09:00 / UTC / every day / ±15 min) via normalizeScheduling.
	PublishingTime string             `json:"publishing_time"`
	Timezone       string             `json:"timezone"`
	PublishingDays models.StringSlice `json:"publishing_days"`
	SpreadMinutes  *int               `json:"spread_minutes"`
	// Goal cadence (CON-182): "week" | "month". Empty falls back to "month".
	// estimated_post_count is the target posts per one of these periods.
	GoalCadence string `json:"goal_cadence"`
}

// normalizeScheduling validates the request's scheduling fields and returns the
// effective values with defaults applied. A validation failure is returned as a
// 400-worthy error.
func (r *campaignRequest) normalizeScheduling() (publishingTime string, timezone string, days models.StringSlice, spread int, err error) {
	publishingTime = strings.TrimSpace(r.PublishingTime)
	if publishingTime == "" {
		publishingTime = scheduling.DefaultPublishingTime
	} else if !scheduling.ValidClock(publishingTime) {
		return "", "", nil, 0, fmt.Errorf("publishing_time must be HH:MM (24-hour), got %q", r.PublishingTime)
	}

	timezone = strings.TrimSpace(r.Timezone)
	if timezone != "" {
		if _, tzErr := settings.ResolveTimezone(timezone); tzErr != nil {
			return "", "", nil, 0, fmt.Errorf("invalid timezone: %s", timezone)
		}
	}

	if len(r.PublishingDays) == 0 {
		days = scheduling.DefaultPublishingDays()
	} else {
		seen := make(map[string]bool, len(r.PublishingDays))
		days = make(models.StringSlice, 0, len(r.PublishingDays))
		for _, d := range r.PublishingDays {
			tok := strings.ToLower(strings.TrimSpace(d))
			if !scheduling.ValidWeekday(tok) {
				return "", "", nil, 0, fmt.Errorf("invalid publishing day: %q", d)
			}
			if seen[tok] {
				return "", "", nil, 0, fmt.Errorf("duplicate publishing day: %q", tok)
			}
			seen[tok] = true
			days = append(days, tok)
		}
	}

	spread = scheduling.DefaultSpreadMinutes
	if r.SpreadMinutes != nil {
		spread = *r.SpreadMinutes
		if spread < 0 || spread > scheduling.MaxSpreadMinutes {
			return "", "", nil, 0, fmt.Errorf("spread_minutes must be between 0 and %d", scheduling.MaxSpreadMinutes)
		}
	}
	return publishingTime, timezone, days, spread, nil
}

// toStatus resolves the campaign's status, defaulting to active. CON-156 BE 6:
// draft is not a user-facing distinction (nothing behaves differently), so a
// campaign with no explicit status is created active rather than draft. Existing
// draft campaigns keep working — nothing in the backend branches on the value.
func (r *campaignRequest) toStatus() models.CampaignStatus {
	if r.Status == "" {
		return models.StatusActive
	}
	return r.Status
}

// List godoc
// @Summary      List campaigns
// @Description  Returns the active set (neither archived nor deleted) ordered by creation date. Pass ?archived=true to list archived campaigns instead.
// @Tags         campaigns
// @Produce      json
// @Security     CookieAuth
// @Param        archived  query     bool  false  "List archived campaigns instead of the active set"
// @Success      200  {array}   models.Campaign
// @Failure      401  {object}  map[string]string
// @Router       /api/campaigns [get]
func (h *CampaignsHandler) List(c *fiber.Ctx) error {
	list := h.repo.List
	if c.QueryBool("archived", false) {
		list = h.repo.ListArchived
	}
	campaigns, err := list(c.Context())
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
	publishingTime, timezone, publishingDays, spread, err := req.normalizeScheduling()
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	goalCadence, err := campaigngoal.Normalize(strings.TrimSpace(req.GoalCadence))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	session := c.Locals("session").(*models.Session)

	id, err := models.NewID()
	if err != nil {
		return err
	}

	if err := validateBrandRefs(c.Context(), h.brandRepo, req.BrandVoiceID.Value, req.BrandAudienceID.Value); err != nil {
		return err
	}

	campaign := &models.Campaign{
		ID:                 id,
		Name:               req.Name,
		Description:        req.Description,
		TargetPersona:      req.TargetPersona,
		KeyMessages:        req.KeyMessages,
		ToneGuidelines:     req.ToneGuidelines,
		BrandVoiceID:       req.BrandVoiceID.Value,
		BrandAudienceID:    req.BrandAudienceID.Value,
		UseAssets:          req.UseAssets,
		AssetIDs:           nullSlice(req.AssetIDs),
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
		PublishingTime:     publishingTime,
		Timezone:           timezone,
		PublishingDays:     publishingDays,
		SpreadMinutes:      spread,
		GoalCadence:        goalCadence,
		CreatedBy:          session.UserID,
	}
	if err := h.repo.Create(c.Context(), campaign); err != nil {
		return err
	}
	h.recordActivity(c, activity.CategoryCampaign, "campaign_created",
		activity.WithEntity("campaign", campaign.ID),
		activity.WithPayload(map[string]any{"status": string(campaign.Status), "campaign_type_id": campaign.CampaignTypeID}),
	)
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
	publishingTime, timezone, publishingDays, spread, err := req.normalizeScheduling()
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	goalCadence, err := campaigngoal.Normalize(strings.TrimSpace(req.GoalCadence))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	if err := validateBrandRefs(c.Context(), h.brandRepo, req.BrandVoiceID.Value, req.BrandAudienceID.Value); err != nil {
		return err
	}

	campaign, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "campaign not found")
		}
		return err
	}

	// Snapshot the fields we emit change-events for before overwriting them.
	prevStatus := campaign.Status
	prevStart, prevEnd := campaign.StartDate, campaign.EndDate

	campaign.Name = req.Name
	campaign.Description = req.Description
	campaign.TargetPersona = req.TargetPersona
	campaign.KeyMessages = req.KeyMessages
	campaign.ToneGuidelines = req.ToneGuidelines
	// Presence-aware (CON-245): omit to leave alone, explicit null to clear.
	req.BrandVoiceID.applyTo(&campaign.BrandVoiceID)
	req.BrandAudienceID.applyTo(&campaign.BrandAudienceID)
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
	campaign.PublishingTime = publishingTime
	campaign.Timezone = timezone
	campaign.PublishingDays = publishingDays
	campaign.SpreadMinutes = spread
	campaign.GoalCadence = goalCadence
	campaign.UpdatedAt = time.Now().UTC()

	if err := h.repo.Update(c.Context(), campaign); err != nil {
		return err
	}
	h.recordActivity(c, activity.CategoryCampaign, "campaign_updated",
		activity.WithEntity("campaign", campaign.ID),
		activity.WithStatus(string(campaign.Status)),
	)
	if prevStatus != campaign.Status {
		h.recordActivity(c, activity.CategoryCampaign, "campaign_status_changed",
			activity.WithEntity("campaign", campaign.ID),
			activity.WithStatus(string(prevStatus)+"->"+string(campaign.Status)),
		)
	}
	if !timePtrEqual(prevStart, campaign.StartDate) || !timePtrEqual(prevEnd, campaign.EndDate) {
		h.recordActivity(c, activity.CategoryCampaign, "campaign_dates_changed",
			activity.WithEntity("campaign", campaign.ID),
		)
	}
	return c.JSON(campaign)
}

// Delete godoc
// @Summary      Delete campaign
// @Description  Soft-deletes a campaign by Sqid. The row is retained as a safety net (no self-serve restore); it disappears from lists and reads.
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
	h.recordActivity(c, activity.CategoryCampaign, "campaign_deleted",
		activity.WithEntity("campaign", c.Params("id")),
	)
	return c.SendStatus(fiber.StatusNoContent)
}

// Archive godoc
// @Summary      Archive campaign
// @Description  Removes a campaign from the active list. Reversible via unarchive.
// @Tags         campaigns
// @Security     CookieAuth
// @Param        id   path  string  true  "Campaign Sqid"
// @Success      204
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/campaigns/{id}/archive [post]
func (h *CampaignsHandler) Archive(c *fiber.Ctx) error {
	ok, err := h.repo.Archive(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "campaign not found")
	}
	h.recordActivity(c, activity.CategoryCampaign, "campaign_archived",
		activity.WithEntity("campaign", c.Params("id")),
	)
	return c.SendStatus(fiber.StatusNoContent)
}

// Unarchive godoc
// @Summary      Unarchive campaign
// @Description  Returns an archived campaign to the active list.
// @Tags         campaigns
// @Security     CookieAuth
// @Param        id   path  string  true  "Campaign Sqid"
// @Success      204
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/campaigns/{id}/unarchive [post]
func (h *CampaignsHandler) Unarchive(c *fiber.Ctx) error {
	ok, err := h.repo.Unarchive(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "campaign not found")
	}
	h.recordActivity(c, activity.CategoryCampaign, "campaign_unarchived",
		activity.WithEntity("campaign", c.Params("id")),
	)
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
	if h.isContentPlanReady != nil && !h.isContentPlanReady() {
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

	h.recordActivity(c, activity.CategoryCampaign, "content_generated",
		activity.WithEntity("campaign", campaign.ID),
		activity.WithPayload(map[string]any{"mode": "generate_draft"}),
	)

	campaignID := campaign.ID
	generateDraft := h.generateDraft
	// Carry the tenant into the detached flow context (the StreamWriter runs
	// after this handler returns) so usage recording + enforcement attribute
	// to the right tenant (CON-86).
	flowCtx := tenantctx.With(context.Background(), session.TenantID)

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		writeEvent := func(event string, data any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
			_ = w.Flush()
		}

		onEvent := content_plan.OnEventFunc(func(name content_plan.SSEEventKind, data any) {
			writeEvent(string(name), data)
		})

		resp, err := generateDraft(flowCtx, campaignID, onEvent)
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

// EnrichBrief godoc
// @Summary      Enrich campaign brief with AI (SSE)
// @Description  Generates a campaign brief (description, target persona, key messages, tone guidelines)
// @Description  from the campaign's title and type, streaming progress via Server-Sent Events.
// @Description  Per-field "*_delta" events preview each value as it is written; a final "complete"
// @Description  event carries the full EnrichBriefResponse and is the signal that the brief is ready.
// @Description  On failure an "error" event carries {"message":"<text>","code":<http_code>}.
// @Description  The brief is returned as a suggestion only — it is not persisted to the campaign.
// @Tags         campaigns
// @Accept       json
// @Produce      text/event-stream
// @Security     CookieAuth
// @Param        id    path  string  true  "Campaign Sqid"
// @Param        body  body  object  false "Optional steering: {\"instruction\":\"...\"}"
// @Success      200  "SSE stream: step / *_delta / complete / error events"
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      403  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/campaigns/{id}/enrich-brief [post]
func (h *CampaignsHandler) EnrichBrief(c *fiber.Ctx) error {
	if h.enrichBrief == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "brief enrichment feature is not enabled")
	}
	if h.isContentPlanReady != nil && !h.isContentPlanReady() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "brief enrichment feature is not enabled")
	}

	// Body is optional; only parse when present so an empty POST is valid.
	var body struct {
		Instruction string `json:"instruction"`
	}
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
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

	h.recordActivity(c, activity.CategoryCampaign, "brief_enriched",
		activity.WithEntity("campaign", campaign.ID),
	)

	req := enrich_brief.EnrichBriefRequest{CampaignID: campaign.ID, Instruction: body.Instruction}
	enrichBrief := h.enrichBrief
	flowCtx := tenantctx.With(context.Background(), session.TenantID)

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		writeEvent := func(event string, data any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
			_ = w.Flush()
		}

		onEvent := enrich_brief.OnEventFunc(func(name enrich_brief.SSEEventKind, data any) {
			writeEvent(string(name), data)
		})

		resp, err := enrichBrief(flowCtx, req, onEvent)
		if err != nil {
			code := fiber.StatusInternalServerError
			msg := err.Error()
			var ve *enrich_brief.ValidationError
			var ae *enrich_brief.AIError
			switch {
			case errors.As(err, &ve):
				code = fiber.StatusBadRequest
				msg = ve.Msg
			case errors.As(err, &ae):
				code = fiber.StatusBadGateway
				msg = ae.Msg
			}
			writeEvent(string(enrich_brief.SSEEventError), enrich_brief.ErrorEventPayload{Message: msg, Code: code})
			return
		}

		writeEvent(string(enrich_brief.SSEEventComplete), resp)
	}))

	return nil
}

// Assistant godoc
// @Summary      Campaign assistant (SSE)
// @Description  Sends an instruction to the Campaign Assistant and streams progress via Server-Sent Events.
// @Description  "explanation_delta" carries {"delta":"..."} fragments as the reply streams. "tool_call"/"tool_result"
// @Description  signal tool invocations. When a content plan runs, "content_plan_started", "content_plan_post"
// @Description  (etc.) and "content_plan_complete" are forwarded; when the brief is enriched, "enrich_brief_started",
// @Description  the per-field "enrich_brief_*_delta" events and "enrich_brief_complete" are forwarded. A final
// @Description  "complete" event carries the CampaignAssistantResponse. "error" carries {"message":"...","code":<http_code>}.
// @Description  Available to any user in the campaign's tenant.
// @Tags         campaigns
// @Accept       json
// @Produce      text/event-stream
// @Security     CookieAuth
// @Param        id    path      string           true  "Campaign Sqid"
// @Param        body  body      assistantRequest true  "Instruction payload"
// @Success      200  "SSE stream: delta / tool_call / tool_result / *_complete / complete / error events"
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      503   {object}  map[string]string
// @Router       /api/campaigns/{id}/assistant [post]
func (h *CampaignsHandler) Assistant(c *fiber.Ctx) error {
	if h.assistant == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "campaign assistant is not available")
	}
	if h.isContentPlanReady != nil && !h.isContentPlanReady() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "campaign assistant is not available")
	}

	var req assistantRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	h.recordActivity(c, activity.CategoryAIFlow, "campaign_assistant_turn",
		activity.WithEntity("campaign", c.Params("id")),
		activity.WithSource(activity.SourceAssistant),
	)

	// Copy the buffer-backed request values: the StreamWriter runs after this
	// handler returns, by which point fasthttp may have recycled the request
	// buffer into a concurrent request and corrupted them. See the post
	// assistant handler for the failure this prevents.
	campaignID := strings.Clone(c.Params("id"))
	instruction := strings.Clone(req.Instruction)
	assistant := h.assistant
	session := c.Locals("session").(*models.Session)
	// Carry the tenant into the detached flow context (the StreamWriter runs
	// after this handler returns) so the tenant-scoped campaign load, brief
	// write, and usage recording all attribute to the right tenant (CON-86/97).
	flowCtx := tenantctx.With(context.Background(), session.TenantID)

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		writeEvent := func(event string, data any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
			_ = w.Flush()
		}

		onEvent := campaign_assistant.OnEventFunc(func(name campaign_assistant.SSEEventKind, data any) {
			writeEvent(string(name), data)
		})

		_, err := assistant(flowCtx, campaign_assistant.CampaignAssistantRequest{
			CampaignID:  campaignID,
			Instruction: instruction,
		}, onEvent)
		if err != nil {
			code := fiber.StatusInternalServerError
			msg := err.Error()
			var ve *campaign_assistant.ValidationError
			var ae *campaign_assistant.AIError
			switch {
			case errors.As(err, &ve):
				code = fiber.StatusBadRequest
				msg = ve.Msg
			case errors.As(err, &ae):
				code = fiber.StatusBadGateway
				msg = ae.Msg
			}
			writeEvent(string(campaign_assistant.SSEEventError), campaign_assistant.ErrorEventPayload{Message: msg, Code: code})
			return
		}
		// "complete" is emitted by the runner itself; nothing to write here.
	}))

	return nil
}

// ListMessages godoc
// @Summary      List campaign assistant messages
// @Description  Returns the most recent Campaign Assistant conversation messages for a campaign.
// @Tags         campaigns
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "Campaign Sqid"
// @Success      200  {array}   models.CampaignAssistantMessage
// @Failure      401  {object}  map[string]string
// @Router       /api/campaigns/{id}/messages [get]
func (h *CampaignsHandler) ListMessages(c *fiber.Ctx) error {
	if h.messageRepo == nil {
		return c.JSON([]models.CampaignAssistantMessage{})
	}
	msgs, err := h.messageRepo.ListRecentByCampaignID(c.Context(), c.Params("id"), 50)
	if err != nil {
		return err
	}
	// Preserve the array contract: an empty history serializes as [] not null.
	if msgs == nil {
		msgs = []models.CampaignAssistantMessage{}
	}
	return c.JSON(msgs)
}

// Overview godoc
// @Summary      Campaign overview
// @Description  Returns a quick overview of the campaign: brief recap, phases
// @Description  (with per-phase post counts), and content distribution by
// @Description  status, platform, and content type. Available to any user in
// @Description  the campaign's tenant.
// @Tags         campaigns
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "Campaign Sqid"
// @Success      200  {object}  overview.Overview
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/campaigns/{id}/overview [get]
func (h *CampaignsHandler) Overview(c *fiber.Ctx) error {
	if h.overview == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "campaign overview is not available")
	}
	ov, err := h.overview.Overview(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, overview.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "campaign not found")
		}
		return err
	}
	return c.JSON(ov)
}

// Summaries godoc
// @Summary      Batched campaign post summaries
// @Description  Returns a slim per-post projection for every campaign in the
// @Description  tenant, grouped by campaign (CON-152). The Campaigns list runs
// @Description  its readiness rules on this single response instead of firing
// @Description  one GET /campaigns/:id/posts per card. Carries only the fields
// @Description  the rules read — no title, content, or hydrated relations.
// @Tags         campaigns
// @Produce      json
// @Security     CookieAuth
// @Success      200  {object}  summaries.Summaries
// @Failure      401  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/campaigns/summaries [get]
func (h *CampaignsHandler) Summaries(c *fiber.Ctx) error {
	if h.summaries == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "campaign summaries are not available")
	}
	out, err := h.summaries.Summaries(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(out)
}

// GeneratePosts godoc
// @Summary      Generate targeted posts (SSE)
// @Description  Generates a few new draft posts for a platform subset, a single
// @Description  phase, and a publish-date window (CON-114). Streams the same
// @Description  step / post / warning / complete / error events as generate-draft.
// @Description  Available to any user in the campaign's tenant.
// @Tags         campaigns
// @Accept       json
// @Produce      text/event-stream
// @Security     CookieAuth
// @Param        id    path  string  true  "Campaign Sqid"
// @Success      200  "SSE stream: step / post / warning / complete / error events"
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/campaigns/{id}/generate-posts [post]
func (h *CampaignsHandler) GeneratePosts(c *fiber.Ctx) error {
	if h.generatePosts == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "targeted generation is not available")
	}
	if h.isContentPlanReady != nil && !h.isContentPlanReady() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "targeted generation is not available")
	}

	var body struct {
		PlatformIDs []string `json:"platformIds"`
		PhaseID     string   `json:"phaseId"`
		Count       int      `json:"count"`
		WindowStart string   `json:"windowStart"`
		WindowEnd   string   `json:"windowEnd"`
		PostType    string   `json:"postType"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if body.Count < 1 {
		return fiber.NewError(fiber.StatusBadRequest, "count must be at least 1")
	}
	max := h.generatePostsMax
	if max <= 0 {
		max = 10
	}
	if body.Count > max {
		body.Count = max
	}

	// 404 before opening the stream (tenant-scoped).
	if _, err := h.repo.GetByID(c.Context(), c.Params("id")); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "campaign not found")
		}
		return err
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	h.recordActivity(c, activity.CategoryCampaign, "content_generated",
		activity.WithEntity("campaign", c.Params("id")),
		activity.WithPayload(map[string]any{"mode": "generate_posts", "count": body.Count}),
	)

	session := c.Locals("session").(*models.Session)
	flowCtx := tenantctx.With(context.Background(), session.TenantID)
	generatePosts := h.generatePosts
	req := content_plan.GeneratePostsRequest{
		// Copied: the StreamWriter below outlives the request buffer this id
		// points into (see the post assistant handler).
		CampaignID:  strings.Clone(c.Params("id")),
		PlatformIDs: body.PlatformIDs,
		PhaseID:     body.PhaseID,
		Count:       body.Count,
		WindowStart: body.WindowStart,
		WindowEnd:   body.WindowEnd,
		PostType:    body.PostType,
	}

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		writeEvent := func(event string, data any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
			_ = w.Flush()
		}

		onEvent := content_plan.OnEventFunc(func(name content_plan.SSEEventKind, data any) {
			writeEvent(string(name), data)
		})

		resp, err := generatePosts(flowCtx, req, onEvent)
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

// BriefReview godoc
// @Summary      Review the campaign brief for consistency (SSE)
// @Description  Read-only review of the brief's internal consistency and
// @Description  completeness (CON-116). Streams step / complete / error events.
// @Description  Does not modify the brief. Available to any user in the tenant.
// @Tags         campaigns
// @Produce      text/event-stream
// @Security     CookieAuth
// @Param        id   path  string  true  "Campaign Sqid"
// @Success      200  "SSE stream: step / complete / error events"
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/campaigns/{id}/brief-review [post]
func (h *CampaignsHandler) BriefReview(c *fiber.Ctx) error {
	if h.checkBrief == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "brief review is not available")
	}
	if h.isContentPlanReady != nil && !h.isContentPlanReady() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "brief review is not available")
	}

	// 404 before opening the stream (tenant-scoped).
	if _, err := h.repo.GetByID(c.Context(), c.Params("id")); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "campaign not found")
		}
		return err
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	session := c.Locals("session").(*models.Session)
	flowCtx := tenantctx.With(context.Background(), session.TenantID)
	h.recordActivity(c, activity.CategoryAIFlow, "brief_review",
		activity.WithEntity("campaign", c.Params("id")),
	)

	checkBrief := h.checkBrief
	// Copied: the StreamWriter outlives the request buffer (see post assistant).
	campaignID := strings.Clone(c.Params("id"))

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		writeEvent := func(event string, data any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
			_ = w.Flush()
		}

		onEvent := consistency.OnEventFunc(func(name consistency.SSEEventKind, data any) {
			writeEvent(string(name), data)
		})

		resp, err := checkBrief(flowCtx, campaignID, onEvent)
		if err != nil {
			writeEvent(string(consistency.SSEEventError), consistencyError(err))
			return
		}
		writeEvent(string(consistency.SSEEventComplete), resp)
	}))

	return nil
}

// PostsReview godoc
// @Summary      Review campaign posts against the brief (SSE)
// @Description  Read-only check of whether the campaign's non-published posts
// @Description  follow the brief (CON-116). Only the first N posts are checked.
// @Description  Streams step / complete / error events. Does not modify any post.
// @Tags         campaigns
// @Accept       json
// @Produce      text/event-stream
// @Security     CookieAuth
// @Param        id   path  string  true  "Campaign Sqid"
// @Success      200  "SSE stream: step / complete / error events"
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/campaigns/{id}/posts-review [post]
func (h *CampaignsHandler) PostsReview(c *fiber.Ctx) error {
	if h.checkPosts == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "posts review is not available")
	}
	if h.isContentPlanReady != nil && !h.isContentPlanReady() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "posts review is not available")
	}

	var body struct {
		Max int `json:"max"`
	}
	// Body is optional — an empty POST reviews with the default cap.
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	}

	// 404 before opening the stream (tenant-scoped).
	if _, err := h.repo.GetByID(c.Context(), c.Params("id")); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "campaign not found")
		}
		return err
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	session := c.Locals("session").(*models.Session)
	flowCtx := tenantctx.With(context.Background(), session.TenantID)
	h.recordActivity(c, activity.CategoryAIFlow, "posts_review",
		activity.WithEntity("campaign", c.Params("id")),
	)

	checkPosts := h.checkPosts
	// Copied: the StreamWriter outlives the request buffer (see post assistant).
	req := consistency.PostsCheckRequest{CampaignID: strings.Clone(c.Params("id")), Max: body.Max}

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		writeEvent := func(event string, data any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
			_ = w.Flush()
		}

		onEvent := consistency.OnEventFunc(func(name consistency.SSEEventKind, data any) {
			writeEvent(string(name), data)
		})

		resp, err := checkPosts(flowCtx, req, onEvent)
		if err != nil {
			writeEvent(string(consistency.SSEEventError), consistencyError(err))
			return
		}
		writeEvent(string(consistency.SSEEventComplete), resp)
	}))

	return nil
}

// consistencyError maps a consistency flow error onto the SSE error payload,
// classifying validation vs. AI-provider failures for the client.
func consistencyError(err error) consistency.ErrorEventPayload {
	code := fiber.StatusInternalServerError
	msg := err.Error()
	var ve *consistency.ValidationError
	var ae *consistency.AIError
	switch {
	case errors.As(err, &ve):
		code = fiber.StatusBadRequest
		msg = ve.Msg
	case errors.As(err, &ae):
		code = fiber.StatusBadGateway
		msg = ae.Msg
	}
	return consistency.ErrorEventPayload{Message: msg, Code: code}
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
