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

	"github.com/mikestefanello/backlite"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/genkit/flows/post_assistant"
	"github.com/ogen-app/ogen/src/genkit/flows/post_quality"
	"github.com/ogen-app/ogen/src/jobs/queues"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/platforms"
	"github.com/ogen-app/ogen/src/post_actions/clone"
	"github.com/ogen-app/ogen/src/post_actions/logs"
	"github.com/ogen-app/ogen/src/post_actions/restore"
	"github.com/ogen-app/ogen/src/post_actions/schedule"
	"github.com/ogen-app/ogen/src/repository"
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
	repo           repository.PostRepository
	versionRepo    repository.PostVersionRepository
	messageRepo    repository.PostAssistantMessageRepository
	platformRepo   repository.PlatformRepository
	attachmentRepo repository.PostAttachmentRepository
	auth           fiber.Handler
	assistant      func(ctx context.Context, req post_assistant.PostAssistantRequest, onEvent post_assistant.OnEventFunc) (*post_assistant.PostAssistantResponse, error)
	// assessQuality runs the Post quality assessment agent (CON-85). nil
	// makes the /assess endpoint return 503. Wired via SetQualityAssessor.
	assessQuality func(ctx context.Context, postID string, onEvent post_quality.OnEventFunc) (*post_quality.PostQualityResponse, error)
	// evaluationRepo serves the cached assessment read (CON-92). nil makes
	// GET /:id/assessment return 503. Wired via SetEvaluationRepo.
	evaluationRepo repository.PostEvaluationRepository
	// analyticsRepo serves the per-post analytics snapshot read (CON-93
	// FR4, GET /:id/analytics). nil makes that endpoint return 503. Wired
	// via SetAnalyticsRepo.
	analyticsRepo repository.PostAnalyticsRepository
	// isAssistantReady reports whether the Anthropic key is currently
	// configured. nil is treated as "always ready" so existing test
	// fixtures keep working without rewiring.
	isAssistantReady func() bool
	// onBeforeDelete runs before the post row is deleted. The server
	// wires this to clean up S3 objects belonging to the post's
	// attachments (CON-73 §2.7 — "all of its attachments are deleted
	// from S3 immediately as part of the same operation"). nil is
	// treated as no-op for fixtures that don't care about attachments.
	onBeforeDelete func(ctx context.Context, postID string) error
	// postLogRepo records every meaningful operation against a Post
	// (CON-69 §11). nil makes log writes no-ops so legacy fixtures stay
	// green.
	postLogRepo repository.PostLogRepository
	// allowlistRepo answers "is this Zernio platform allowed to
	// auto-publish?" when the user moves a post to Scheduled
	// (CON-69 §5). nil disables the allowlist branch entirely;
	// posts go straight to Scheduled with no Backlite enqueue.
	allowlistRepo repository.AutoPublishAllowlistRepository
	// jobsClient is the Backlite client used to enqueue submit tasks
	// transactionally with the status change. nil disables enqueue.
	jobsClient *backlite.Client
	// db is the Bun DB handle. Held so the schedule path can run the
	// status update + PostLog write + Backlite enqueue in a single
	// SQLite transaction (CON-69 §5).
	db *bun.DB
	// cloneSvc duplicates a post (CON-59). nil disables the clone
	// endpoint (503), keeping fixtures that don't wire it green.
	cloneSvc *clone.Service
	// restoreSvc rolls a post back to an earlier version (CON-68). nil
	// disables the restore endpoint (503).
	restoreSvc *restore.Service
	// scheduleSvc schedules a post for publishing (CON-78): the single
	// source of truth for allowlist routing + transactional persist +
	// Zernio enqueue, shared by POST /:id/schedule, the assistant's
	// schedulePost tool, and the PUT scheduling branch. nil disables the
	// schedule endpoint (503) and makes the PUT branch fall back to a
	// plain status update (test fixtures that don't wire scheduling).
	scheduleSvc *schedule.Service
}

// SetOnBeforeDelete registers a hook that runs before a post is
// deleted. Used by the server to clean up post-attachment S3 objects
// without forcing every PostsHandler caller to know about attachments.
func (h *PostsHandler) SetOnBeforeDelete(fn func(ctx context.Context, postID string) error) {
	h.onBeforeDelete = fn
}

// SetAttachmentRepo wires the repository the validation gate consults
// when transitioning Draft→ReadyForPublish. Until set, the gate is a
// no-op (used by fixtures that don't exercise the publish path).
func (h *PostsHandler) SetAttachmentRepo(r repository.PostAttachmentRepository) {
	h.attachmentRepo = r
}

// SetPostLogRepo wires the audit repository. Until set, every PostLog
// write performed by this handler is a no-op (used by handler-test
// fixtures that don't care about the audit trail).
func (h *PostsHandler) SetPostLogRepo(r repository.PostLogRepository) {
	h.postLogRepo = r
}

