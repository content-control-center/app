package handlers

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/activity"
	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/genkit/flows/post_assistant"
	"github.com/ogen-app/ogen/src/genkit/flows/post_quality"
	"github.com/ogen-app/ogen/src/jobs"
	"github.com/ogen-app/ogen/src/jobs/queues"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/platforms"
	"github.com/ogen-app/ogen/src/post_actions/clone"
	"github.com/ogen-app/ogen/src/post_actions/logs"
	"github.com/ogen-app/ogen/src/post_actions/restore"
	"github.com/ogen-app/ogen/src/post_actions/schedule"
	"github.com/ogen-app/ogen/src/publishers/zernio"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/tenantctx"
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
	// brandRepo validates a post's brand_voice_id/brand_audience_id belong to
	// the tenant (CON-245). Optional (SetBrandRepo); nil skips validation.
	brandRepo repository.BrandRepository
	auth      fiber.Handler
	assistant func(ctx context.Context, req post_assistant.PostAssistantRequest, onEvent post_assistant.OnEventFunc) (*post_assistant.PostAssistantResponse, error)
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
	// posts go straight to Scheduled with no River enqueue.
	allowlistRepo repository.AutoPublishAllowlistRepository
	// jobsClient enqueues the Zernio cancellation task (CON-69 §9). nil
	// disables the cancel endpoint (503).
	jobsClient CancelEnqueuer
	// db is the Bun DB handle. Held so the schedule path can run the
	// status update + PostLog write + River enqueue in a single
	// transaction (CON-69 §5).
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
	// activity records CON-125 user-activity events (post_created,
	// post_scheduled, …) to the analytics store. nil is a no-op (analytics
	// disabled / fixtures). Wired via SetActivityRecorder.
	activity *activity.Recorder
	// verify-external deps (CON-153): confirm a manually-published post via
	// Zernio's sync-external, then back-fill publisher_post_id + a first
	// analytics snapshot. A nil zernioClient disables POST /:id/verify-external
	// (503). Wired via SetVerifyExternalDeps.
	zernioClient      *zernio.Client
	socialAccountRepo repository.SocialAccountRepository
	profileID         func(ctx context.Context) (string, error)
	// analyticsHub emits the post.analytics.updated event after a successful
	// verify-external so open analytics streams refresh (CON-93 §8). nil = no-op.
	analyticsHub eventhub.Hub
}