// SetQualityAssessor wires the Post quality assessment callback (CON-85).
// Kept as a setter (not a constructor arg) so existing NewPostsHandler
// call sites and test fixtures stay unchanged.
func (h *PostsHandler) SetQualityAssessor(fn func(ctx context.Context, postID string, onEvent post_quality.OnEventFunc) (*post_quality.PostQualityResponse, error)) {
	h.assessQuality = fn
}

// SetEvaluationRepo wires the repository backing the cached assessment read
// (CON-92, GET /:id/assessment). Setter (not a constructor arg) so existing
// NewPostsHandler call sites and fixtures stay unchanged.
func (h *PostsHandler) SetEvaluationRepo(r repository.PostEvaluationRepository) {
	h.evaluationRepo = r
}

// SetAnalyticsRepo wires the repository backing the per-post analytics
// read (CON-93 FR4, GET /:id/analytics). nil leaves the endpoint at 503.
func (h *PostsHandler) SetAnalyticsRepo(r repository.PostAnalyticsRepository) {
	h.analyticsRepo = r
}

// SetSchedulingDeps wires everything the schedule path needs: the
// allowlist repo (to choose Scheduled vs ScheduledForManualPublish),
// the Backlite client (to enqueue submit tasks), and the bun DB
// handle (to do the status change + PostLog write + enqueue in one
// SQLite transaction). Any nil disables the schedule branch — useful
// for fixtures that don't exercise auto-publish.
func (h *PostsHandler) SetSchedulingDeps(allowlist repository.AutoPublishAllowlistRepository, client *backlite.Client, db *bun.DB) {
	h.allowlistRepo = allowlist
	h.jobsClient = client
	h.db = db
}

// SetCloneService wires the post-clone service (CON-59). Until set, the
// clone endpoint returns 503.
func (h *PostsHandler) SetCloneService(s *clone.Service) {
	h.cloneSvc = s
}

// SetRestoreService wires the post-restore service (CON-68). Until set,
// the restore endpoint returns 503.
func (h *PostsHandler) SetRestoreService(s *restore.Service) {
	h.restoreSvc = s
}

// SetScheduleService wires the post-schedule service (CON-78). Until set,
// the schedule endpoint returns 503 and the PUT scheduling branch falls
// back to a plain status update.
func (h *PostsHandler) SetScheduleService(s *schedule.Service) {
	h.scheduleSvc = s
}

// logEvent appends a PostLog entry, swallowing repo errors so logging
// failures never leak through to the user-facing response. Emitting
// an audit entry is best-effort by design — losing one log line
// matters less than failing the operation it describes.
func (h *PostsHandler) logEvent(c *fiber.Ctx, postID string, eventType models.PostLogEventType, fromStatus, toStatus *models.PostStatus, summary, payload string) {
	if h.postLogRepo == nil {
		return
	}
	id, err := models.NewID()
	if err != nil {
		return
	}
	actor := models.ActorSystem
	if sess, ok := c.Locals("session").(*models.Session); ok && sess != nil {
		actor = sess.UserID
	}
	_ = h.postLogRepo.Append(c.Context(), &models.PostLog{
		ID:         id,
		PostID:     postID,
		EventType:  eventType,
		Actor:      actor,
		FromStatus: fromStatus,
		ToStatus:   toStatus,
		Summary:    summary,
		Payload:    logs.SanitizeAndCap(payload),
	})
}

// hasAnyErrors reports whether the platform→errors map has at least
// one non-empty error list. ValidateForPublish always populates an
// entry per platform with rules; an empty list means "passes".
func hasAnyErrors(m map[string][]platforms.ValidationError) bool {
	for _, v := range m {
		if len(v) > 0 {
			return true
		}
	}
	return false
}

// validateReadyForPublish runs the CON-69 §4 attachment gate when a
// Draft is moving to ReadyForPublish. Returns done=true when the gate
// has already written the 422 response and the caller must stop. A
// non-nil error means a transient repository failure — the caller
// should bubble it up. done=false, err=nil means the gate passed (or
// did not apply) and the caller should continue.
func (h *PostsHandler) validateReadyForPublish(c *fiber.Ctx, post *models.Post, req *postRequest, status models.PostStatus) (done bool, err error) {
	if post.Status != models.PostStatusDraft || status != models.PostStatusReadyForPublish || h.attachmentRepo == nil {
		return false, nil
	}
	atts, err := h.attachmentRepo.ListByPostID(c.Context(), post.ID)
	if err != nil {
		return false, err
	}
	// Validate against the new (incoming) platform, not the prior one,
	// since a Draft can switch platforms in the same Update call. The
	// post row hasn't been written yet, so refetching via repo.GetByID
	// would just return the stale platform — go straight to the
	// platform repository when the request changed it.
	platform := post.Platform
	if req.PlatformID != "" && (platform == nil || platform.ID != req.PlatformID) && h.platformRepo != nil {
		fresh, perr := h.platformRepo.GetByID(c.Context(), req.PlatformID)
		if perr == nil {
			platform = fresh
		}
	}
	errsByPlatform := platforms.ValidateForPublish(atts, []*models.Platform{platform})
	// CON-74: per-post-type rules (whitelist + content/min/max-attachments/kind).
	// Validates against the incoming request's content/post_type so the
	// gate sees what's about to be persisted, not the prior draft. Errors
	// are folded into the same per-platform map for one unified response.
	if platform != nil {
		incoming := *post
		incoming.PlatformPostType = req.PlatformPostType
		incoming.Content = req.Content
		if typeErrs := platforms.ValidatePostType(&incoming, platform, atts); len(typeErrs) > 0 {
			errsByPlatform[platform.ID] = append(errsByPlatform[platform.ID], typeErrs...)
		}
	}
	from := post.Status
	if hasAnyErrors(errsByPlatform) {
		h.logEvent(c, post.ID, models.PostLogEventValidationFailed, &from, &status,
			"draft → ready_for_publish blocked by platform validation",
			logs.MarshalCapped(map[string]any{"platform_validation": errsByPlatform}),
		)
		if err := c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error":               "post is not ready for publish",
			"platform_validation": errsByPlatform,
		}); err != nil {
			return true, err
		}
		return true, nil
	}
	h.logEvent(c, post.ID, models.PostLogEventValidationPassed, &from, &status,
		"draft → ready_for_publish passed platform validation",
		logs.MarshalCapped(map[string]any{"platform_validation": errsByPlatform}),
	)
	return false, nil
}