// SetVerifyExternalDeps wires the CON-153 external-post verification path. Kept
// a setter so existing NewPostsHandler call sites and fixtures stay unchanged;
// a nil client leaves POST /:id/verify-external at 503.
func (h *PostsHandler) SetVerifyExternalDeps(client *zernio.Client, accounts repository.SocialAccountRepository, profileID func(ctx context.Context) (string, error), hub eventhub.Hub) {
	h.zernioClient = client
	h.socialAccountRepo = accounts
	h.profileID = profileID
	h.analyticsHub = hub
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

// CancelEnqueuer enqueues a Zernio cancellation task. Implemented by
// *queues.Enqueuer; kept as a narrow interface so the handler depends on a
// tiny method set rather than the queue runtime directly.
type CancelEnqueuer interface {
	EnqueueCancel(ctx context.Context, postID string, target queues.CancelTarget, actor string) error
}

// SetSchedulingDeps wires everything the schedule path needs: the
// allowlist repo (to choose Scheduled vs ScheduledForManualPublish),
// the job enqueuer (to enqueue cancellation tasks), and the bun DB
// handle. Any nil disables the corresponding branch — useful for
// fixtures that don't exercise auto-publish.
func (h *PostsHandler) SetSchedulingDeps(allowlist repository.AutoPublishAllowlistRepository, client CancelEnqueuer, db *bun.DB) {
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

// SetBrandRepo wires the CON-245 brand repository so a post's brand refs can be
// tenant-validated. Optional; nil skips validation.
func (h *PostsHandler) SetBrandRepo(r repository.BrandRepository) {
	h.brandRepo = r
}

// SetActivityRecorder wires the CON-125 activity recorder. nil (analytics
// disabled) makes every activity emission a no-op.
func (h *PostsHandler) SetActivityRecorder(r *activity.Recorder) {
	h.activity = r
}

// recordActivity emits a best-effort CON-125 "post" activity event. Tenant +
// user are resolved from the request context (set by the auth middleware);
// source defaults to api and can be overridden by a later option.
func (h *PostsHandler) recordActivity(c *fiber.Ctx, typ string, opts ...activity.Option) {
	h.activity.Record(c.Context(), activity.CategoryPost, typ,
		append([]activity.Option{activity.WithSource(activity.SourceAPI)}, opts...)...)
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
		h.recordActivity(c, "post_validation_failed",
			activity.WithEntity("post", post.ID),
			activity.WithStatus("failed"),
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
	h.recordActivity(c, "post_state_transition",
		activity.WithEntity("post", post.ID),
		activity.WithStatus(string(prev)+"->"+string(next)),
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

// writeAccountSelectionError renders a CON-150 account-selection failure as a
// 422 carrying the stable machine reason, the platform, and — for the
// ambiguous case — the connected accounts the client can offer as a picker.
func writeAccountSelectionError(c *fiber.Ctx, e *schedule.AccountSelectionError) error {
	body := fiber.Map{"error": e.Reason, "platform": e.Platform}
	if e.Candidates != nil {
		body["candidates"] = e.Candidates
	}
	return c.Status(fiber.StatusUnprocessableEntity).JSON(body)
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
		var aerr *schedule.AccountSelectionError
		switch {
		case errors.Is(err, schedule.ErrPostNotFound):
			return fiber.NewError(fiber.StatusNotFound, "post not found")
		case errors.As(err, &aerr):
			return writeAccountSelectionError(c, aerr)
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
	h.recordActivity(c, "post_scheduled",
		activity.WithEntity("post", updated.ID),
		activity.WithStatus(string(res.Status)),
		activity.WithPayload(map[string]any{
			"scheduled_at": req.ScheduledAt,
			"auto_publish": res.AutoPublish,
			"promoted":     res.Promoted,
		}),
	)
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
// @Description  Enqueues a River cancel_zernio_job task. The Post
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

	if err := h.jobsClient.EnqueueCancel(c.Context(), post.ID, target, actor); err != nil {
		return fmt.Errorf("cancel: enqueue: %w", err)
	}

	h.logEvent(c, post.ID, models.PostLogEventUserCancel, &post.Status, &post.Status,
		"user requested cancellation; cancel_zernio_job enqueued",
		logs.MarshalCapped(map[string]any{"target": string(target)}),
	)
	h.recordActivity(c, "post_schedule_cancelled",
		activity.WithEntity("post", post.ID),
		activity.WithPayload(map[string]any{"target": string(target)}),
	)

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"status":  "cancellation_enqueued",
		"target":  string(target),
		"post_id": post.ID,
	})
}

// convertToManualRequest is the body for POST /api/posts/convert-to-manual.
// Exactly one of Platform / PostIDs must be set. Platform is a Zernio
// platform id (e.g. "linkedin"), matching the auto-publish allowlist's
// vocabulary; it selects every scheduled post for that platform.
type convertToManualRequest struct {
	Platform string   `json:"platform"`
	PostIDs  []string `json:"post_ids"`
}

// convertFailure is one rejected post in the convert-to-manual response.
type convertFailure struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// ConvertToManual godoc
// @Summary      Convert scheduled posts to manual publishing
// @Description  Converts a set of posts — or every scheduled post for a
// @Description  platform — from auto-publish (`scheduled`) to
// @Description  `scheduled_for_manual_publishing`, keeping `scheduled_at`.
// @Description  Each post's live Zernio job is cancelled server-side by a
// @Description  cancel_zernio_job task, which then lands the post directly
// @Description  on `scheduled_for_manual_publishing` — no detour through
// @Description  `ready_for_publish`, so the post is never left unscheduled
// @Description  if the client disconnects mid-flight. Returns per-post
// @Description  outcomes: `converted` (accepted / already manual) and
// @Description  `failed` (not found, or not in a convertible state). A
// @Description  post that publishes before its cancel lands is handled by
// @Description  the worker and recorded in the Post Log (CON-130).
// @Tags         posts
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body      convertToManualRequest  true  "Platform or explicit post ids"
// @Success      202   {object}  map[string]interface{}
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      503   {object}  map[string]string
// @Router       /api/posts/convert-to-manual [post]
func (h *PostsHandler) ConvertToManual(c *fiber.Ctx) error {
	if h.jobsClient == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "background job runtime not configured")
	}
	var req convertToManualRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	byPlatform := req.Platform != ""
	byIDs := len(req.PostIDs) > 0
	if byPlatform == byIDs {
		return fiber.NewError(fiber.StatusBadRequest, `provide exactly one of "platform" or "post_ids"`)
	}

	actor := models.ActorSystem
	if sess, ok := c.Locals("session").(*models.Session); ok && sess != nil {
		actor = sess.UserID
	}

	converted := make([]string, 0)
	failed := make([]convertFailure, 0)

	// convert enqueues one durable cancel→manual task per still-scheduled
	// post. Once enqueued the worker owns the cancel, the wait, and the
	// final status, so the request can return without holding the post in
	// an unscheduled limbo (CON-130).
	convert := func(post *models.Post) {
		switch post.Status {
		case models.PostStatusScheduledForManualPublish:
			// Already where the caller wants it — idempotent success.
			converted = append(converted, post.ID)
		case models.PostStatusScheduled:
			if err := h.jobsClient.EnqueueCancel(c.Context(), post.ID, queues.CancelTargetManualPublish, actor); err != nil {
				failed = append(failed, convertFailure{ID: post.ID, Reason: "could not enqueue conversion: " + err.Error()})
				return
			}
			converted = append(converted, post.ID)
		default:
			failed = append(failed, convertFailure{ID: post.ID,
				Reason: "post is not scheduled (status: " + string(post.Status) + ")"})
		}
	}

	if byPlatform {
		sqid := zernio.LookupSqidByZernioID(req.Platform)
		if sqid == "" {
			return fiber.NewError(fiber.StatusBadRequest,
				fmt.Sprintf("platform %q is not in the Ogen-supported set", req.Platform))
		}
		posts, err := h.repo.ListScheduledByPlatform(c.Context(), sqid)
		if err != nil {
			return err
		}
		for i := range posts {
			convert(&posts[i])
		}
	} else {
		seen := make(map[string]bool, len(req.PostIDs))
		for _, id := range req.PostIDs {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			post, err := h.repo.GetByID(c.Context(), id)
			if err != nil {
				reason := "could not load post: " + err.Error()
				if errors.Is(err, sql.ErrNoRows) {
					reason = "post not found"
				}
				// Report this id and keep going — one bad id must not sink the
				// whole batch, matching the endpoint's per-post contract.
				failed = append(failed, convertFailure{ID: id, Reason: reason})
				continue
			}
			convert(post)
		}
	}

	h.recordActivity(c, "posts_converted_to_manual",
		activity.WithPayload(map[string]any{
			"platform":  req.Platform,
			"converted": len(converted),
			"failed":    len(failed),
		}),
	)

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"converted": converted,
		"failed":    failed,
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

// postBrandRequest is the body of PUT /api/posts/:id/brand — a targeted set of
// just the post's voice + audience (CON-245), so the editor's picker need not
// round-trip the whole resource.
type postBrandRequest struct {
	BrandVoiceID    *string `json:"brand_voice_id"`
	BrandAudienceID *string `json:"brand_audience_id"`
}

// SetBrand sets a post's own brand voice + audience. Both are tenant-checked and
// nullable (null clears the ref). It changes only the two refs — no status
// machine, no publish gate — so it is a plain scoped update.
func (h *PostsHandler) SetBrand(c *fiber.Ctx) error {
	var req postBrandRequest
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
	if err := validateBrandRefs(c.Context(), h.brandRepo, req.BrandVoiceID, req.BrandAudienceID); err != nil {
		return err
	}
	post.BrandVoiceID = req.BrandVoiceID
	post.BrandAudienceID = req.BrandAudienceID
	post.UpdatedAt = time.Now().UTC()
	if err := h.repo.Update(c.Context(), post); err != nil {
		return err
	}
	return c.JSON(post)
}

func (h *PostsHandler) Register(app *fiber.App) {
	g := app.Group("/api/posts")
	g.Get("/", h.auth, h.List)
	g.Post("/", h.auth, h.Create)
	// Static route registered before "/:id/..." so it isn't shadowed by
	// the id-parametrised routes (CON-130).
	g.Post("/convert-to-manual", h.auth, h.ConvertToManual)
	g.Get("/:id", h.auth, h.Get)
	g.Put("/:id", h.auth, h.Update)
	// CON-245: targeted set of a post's own brand voice + audience.
	g.Put("/:id/brand", h.auth, h.SetBrand)
	g.Delete("/:id", h.auth, h.Delete)
	g.Post("/:id/assistant", h.auth, h.Assistant)
	g.Post("/:id/assess", h.auth, h.Assess)
	g.Get("/:id/assessment", h.auth, h.GetAssessment)
	g.Get("/:id/analytics", h.auth, h.GetAnalytics)
	g.Post("/:id/verify-external", h.auth, h.VerifyExternal)
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
	PlatformID       string `json:"platform_id"`
	PlatformPostType string `json:"platform_post_type"`
	// SocialAccountID (CON-150) names which same-platform account the post
	// publishes to. Optional: omit it and the submit worker auto-selects the
	// platform's single account, or requires a choice when there are several.
	SocialAccountID     string             `json:"social_account_id"`
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
	BrandVoiceID        *string            `json:"brand_voice_id"`
	BrandAudienceID     *string            `json:"brand_audience_id"`
	// PublishedURL (CON-165) lets the front-end record a permalink for posts
	// Zernio cannot verify (the CON-149 skip path — e.g. LinkedIn personal
	// accounts) or correct a wrong one. Like every field on this whole-resource
	// PUT, the FE round-trips the current value; an empty string clears it.
	PublishedURL string `json:"published_url"`
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
	post.SocialAccountID = r.SocialAccountID
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
	// CON-165: published_url stays writable after publish on purpose — recording
	// a permalink is a post-publish affordance. When CON-251's content-lock
	// lands, keep this field on the allowed-after-submit list.
	post.PublishedURL = r.PublishedURL
	post.BrandVoiceID = r.BrandVoiceID
	post.BrandAudienceID = r.BrandAudienceID
	post.UsedAssetIDs = nullSlice(r.UsedAssetIDs)
	post.UpdatedAt = time.Now().UTC()
}

// mutatesLockedContent reports whether the request would change any of the
// content-identity fields CON-251 freezes once a post is submitted: the
// body, title, media, platform, post type, or the sources it was built
// from. The date and account are locked by the schedule/cancel flows that
// own them, and a status-only transition (e.g. unschedule to edit) leaves
// every field below equal, so neither is compared here — this gates the
// silent-divergence edit, not the legitimate move off a submitted state.
func (r *postRequest) mutatesLockedContent(post *models.Post) bool {
	return r.Content != post.Content ||
		r.Title != post.Title ||
		r.PlatformID != post.PlatformID ||
		r.PlatformPostType != post.PlatformPostType ||
		!slices.Equal(nullSlice(r.MediaURLs), post.MediaURLs) ||
		!slices.Equal(nullSlice(r.UsedAssetIDs), post.UsedAssetIDs)
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
	h.recordActivity(c, "post_validation_failed",
		activity.WithEntity("post", post.ID),
		activity.WithStatus("failed"),
	)
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
	h.recordActivity(c, "post_created",
		activity.WithEntity("post", post.ID),
		activity.WithPayload(map[string]any{"status": string(post.Status), "campaign_id": post.CampaignID}),
	)
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
	h.recordActivity(c, "post_cloned",
		activity.WithEntity("post", res.Post.ID),
		activity.WithPayload(map[string]any{"source_post_id": c.Params("id")}),
	)
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

	if err := validateBrandRefs(c.Context(), h.brandRepo, req.BrandVoiceID, req.BrandAudienceID); err != nil {
		return err
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
	// CON-251: once a post is submitted (scheduled or published) a copy of it
	// exists outside Ogen — Zernio holds the scheduled submission (content
	// snapshotted at schedule time), the network holds the published post.
	// Editing the body/title/media/platform/post-type/sources here would
	// silently rewrite Ogen's record of what goes, or went, out, so those
	// fields are frozen — the server backstop to the FE lock, mirroring the
	// attachment freeze. A status-only transition (unschedule to edit) still
	// passes, as does a no-op save; only a real content change is rejected.
	if post.Status.IsSubmitted() && req.mutatesLockedContent(post) {
		return fiber.NewError(fiber.StatusConflict,
			"post has been submitted ("+string(post.Status)+") and its content is locked; unschedule to edit")
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
	// River task transactionally — now via the shared schedule
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
			var aerr *schedule.AccountSelectionError
			if errors.As(err, &aerr) {
				return writeAccountSelectionError(c, aerr)
			}
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
	h.recordActivity(c, "post_updated",
		activity.WithEntity("post", post.ID),
		activity.WithStatus(string(updated.Status)),
	)
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
	h.recordActivity(c, "post_deleted", activity.WithEntity("post", id))
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

	h.activity.Record(c.Context(), activity.CategoryAIFlow, "post_assistant_turn",
		activity.WithEntity("post", c.Params("id")),
		activity.WithSource(activity.SourceAssistant),
	)

	// c.Params / BodyParser values alias fasthttp's request buffer, which is
	// recycled for a later request the moment this handler returns — but the
	// StreamWriter below runs *after* that return and only persists at the end
	// of a multi-minute model call. Without copying, a concurrent request can
	// overwrite the buffer mid-turn, corrupting the post id (observed in the
	// wild as post_id="ations/zerni") so the final insert trips the post_id FK
	// and is mis-surfaced as "this post was deleted while the assistant was
	// working". Clone the retained strings to pin their own backing arrays.
	postID := strings.Clone(c.Params("id"))
	instruction := strings.Clone(req.Instruction)
	assistant := h.assistant
	// Carry the tenant into the detached flow context (the StreamWriter runs
	// after this handler returns) so usage recording + enforcement attribute
	// to the right tenant (CON-86).
	tenantID, _ := c.Locals(tenantctx.Key).(string)
	flowCtx := tenantctx.With(context.Background(), tenantID)

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		writeEvent := func(event string, data any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
			_ = w.Flush()
		}

		onEvent := post_assistant.OnEventFunc(func(name post_assistant.SSEEventKind, data any) {
			writeEvent(string(name), data)
		})

		_, err := assistant(flowCtx, post_assistant.PostAssistantRequest{
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
	tenantID, _ := c.Locals(tenantctx.Key).(string)
	flowCtx := tenantctx.With(context.Background(), tenantID)
	// Capture the actor now; the stream writer runs after the request context
	// may be recycled, so the activity record can't read it from c.Context().
	var actorID string
	if sess, ok := c.Locals("session").(*models.Session); ok && sess != nil {
		actorID = sess.UserID
	}

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		writeEvent := func(event string, data any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
			_ = w.Flush()
		}

		onEvent := post_quality.OnEventFunc(func(name post_quality.SSEEventKind, data any) {
			writeEvent(string(name), data)
		})

		_, err := assess(flowCtx, postID, onEvent)
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
		h.activity.Record(flowCtx, activity.CategoryPost, "post_quality_assessed",
			activity.WithUser(actorID),
			activity.WithEntity("post", postID),
			activity.WithSource(activity.SourceAPI),
			activity.WithStatus("success"),
		)
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
// @Description  Returns the latest engagement snapshot for a post published through a publisher. Served entirely from the database — never live-calls the publisher (CON-93 FR4). Two 200 shapes: a full snapshot, or — when the background refresh has not yet covered the post — the pending form `{"status":"pending","post_id":"..."}` so clients can poll. Both are covered by the schema below (the snapshot fields are absent on the pending response; `status` is absent on a snapshot).
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

// verifyExternalRequest is the body for POST /:id/verify-external. At least
// one locator (url or post_id) is required.
type verifyExternalRequest struct {
	URL    string `json:"url"`
	PostID string `json:"post_id"`
}

// VerifyExternal godoc
// @Summary      Verify a manually-published post
// @Description  Confirms an externally/manually published post exists via Zernio's on-demand sync (CON-153), resolving the post's connected account (CON-150 rules). On a match it back-fills publisher_post_id, marks the post published, writes a first analytics snapshot, and emits post.analytics.updated. Returns {found:false} when the post can't be located.
// @Tags         posts
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path  string                 true  "Post Sqid"
// @Param        body  body  handlers.verifyExternalRequest  true  "Post URL or platform post id"
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      409  {object}  map[string]string
// @Failure      422  {object}  map[string]string
// @Failure      502  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/posts/{id}/verify-external [post]
func (h *PostsHandler) VerifyExternal(c *fiber.Ctx) error {
	if h.zernioClient == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "external verification is not available")
	}
	var body verifyExternalRequest
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if body.URL == "" && body.PostID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "url or post_id is required")
	}

	post, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "post not found")
		}
		return err
	}

	profileID := ""
	if h.profileID != nil {
		if profileID, err = h.profileID(c.Context()); err != nil {
			return err
		}
	}
	if profileID == "" {
		return fiber.NewError(fiber.StatusServiceUnavailable, "zernio integration is not configured")
	}

	// Resolve the connected account whose token reads the platform (CON-150).
	supported := zernio.LookupSupportedBySqid(post.PlatformID)
	if supported == nil {
		return fiber.NewError(fiber.StatusConflict, "post has no supported platform")
	}
	zernioPlatform := supported.ZernioID

	accountID, handled, err := h.resolveExternalAccountID(c, post, profileID, zernioPlatform)
	if handled {
		return err
	}

	result, err := h.zernioClient.SyncExternalPost(c.Context(), zernio.SyncExternalRequest{
		AccountID: accountID,
		URL:       body.URL,
		PostID:    body.PostID,
	})
	if err != nil {
		// A 404 means the platform has no such post — a normal "not found",
		// not a server error.
		if zernio.IsStatus(err, fiber.StatusNotFound) {
			jobs.ZernioExternalVerifyNotFound.Add(1)
			return c.JSON(fiber.Map{"found": false})
		}
		jobs.ZernioExternalVerifyFailed.Add(1)
		return fiber.NewError(fiber.StatusBadGateway, "sync-external upstream error")
	}
	if !result.Found || result.Post == nil {
		jobs.ZernioExternalVerifyNotFound.Add(1)
		return c.JSON(fiber.Map{"found": false})
	}
	ext := result.Post

	// Back-fill the publisher linkage so per-post analytics resolve, and mark
	// the post published-external.
	if post.PublisherPostID == "" {
		post.PublisherPostID = ext.PlatformPostID
	}
	if post.Publisher == "" {
		post.Publisher = models.PublisherZernio
	}
	if post.PublishedAt == nil {
		if pa := ext.PublishedAtTime(); pa != nil {
			post.PublishedAt = pa
		}
	}
	// CON-165: persist the canonical, platform-normalised permalink. Verification
	// is authoritative, so a confirmed URL overwrites any user-pasted one.
	if ext.PlatformPostURL != "" {
		post.PublishedURL = ext.PlatformPostURL
	}
	post.Status = models.PostStatusPublished
	post.UpdatedAt = time.Now().UTC()
	if err := h.repo.Update(c.Context(), post); err != nil {
		jobs.ZernioExternalVerifyFailed.Add(1)
		return err
	}
	// CON-251: the post is now confirmed published — snapshot the content as a
	// durable record of what went out (best-effort, deduped against the head).
	h.snapshotPublished(c.Context(), post)

	// Refresh the current-state analytics row (best-effort) and emit the update
	// event so open analytics streams refresh. The response carries the fetched
	// metrics regardless of whether persistence is available. Mirrors the refresh
	// job's dedup discipline (CON-236): preserve first_seen_at, always bump
	// last_checked_at, and append a trend point / publish only on a real change —
	// so re-verifying an already-tracked post doesn't clobber its history.
	metrics := externalMetrics(ext.Analytics)
	if h.analyticsRepo != nil {
		now := time.Now().UTC()
		built := h.buildExternalCurrent(post, ext)
		prev, _ := h.analyticsRepo.GetByPostID(c.Context(), post.ID)
		changed := prev == nil || prev.MetricsKey() != built.MetricsKey()
		built.LastCheckedAt = now
		if prev == nil {
			built.FirstSeenAt = now
			built.LastChangedAt = now
		} else {
			built.FirstSeenAt = prev.FirstSeenAt
			if changed {
				built.LastChangedAt = now
			} else {
				built.LastChangedAt = prev.LastChangedAt
			}
		}
		if changed {
			if id, ierr := models.NewID(); ierr == nil {
				if werr := h.analyticsRepo.UpsertWithSnapshot(c.Context(), built, built.NewSnapshot(id, now)); werr == nil {
					h.publishAnalyticsUpdated(c.Context(), built)
				}
			}
		} else {
			_ = h.analyticsRepo.Upsert(c.Context(), built)
		}
	}

	jobs.ZernioExternalVerifySucceeded.Add(1)
	return c.JSON(fiber.Map{
		"found": true,
		"post": fiber.Map{
			"id": post.ID,
			// The confirmed external post's own id — the one ext.Analytics
			// belong to (post.PublisherPostID is left as-is when already set).
			"publisher_post_id": ext.PlatformPostID,
			// CON-165: echo the persisted permalink so the FE can render
			// "View post" straight after verify, without a re-fetch.
			"published_url": post.PublishedURL,
			"sync_status":   "synced",
		},
		"analytics": metrics,
	})
}

// resolveExternalAccountID resolves the Zernio account id to read the platform
// with, applying the CON-150 rules. It returns (accountID, handled, err): when
// handled is true the caller must return err immediately — either a response
// has already been written (in which case err is the fiber write result, which
// is nil on success) or err is a real failure. When handled is false, accountID
// is non-empty and safe to use. The explicit handled flag is required because
// c.Status(...).JSON(...) / writeAccountSelectionError return nil on a
// successful write, so a nil error alone cannot signal "already responded".
func (h *PostsHandler) resolveExternalAccountID(c *fiber.Ctx, post *models.Post, profileID, zernioPlatform string) (string, bool, error) {
	if h.socialAccountRepo == nil {
		return "", true, fiber.NewError(fiber.StatusServiceUnavailable, "social accounts are not available")
	}
	// Explicit account on the post wins; verify it's connected and on-platform.
	if post.SocialAccountID != "" {
		acc, err := h.socialAccountRepo.GetActive(c.Context(), profileID, post.SocialAccountID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "", true, writeAccountSelectionError(c, &schedule.AccountSelectionError{Reason: "account_unavailable", Platform: zernioPlatform})
			}
			return "", true, err
		}
		if acc.Platform != zernioPlatform {
			return "", true, writeAccountSelectionError(c, &schedule.AccountSelectionError{Reason: "account_platform_mismatch", Platform: zernioPlatform})
		}
		return acc.ID, false, nil
	}
	// No explicit choice: auto-select the single account, require a choice at
	// 2+, terminally fail at 0 (CON-150).
	accounts, err := h.socialAccountRepo.ListActiveByPlatform(c.Context(), profileID, zernioPlatform)
	if err != nil {
		return "", true, err
	}
	switch len(accounts) {
	case 0:
		return "", true, c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "no_account_connected", "platform": zernioPlatform})
	case 1:
		return accounts[0].ID, false, nil
	default:
		candidates := make([]schedule.AccountCandidate, 0, len(accounts))
		for _, a := range accounts {
			candidates = append(candidates, schedule.AccountCandidate{ID: a.ID, Username: a.Username, DisplayName: a.DisplayName})
		}
		return "", true, writeAccountSelectionError(c, &schedule.AccountSelectionError{Reason: "account_selection_required", Platform: zernioPlatform, Candidates: candidates})
	}
}

// externalMetrics maps a synced external post's analytics onto the shared
// metrics block.
func externalMetrics(a zernio.ExternalPostAnalytics) models.PostAnalyticsMetrics {
	return models.PostAnalyticsMetrics{
		Impressions:    a.Impressions,
		Reach:          a.Reach,
		Likes:          a.Likes,
		Comments:       a.Comments,
		Shares:         a.Shares,
		Saves:          a.Saves,
		Clicks:         a.Clicks,
		Views:          a.Views,
		EngagementRate: a.EngagementRate,
	}
}

// buildExternalCurrent builds the current-state analytics row from a synced
// external post, denormalising the post's display fields exactly like the
// refresh job (CON-236). The first_seen/last_changed/last_checked timestamps are
// stamped by the caller (they depend on the dedup comparison against any
// existing current row).
func (h *PostsHandler) buildExternalCurrent(post *models.Post, ext *zernio.ExternalPost) *models.PostAnalytics {
	m := externalMetrics(ext.Analytics)
	platformName := ext.Platform
	if post.Platform != nil && post.Platform.Name != "" {
		platformName = post.Platform.Name
	}
	return &models.PostAnalytics{
		PostID: post.ID,
		// The synced external post's id — kept consistent with the metrics
		// below (ext.Analytics), not the post's possibly-preexisting linkage.
		PublisherPostID: ext.PlatformPostID,
		Publisher:       models.PublisherZernio,
		Platform:        platformName,
		Title:           post.Title,
		PublishedAt:     post.PublishedAt,
		Impressions:     m.Impressions,
		Reach:           m.Reach,
		Likes:           m.Likes,
		Comments:        m.Comments,
		Shares:          m.Shares,
		Saves:           m.Saves,
		Clicks:          m.Clicks,
		Views:           m.Views,
		EngagementRate:  m.EngagementRate,
		PlatformAnalytics: models.PlatformAnalyticsList{{
			Platform:        ext.Platform,
			PlatformPostID:  ext.PlatformPostID,
			PlatformPostURL: ext.PlatformPostURL,
			SyncStatus:      "synced",
			Analytics:       m,
		}},
		SyncStatus:         "synced",
		MetricsLastUpdated: ext.Analytics.LastUpdatedTime(),
	}
}