// logTransition writes the §11 state-transition entry (and the §10
// manual-retry entry, when applicable) for a successful Update. A
// status that didn't actually change is not a state-machine event and
// produces no log lines.
func (h *PostsHandler) logTransition(c *fiber.Ctx, post *models.Post, prev, next models.PostStatus) {
	if prev == next {
		return
	}
	h.logEvent(c, post.ID, models.PostLogEventStateTransition, &prev, &next,
		"status changed via PUT /api/posts/:id", "{}",
	)
	if prev == models.PostStatusFailed && next == models.PostStatusReadyForPublish {
		h.logEvent(c, post.ID, models.PostLogEventUserRetry, &prev, &next,
			"manual retry: user moved Failed → ReadyForPublish",
			logs.MarshalCapped(map[string]any{
				"prior_failure_reason":    post.FailureReason,
				"prior_publisher_post_id": post.PublisherPostID,
			}),
		)
	}
}

// scheduleRequest is the body for POST /api/posts/:id/schedule. The
// instant is absolute (relative expressions like "tomorrow 9am" are
// resolved by the assistant before it calls the shared service; the
// REST endpoint takes the resolved value directly).
type scheduleRequest struct {
	ScheduledAt  *time.Time `json:"scheduled_at"`
	AllowPromote bool       `json:"allow_promote"`
}

// Schedule godoc
// @Summary      Schedule a post for publishing
// @Description  Schedules a `ready_for_publish` post for the given absolute
// @Description  time (CON-78). Allowlisted platforms route to `scheduled`
// @Description  (auto-publish, Zernio submit enqueued); others route to
// @Description  `scheduled_for_manual_publishing`. Pass `allow_promote` to
// @Description  auto-promote a `draft` (runs CON-74 pre-publish validation,
// @Description  then Draft → ReadyForPublish) before scheduling. The status
// @Description  change, `scheduled_at`, audit entries, and Zernio enqueue
// @Description  commit in one transaction.
// @Tags         posts
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string           true  "Post Sqid"
// @Param        body  body      scheduleRequest  true  "Schedule payload"
// @Success      200   {object}  map[string]interface{}
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      422   {object}  map[string]interface{}
// @Failure      503   {object}  map[string]string
// @Router       /api/posts/{id}/schedule [post]
func (h *PostsHandler) Schedule(c *fiber.Ctx) error {
	if h.scheduleSvc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "scheduling is not available")
	}
	var req scheduleRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if req.ScheduledAt == nil {
		return fiber.NewError(fiber.StatusBadRequest, "scheduled_at is required")
	}

	session := c.Locals("session").(*models.Session)
	res, err := h.scheduleSvc.Schedule(c.Context(), c.Params("id"), schedule.Options{
		ScheduledAt:  *req.ScheduledAt,
		AllowPromote: req.AllowPromote,
		Actor:        session.UserID,
		Trigger:      schedule.TriggerAPI,
	})
	if err != nil {
		var verr *schedule.ValidationError
		switch {
		case errors.Is(err, schedule.ErrPostNotFound):
			return fiber.NewError(fiber.StatusNotFound, "post not found")
		case errors.As(err, &verr):
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"error":               "post is not ready for publish",
				"platform_validation": verr.Errors,
			})
		case errors.Is(err, schedule.ErrScheduledAtRequired),
			errors.Is(err, schedule.ErrScheduledAtInPast),
			errors.Is(err, schedule.ErrNoPlatform),
			errors.Is(err, schedule.ErrNotSchedulable):
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return err
	}

	// Re-fetch so the response carries a fully hydrated post (campaign /
	// platform / assets), matching the Update/Restore handlers' contract.
	updated, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"post":         updated,
		"status":       string(res.Status),
		"auto_publish": res.AutoPublish,
		"promoted":     res.Promoted,
	})
}

// cancelRequest is the body shape for POST /api/posts/:id/cancel.
type cancelRequest struct {
	// Target is the status the user wants the post moved to once
	// Zernio confirms the cancellation. Defaults to "ready_for_publish"
	// when omitted (CON-69 §9 — ReadyForPublish is the most common
	// "cancel and edit" landing state).
	Target string `json:"target"`
}

// Cancel godoc
// @Summary      Cancel a Scheduled post
// @Description  Enqueues a Backlite cancel_zernio_job task. The Post
// @Description  remains in `Scheduled` until Zernio confirms the
// @Description  cancellation; on confirmation it transitions to the
// @Description  requested target (`ready_for_publish` or `draft`).
// @Description  If Zernio reports the job has already been published
// @Description  (race), the cancellation is a no-op and the next poll
// @Description  cycle lands `Published` per the normal success path
// @Description  (CON-69 §9).
// @Tags         posts
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string         true  "Post Sqid"
// @Param        body  body      cancelRequest  false "Cancellation target"
// @Success      202   {object}  map[string]string
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Failure      503   {object}  map[string]string
// @Router       /api/posts/{id}/cancel [post]
func (h *PostsHandler) Cancel(c *fiber.Ctx) error {
	if h.jobsClient == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "background job runtime not configured")
	}
	post, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "post not found")
		}
		return err
	}
	if post.Status != models.PostStatusScheduled {
		return fiber.NewError(fiber.StatusConflict,
			"only Scheduled posts can be cancelled (current status: "+string(post.Status)+")")
	}

	var req cancelRequest
	_ = c.BodyParser(&req) // body is optional
	target := queues.CancelTargetReadyForPublish
	if req.Target != "" {
		switch queues.CancelTarget(req.Target) {
		case queues.CancelTargetReadyForPublish, queues.CancelTargetDraft:
			target = queues.CancelTarget(req.Target)
		default:
			return fiber.NewError(fiber.StatusBadRequest,
				`target must be "ready_for_publish" or "draft"`)
		}
	}

	actor := models.ActorSystem
	if sess, ok := c.Locals("session").(*models.Session); ok && sess != nil {
		actor = sess.UserID
	}

	if _, err := h.jobsClient.Add(queues.CancelZernioJobTask{
		PostID: post.ID,
		Target: target,
		Actor:  actor,
	}).Save(); err != nil {
		return fmt.Errorf("cancel: enqueue: %w", err)
	}

	h.logEvent(c, post.ID, models.PostLogEventUserCancel, &post.Status, &post.Status,
		"user requested cancellation; cancel_zernio_job enqueued",
		logs.MarshalCapped(map[string]any{"target": string(target)}),
	)

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"status":  "cancellation_enqueued",
		"target":  string(target),
		"post_id": post.ID,
	})
}

func NewPostsHandler(
	repo repository.PostRepository,
	versionRepo repository.PostVersionRepository,
	messageRepo repository.PostAssistantMessageRepository,
	platformRepo repository.PlatformRepository,
	attachmentRepo repository.PostAttachmentRepository,
	auth fiber.Handler,
	assistant func(ctx context.Context, req post_assistant.PostAssistantRequest, onEvent post_assistant.OnEventFunc) (*post_assistant.PostAssistantResponse, error),
	isAssistantReady func() bool,
) *PostsHandler {
	return &PostsHandler{
		repo:             repo,
		versionRepo:      versionRepo,
		messageRepo:      messageRepo,
		platformRepo:     platformRepo,
		attachmentRepo:   attachmentRepo,
		auth:             auth,
		assistant:        assistant,
		isAssistantReady: isAssistantReady,
	}
}

func (h *PostsHandler) Register(app *fiber.App) {
	g := app.Group("/api/posts")
	g.Get("/", h.auth, h.List)
	g.Post("/", h.auth, h.Create)
	g.Get("/:id", h.auth, h.Get)
	g.Put("/:id", h.auth, h.Update)
	g.Delete("/:id", h.auth, h.Delete)
	g.Post("/:id/assistant", h.auth, h.Assistant)
	g.Post("/:id/assess", h.auth, h.Assess)
	g.Get("/:id/assessment", h.auth, h.GetAssessment)
	g.Get("/:id/analytics", h.auth, h.GetAnalytics)
	g.Post("/:id/clone", h.auth, h.Clone)
	g.Get("/:id/messages", h.auth, h.ListMessages)
	g.Get("/:id/versions", h.auth, h.ListVersions)
	g.Post("/:id/versions", h.auth, h.CreateVersion)
	g.Post("/:id/restore", h.auth, h.Restore)
	g.Post("/:id/schedule", h.auth, h.Schedule)
	g.Post("/:id/cancel", h.auth, h.Cancel)

	app.Get("/api/campaigns/:campaign_id/posts", h.auth, h.ListByCampaign)
}