// publishAnalyticsUpdated emits the post.analytics.updated event (CON-93 §8)
// after a verify-external snapshot. No-op when no Hub is wired.
func (h *PostsHandler) publishAnalyticsUpdated(ctx context.Context, a *models.PostAnalytics) {
	if h.analyticsHub == nil {
		return
	}
	tid, _ := tenantctx.From(ctx)
	_ = h.analyticsHub.Publish(ctx, eventhub.Event{
		Topic:    fmt.Sprintf("entity:post:%s", a.PostID),
		TenantID: tid,
		Type:     "post.analytics.updated",
		Payload: map[string]any{
			"post_id":     a.PostID,
			"sync_status": a.SyncStatus,
			"analytics":   a.Metrics(),
		},
	})
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
	// Preserve the array contract: an empty history serializes as [] not null.
	if msgs == nil {
		msgs = []models.PostAssistantMessage{}
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
// snapshotPublished records a system-authored version of a post's content at
// the moment it is confirmed published (CON-251), so "what actually went out"
// becomes a durable record rather than an assumption. Deduped only against a
// prior "Published" system snapshot of identical content, so re-verifying an
// already-published post adds nothing — but a matching-content user/assistant
// edit (or the schedule-time "Submitted" snapshot) at the head does NOT
// suppress it, so the publish is always recorded once. Best-effort: a nil repo
// or any error is swallowed — the publish has already committed, mirroring the
// surrounding analytics writes.
func (h *PostsHandler) snapshotPublished(ctx context.Context, post *models.Post) {
	if h.versionRepo == nil {
		return
	}
	latest, err := h.versionRepo.GetLatestByPostID(ctx, post.ID)
	if err != nil {
		return
	}
	if latest != nil && latest.IsSystemSnapshot() &&
		latest.Note == models.PostVersionNotePublished && latest.Content == post.Content {
		return
	}
	nextNum := 1
	if latest != nil {
		nextNum = latest.VersionNumber + 1
	}
	id, err := models.NewID()
	if err != nil {
		return
	}
	_ = h.versionRepo.Create(ctx, &models.PostVersion{
		ID:            id,
		PostID:        post.ID,
		VersionNumber: nextNum,
		Content:       post.Content,
		Note:          models.PostVersionNotePublished,
		Creator:       models.PostVersionCreatorSystem,
	})
}

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
	h.recordActivity(c, "post_restored",
		activity.WithEntity("post", c.Params("id")),
		activity.WithPayload(map[string]any{
			"restored_from_version": res.RestoredFromVersion,
			"new_version_number":    res.NewVersionNumber,
			"no_op":                 res.NoOp,
		}),
	)
	return c.JSON(fiber.Map{
		"post":                  updated,
		"restored_from_version": res.RestoredFromVersion,
		"new_version_number":    res.NewVersionNumber,
		"auto_snapshot_created": res.AutoSnapshotCreated,
		"no_op":                 res.NoOp,
	})
}