type postRequest struct {
	CampaignID string `json:"campaign_id"             validate:"required"`
	// PlatformID + PlatformPostType are required only when status is not
	// "draft" — see requirePlatformIfNotDraft below. Drafts can be saved
	// before the user has picked a platform (CON-60).
	PlatformID          string             `json:"platform_id"`
	PlatformPostType    string             `json:"platform_post_type"`
	Title               string             `json:"title"`
	Content             string             `json:"content"`
	MediaURLs           models.StringSlice `json:"media_urls"`
	ScheduledAt         *time.Time         `json:"scheduled_at"`
	PublishedAt         *time.Time         `json:"published_at"`
	Status              models.PostStatus  `json:"status"`
	CTAType             models.PostCTAType `json:"cta_type"`
	CTAUrl              string             `json:"cta_url"`
	TargetAudienceNotes string             `json:"target_audience_notes"`
	UsedAssetIDs        models.StringSlice `json:"used_asset_ids"`
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

// apply copies the mutable fields from a parsed request onto an
// existing Post, including the resolved status/ctaType (passed
// separately because they're already normalized by toStatus/toCTAType
// and re-validated by the caller) and a fresh UpdatedAt.
func (r *postRequest) apply(post *models.Post, status models.PostStatus, ctaType models.PostCTAType) {
	post.CampaignID = r.CampaignID
	post.PlatformID = r.PlatformID
	post.PlatformPostType = r.PlatformPostType
	post.Title = r.Title
	post.Content = r.Content
	post.MediaURLs = nullSlice(r.MediaURLs)
	post.ScheduledAt = r.ScheduledAt
	post.PublishedAt = r.PublishedAt
	post.Status = status
	post.CTAType = ctaType
	post.CTAUrl = r.CTAUrl
	post.CampaignTypePhaseID = r.CampaignTypePhaseID
	post.TargetAudienceNotes = r.TargetAudienceNotes
	post.UsedAssetIDs = nullSlice(r.UsedAssetIDs)
	post.UpdatedAt = time.Now().UTC()
}

// requirePlatformIfNotDraft enforces that platform fields are populated
// for any status other than draft. Drafts can sit without a platform
// chosen so the user can write content first and pick a platform later;
// once they're moving the post toward publication, both fields are
// mandatory.
func requirePlatformIfNotDraft(status models.PostStatus, platformID, platformPostType string) error {
	if status == models.PostStatusDraft {
		return nil
	}
	if platformID == "" {
		return fmt.Errorf("platform_id is required when status is %q", status)
	}
	if platformPostType == "" {
		return fmt.Errorf("platform_post_type is required when status is %q", status)
	}
	return nil
}

// validateForCreate runs the publish gate (CON-69 §4 attachment rules
// + CON-74 §post-type rules) when a post is being created directly in
// a non-draft state. Returns done=true after writing a 422 so the
// caller stops; done=false means the gate passed (or did not apply)
// and the caller may continue. New posts have no attachments yet, so
// only the structural rules can fire.
func (h *PostsHandler) validateForCreate(c *fiber.Ctx, post *models.Post) (done bool, err error) {
	if post.Status == models.PostStatusDraft || post.PlatformID == "" || h.platformRepo == nil {
		return false, nil
	}
	platform, err := h.platformRepo.GetByID(c.Context(), post.PlatformID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, fiber.NewError(fiber.StatusBadRequest, "platform not found")
		}
		return false, err
	}
	errsByPlatform := platforms.ValidateForPublish(nil, []*models.Platform{platform})
	if typeErrs := platforms.ValidatePostType(post, platform, nil); len(typeErrs) > 0 {
		errsByPlatform[platform.ID] = append(errsByPlatform[platform.ID], typeErrs...)
	}
	if !hasAnyErrors(errsByPlatform) {
		return false, nil
	}
	if err := c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
		"error":               "post is not ready for publish",
		"platform_validation": errsByPlatform,
	}); err != nil {
		return true, err
	}
	return true, nil
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
// @Description  The `content` field is a Markdown string; the frontend renders it via BlockNote.
// @Description  `platform_id` and `platform_post_type` are required only when `status` is not `draft`;
// @Description  drafts can be saved without a platform chosen up front.
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
	if err := requirePlatformIfNotDraft(status, req.PlatformID, req.PlatformPostType); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
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
		UsedAssetIDs:        nullSlice(req.UsedAssetIDs),
		CampaignTypePhaseID: req.CampaignTypePhaseID,
		CreatedBy:           session.UserID,
		UsedAssets:          []models.Asset{},
	}

	if done, err := h.validateForCreate(c, post); err != nil {
		return err
	} else if done {
		return nil
	}

	if err := h.repo.Create(c.Context(), post); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(post)
}

// cloneRequest is the (entirely optional) body for the clone endpoint.
// An empty body duplicates the post verbatim into the same campaign.
type cloneRequest struct {
	TargetPlatformID string  `json:"target_platform_id"`
	TargetPostType   string  `json:"target_post_type"`
	Title            *string `json:"title"`
}

// Clone godoc
// @Summary      Clone post
// @Description  Duplicates a post as a new draft in the same campaign and phase, copying
// @Description  title, content, media/attachments, used assets, CTA, and audience notes.
// @Description  Attachments are deep-copied in object storage so the clone is independent
// @Description  of its source. Pass target_platform_id/target_post_type to retarget the
// @Description  clone (verbatim content — the assistant path adapts content for you).
// @Tags         posts
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string        true   "Source post Sqid"
// @Param        body  body      cloneRequest  false  "Optional clone overrides"
// @Success      201   {object}  models.Post
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      503   {object}  map[string]string
// @Router       /api/posts/{id}/clone [post]
func (h *PostsHandler) Clone(c *fiber.Ctx) error {
	if h.cloneSvc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "clone is not available")
	}

	var req cloneRequest
	// The body is optional; only parse when one was sent so an empty
	// POST (the common "just duplicate it" case) doesn't 400.
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	}

	session := c.Locals("session").(*models.Session)
	opts := clone.DefaultOptions(session.UserID, clone.TriggerAPI)
	opts.TargetPlatformID = req.TargetPlatformID
	opts.TargetPostType = req.TargetPostType
	opts.TitleOverride = req.Title

	res, err := h.cloneSvc.Clone(c.Context(), c.Params("id"), opts)
	if err != nil {
		switch {
		case errors.Is(err, clone.ErrSourceNotFound):
			return fiber.NewError(fiber.StatusNotFound, "post not found")
		case errors.Is(err, clone.ErrInvalidPlatform):
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res.Post)
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
// @Description  The `content` field is a Markdown string; the frontend renders it via BlockNote.
// @Description  `platform_id` and `platform_post_type` are required only when the new `status` is
// @Description  not `draft`; transitioning a draft to `ready_for_publish` (or beyond) without a
// @Description  platform chosen returns 400.
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
		from := post.Status
		h.logEvent(c, post.ID, models.PostLogEventStateTransitionBlocked, &from, &status,
			"transition rejected by state machine",
			logs.MarshalCapped(map[string]any{"reason": "invalid_transition"}),
		)
		return fiber.NewError(fiber.StatusBadRequest, "invalid status transition from "+string(post.Status)+" to "+string(status))
	}
	if err := requirePlatformIfNotDraft(status, req.PlatformID, req.PlatformPostType); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	if done, err := h.validateReadyForPublish(c, post, &req, status); err != nil {
		return err
	} else if done {
		return nil
	}

	prevStatus := post.Status

	// CON-69 §5 / CON-78: ReadyForPublish→Scheduled consults the
	// auto-publish allowlist and persists status, PostLog, and the submit
	// Backlite task transactionally — now via the shared schedule
	// service so the REST/assistant/PUT paths can't drift. Falls through
	// to the default save when the schedule service isn't wired (test
	// fixtures).
	if prevStatus == models.PostStatusReadyForPublish && status == models.PostStatusScheduled && h.scheduleSvc != nil {
		req.apply(post, status, ctaType)
		actor := models.ActorSystem
		if sess, ok := c.Locals("session").(*models.Session); ok && sess != nil {
			actor = sess.UserID
		}
		routed, err := h.scheduleSvc.RouteAndPersist(c.Context(), post, prevStatus, actor)
		if err != nil {
			return err
		}
		if routed != "" {
			c.Set("X-Auto-Publish-Decision", routed)
		}
	} else {
		req.apply(post, status, ctaType)
		if err := h.repo.Update(c.Context(), post); err != nil {
			return err
		}
		h.logTransition(c, post, prevStatus, status)
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
	id := c.Params("id")
	// Run the before-delete hook (S3 cleanup) before the row is
	// dropped — the hook needs the s3_keys, which the FK cascade will
	// erase on row delete. A hook error fails the whole DELETE so the
	// caller can retry; we don't end up with an orphaned bucket.
	if h.onBeforeDelete != nil {
		if err := h.onBeforeDelete(c.Context(), id); err != nil {
			return err
		}
	}
	deleted, err := h.repo.Delete(c.Context(), id)
	if err != nil {
		return err
	}
	if !deleted {
		return fiber.NewError(fiber.StatusNotFound, "post not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ── Assistant ────────────────────────────────────────────────────────────────

type assistantRequest struct {
	Instruction string `json:"instruction" validate:"required"`
}

// Assistant godoc
// @Summary      Post assistant (SSE)
// @Description  Sends an instruction to the AI assistant and streams progress via Server-Sent Events.
// @Description  Events: "explanation_delta" and "content_delta" carry {"delta":"..."} fragments (Markdown)
// @Description  as the model generates the explanation and updated content. "tool_call" and "tool_result"
// @Description  signal asset-retrieval tool invocations. "complete" carries the final PostAssistantResponse
// @Description  whose updatedContent is a Markdown string.
// @Description  "error" carries {"message":"...","code":<http_code>}.
// @Tags         posts
// @Accept       json
// @Produce      text/event-stream
// @Security     CookieAuth
// @Param        id    path      string           true  "Post Sqid"
// @Param        body  body      assistantRequest true  "Instruction payload"
// @Success      200  "SSE stream: delta / tool_call / tool_result / complete / error events"
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      503   {object}  map[string]string
// @Router       /api/posts/{id}/assistant [post]
func (h *PostsHandler) Assistant(c *fiber.Ctx) error {
	if h.assistant == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "post assistant is not available")
	}
	if h.isAssistantReady != nil && !h.isAssistantReady() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "post assistant is not available")
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

	postID := c.Params("id")
	instruction := req.Instruction
	assistant := h.assistant

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		writeEvent := func(event string, data any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
			_ = w.Flush()
		}

		onEvent := post_assistant.OnEventFunc(func(name post_assistant.SSEEventKind, data any) {
			writeEvent(string(name), data)
		})

		_, err := assistant(context.Background(), post_assistant.PostAssistantRequest{
			PostID:      postID,
			Instruction: instruction,
		}, onEvent)
		if err != nil {
			code := fiber.StatusInternalServerError
			msg := err.Error()
			var ve *post_assistant.ValidationError
			var ae *post_assistant.AIError
			switch {
			case errors.As(err, &ve):
				code = fiber.StatusBadRequest
				msg = ve.Msg
			case errors.As(err, &ae):
				code = fiber.StatusBadGateway
				msg = ae.Msg
			}
			writeEvent(string(post_assistant.SSEEventError), post_assistant.ErrorEventPayload{Message: msg, Code: code})
			return
		}
		// "complete" is emitted by the runner itself; nothing to write here.
	}))

	return nil
}

// Assess godoc
// @Summary      Assess post quality
// @Description  Runs the Post quality assessment agent (CON-85) and streams
// @Description  Server-Sent Events: a "step" event per flow stage, then a
// @Description  final "complete" event carrying the persisted evaluation
// @Description  (four 0-10 dimension scores with rationale, weakness, and
// @Description  span-anchored suggestions, plus the backend-computed overall).
// @Tags         posts
// @Produce      text/event-stream
// @Security     CookieAuth
// @Param        id   path      string  true  "Post Sqid"
// @Success      200  {object}  post_quality.PostQualityResponse
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/posts/{id}/assess [post]
func (h *PostsHandler) Assess(c *fiber.Ctx) error {
	if h.assessQuality == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "post quality assessment is not available")
	}
	if h.isAssistantReady != nil && !h.isAssistantReady() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "post quality assessment is not available")
	}

	// Confirm the post exists before opening the SSE stream so a bad id
	// gets a clean 404 rather than an in-stream error event. Posts are
	// shared across the workspace — any authenticated user may assess any
	// post, consistent with GET/PUT/DELETE/assistant/clone.
	post, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "post not found")
		}
		return err
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	postID := post.ID
	assess := h.assessQuality

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		writeEvent := func(event string, data any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
			_ = w.Flush()
		}

		onEvent := post_quality.OnEventFunc(func(name post_quality.SSEEventKind, data any) {
			writeEvent(string(name), data)
		})

		_, err := assess(context.Background(), postID, onEvent)
		if err != nil {
			code := fiber.StatusInternalServerError
			msg := err.Error()
			var ve *post_quality.ValidationError
			var ae *post_quality.AIError
			switch {
			case errors.As(err, &ve):
				code = fiber.StatusBadRequest
				msg = ve.Msg
			case errors.As(err, &ae):
				code = fiber.StatusBadGateway
				msg = ae.Msg
			}
			writeEvent(string(post_quality.SSEEventError), post_quality.ErrorEventPayload{Message: msg, Code: code})
			return
		}
		// "complete" is emitted by the runner itself; nothing to write here.
	}))

	return nil
}

// GetAssessment godoc
// @Summary      Get stored post quality assessment
// @Description  Returns the most recent persisted quality evaluation for a
// @Description  post (CON-85/CON-92) without invoking the model. The frontend
// @Description  reads this to render an existing assessment and only triggers
// @Description  POST /assess when the post has changed since it was scored.
// @Tags         posts
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "Post Sqid"
// @Success      200  {object}  models.PostEvaluation
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/posts/{id}/assessment [get]
func (h *PostsHandler) GetAssessment(c *fiber.Ctx) error {
	if h.evaluationRepo == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "post quality assessment is not available")
	}
	eval, err := h.evaluationRepo.GetByPostID(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	// GetByPostID returns (nil, nil) when the post has never been assessed.
	// Surface that as 404 so the frontend shows the "Assess" affordance
	// rather than a stale or empty result.
	if eval == nil {
		return fiber.NewError(fiber.StatusNotFound, "post has not been assessed")
	}
	return c.JSON(eval)
}

// GetAnalytics godoc
// @Summary      Get a post's analytics snapshot
// @Description  Returns the latest engagement snapshot for a post published through a publisher. Served entirely from the database — never live-calls the publisher (CON-93 FR4).
// @Tags         posts
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "Post Sqid"
// @Success      200  {object}  handlers.postAnalyticsResponse
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      409  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/posts/{id}/analytics [get]
func (h *PostsHandler) GetAnalytics(c *fiber.Ctx) error {
	if h.analyticsRepo == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "post analytics is not available")
	}
	post, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "post not found")
		}
		return err
	}
	// Analytics is defined only for posts published through a publisher;
	// today that's Zernio (CON-93 §9). 409 with a machine-readable code so
	// the client can distinguish "wrong kind of post" from "not found".
	if post.PublisherPostID == "" {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"code":  "not_published_via_publisher",
			"error": "post has not been published through a publisher",
		})
	}
	snapshot, err := h.analyticsRepo.GetByPostID(c.Context(), post.ID)
	if err != nil {
		return err
	}
	// No snapshot yet → the background refresh hasn't covered this post.
	// Return 200 pending (not 404) so clients can poll (CON-93 §10).
	if snapshot == nil {
		return c.JSON(fiber.Map{"status": "pending", "post_id": post.ID})
	}
	return c.JSON(newPostAnalyticsResponse(post, snapshot))
}

// ── Messages ─────────────────────────────────────────────────────────────────

// ListMessages godoc
// @Summary      List assistant messages
// @Description  Returns the most recent assistant conversation messages for a post.
// @Tags         posts
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "Post Sqid"
// @Success      200  {array}   models.PostAssistantMessage
// @Failure      401  {object}  map[string]string
// @Router       /api/posts/{id}/messages [get]
func (h *PostsHandler) ListMessages(c *fiber.Ctx) error {
	msgs, err := h.messageRepo.ListRecentByPostID(c.Context(), c.Params("id"), 50)
	if err != nil {
		return err
	}
	return c.JSON(msgs)
}

// ── Versions ─────────────────────────────────────────────────────────────────

// ListVersions godoc
// @Summary      List post versions
// @Description  Returns all version snapshots for a post.
// @Tags         posts
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "Post Sqid"
// @Success      200  {array}   models.PostVersion
// @Failure      401  {object}  map[string]string
// @Router       /api/posts/{id}/versions [get]
func (h *PostsHandler) ListVersions(c *fiber.Ctx) error {
	versions, err := h.versionRepo.ListByPostID(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	return c.JSON(versions)
}

type createVersionRequest struct {
	Note string `json:"note"`
}

// CreateVersion godoc
// @Summary      Create post version
// @Description  Manually creates a version snapshot of the current post content.
// @Tags         posts
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string               true  "Post Sqid"
// @Param        body  body      createVersionRequest true  "Version payload"
// @Success      201   {object}  models.PostVersion
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Router       /api/posts/{id}/versions [post]
func (h *PostsHandler) CreateVersion(c *fiber.Ctx) error {
	var req createVersionRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	post, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "post not found")
		}
		return err
	}

	latest, err := h.versionRepo.GetLatestByPostID(c.Context(), post.ID)
	if err != nil {
		return err
	}
	nextNum := 1
	if latest != nil {
		nextNum = latest.VersionNumber + 1
	}

	id, err := models.NewID()
	if err != nil {
		return err
	}

	version := &models.PostVersion{
		ID:            id,
		PostID:        post.ID,
		VersionNumber: nextNum,
		Content:       post.Content,
		Note:          req.Note,
		Creator:       "user",
	}
	if err := h.versionRepo.Create(c.Context(), version); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(version)
}

type restoreRequest struct {
	VersionNumber int `json:"version_number"`
}

// Restore godoc
// @Summary      Restore post to a version
// @Description  Rolls a post's content back to an earlier version (CON-68).
// @Description  Restore is non-destructive: the target version's content is
// @Description  copied into a brand-new version that becomes the new HEAD, so
// @Description  the full history is preserved and the restore is itself
// @Description  reversible. If the live post has unsnapshotted edits, they are
// @Description  auto-saved as a version first so nothing is lost. Restoring to
// @Description  the version that already matches the current content is a no-op.
// @Tags         posts
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string          true  "Post Sqid"
// @Param        body  body      restoreRequest  true  "Target version"
// @Success      200   {object}  map[string]interface{}
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      503   {object}  map[string]string
// @Router       /api/posts/{id}/restore [post]
func (h *PostsHandler) Restore(c *fiber.Ctx) error {
	if h.restoreSvc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "restore is not available")
	}
	var req restoreRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if req.VersionNumber <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "version_number is required and must be positive")
	}

	session := c.Locals("session").(*models.Session)
	res, err := h.restoreSvc.Restore(c.Context(), c.Params("id"), restore.Options{
		Actor:         session.UserID,
		Trigger:       restore.TriggerAPI,
		VersionNumber: req.VersionNumber,
	})
	if err != nil {
		switch {
		case errors.Is(err, restore.ErrPostNotFound):
			return fiber.NewError(fiber.StatusNotFound, "post not found")
		case errors.Is(err, restore.ErrVersionNotFound):
			return fiber.NewError(fiber.StatusNotFound, "version not found")
		case errors.Is(err, restore.ErrNotEditable):
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return err
	}

	// Re-fetch so the response carries a fully hydrated post (campaign /
	// platform / assets), matching the Update handler's contract.
	updated, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"post":                  updated,
		"restored_from_version": res.RestoredFromVersion,
		"new_version_number":    res.NewVersionNumber,
		"auto_snapshot_created": res.AutoSnapshotCreated,
		"no_op":                 res.NoOp,
	})
}
