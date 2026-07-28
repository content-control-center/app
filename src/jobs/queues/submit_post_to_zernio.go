package queues

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/ogen-app/ogen/src/activity"
	"github.com/ogen-app/ogen/src/jobs"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/post_actions/logs"
	"github.com/ogen-app/ogen/src/publishers/zernio"
	"github.com/ogen-app/ogen/src/settings"
	"github.com/ogen-app/ogen/src/tenantctx"
	"github.com/ogen-app/ogen/src/vendors"
)

// SubmitPostToZernioQueue is the River queue name (CON-69 §3).
const SubmitPostToZernioQueue = "submit_post_to_zernio"

// SubmitPostTask carries the Ogen post id; the worker re-loads the
// row so we never persist stale field copies in the queue payload.
type SubmitPostTask struct {
	PostID string `json:"post_id"`
}

// Kind implements river.JobArgs.
func (SubmitPostTask) Kind() string { return SubmitPostToZernioQueue }

// InsertOpts sets per-kind defaults: 5 total attempts (4 retries) per
// CON-69 §6. Retry backoff is River's default exponential-with-jitter (the
// PRD notes exponential is preferable to backlite's fixed 30s). The
// per-attempt timeout lives on the worker (submitPostWorker.Timeout).
func (SubmitPostTask) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5}
}

// SubmitPostProcessor is the River worker for submit_post_to_zernio. It
// implements river.Worker directly; Process is the test seam Work delegates to.
type SubmitPostProcessor struct {
	river.WorkerDefaults[SubmitPostTask]
	Deps ZernioDeps
	// PollLeadTime is how far in advance of scheduled_at we begin
	// polling. Defaults to 30s when zero.
	PollLeadTime time.Duration
}

// Work is the River entrypoint; it delegates to Process.
func (p *SubmitPostProcessor) Work(ctx context.Context, job *river.Job[SubmitPostTask]) error {
	ctx = WithJobRequestID(ctx, job.JobRow)
	// CON-97: background jobs span tenants (interim until per-tenant, PR4).
	ctx = tenantctx.WithSystem(ctx)
	return p.Process(ctx, job.Args)
}

// Timeout is the per-attempt context deadline.
func (p *SubmitPostProcessor) Timeout(*river.Job[SubmitPostTask]) time.Duration {
	return 30 * time.Second
}

func init() {
	register(func(w *river.Workers, d Deps) {
		river.AddWorker(w, &SubmitPostProcessor{Deps: d.Zernio})
	})
}

// Process runs one submit attempt. Returns a non-nil error to ask
// River to retry; returns nil for both success and terminal
// failure (Post moved to Failed inside this method when the failure
// is terminal).
func (p *SubmitPostProcessor) Process(ctx context.Context, task SubmitPostTask) error {
	post, err := p.Deps.PostRepo.GetByID(ctx, task.PostID)
	if err != nil {
		return fmt.Errorf("submit: load post %s: %w", task.PostID, err)
	}
	// Scope the rest of the job to the owning tenant (CON-97 PR4).
	ctx = tenantctx.With(ctx, post.TenantID)
	if post.Status != models.PostStatusScheduled {
		// User cancelled or reconciliation moved this post; abort
		// quietly. The poll task does the same check.
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventTaskSucceeded, post.Status, post.Status,
			"submit aborted: post no longer Scheduled", `{"reason":"status_changed"}`)
		return nil
	}
	// Idempotency / manual retry path (CON-69 §10):
	//   - On a fresh submit, PublisherPostID is empty and we POST /posts.
	//   - On a manual retry of a previously-failed Post (the user
	//     moved Failed→ReadyForPublish, then ReadyForPublish→Scheduled),
	//     PublisherPostID is still set from the prior attempt. Calling
	//     Zernio's /posts/:id/retry endpoint reuses the same job
	//     identity so we don't create a duplicate Zernio post and
	//     can keep polling against the same id.
	if post.PublisherPostID != "" {
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventZernioRetry, post.Status, post.Status,
			"calling Zernio POST /posts/:id/retry", `{"publisher_post_id":"`+post.PublisherPostID+`"}`)
		job, retryErr := p.Deps.Client.Retry(ctx, post.PublisherPostID)
		if retryErr != nil {
			if zernio.IsTerminalAPIError(retryErr) {
				return p.terminal(ctx, post, "zernio_retry_rejected", retryErr.Error())
			}
			appendLog(ctx, p.Deps, post.ID, models.PostLogEventTaskRetried, post.Status, post.Status,
				"transient Zernio retry error; River will retry", `{"error":"`+retryErr.Error()+`"}`)
			return retryErr
		}
		return p.persistSuccess(ctx, post, job, "")
	}

	platform := post.Platform
	if platform == nil {
		return p.terminal(ctx, post, "missing_platform", "post has no platform set")
	}

	supported := zernio.LookupSupportedBySqid(platform.ID)
	if supported == nil {
		return p.terminal(ctx, post, "unsupported_platform",
			fmt.Sprintf("platform %s (%s) is not Zernio-supported", platform.Name, platform.ID))
	}

	// One Zernio account per (profile, platform) for the single-tenant
	// MVP. Multi-account selection is a follow-up.
	if p.Deps.ProfileID == nil {
		return p.terminal(ctx, post, "integration_disabled", "Zernio integration is not configured")
	}
	profileID, perr := p.Deps.ProfileID(ctx)
	if perr != nil || profileID == "" {
		return p.terminal(ctx, post, "no_profile", "Zernio profile id not yet bootstrapped")
	}
	accounts, err := p.Deps.SocialAccountRepo.ListActive(ctx, profileID)
	if err != nil {
		return fmt.Errorf("submit: list accounts: %w", err)
	}
	var accountID string
	for _, a := range accounts {
		if a.Platform == supported.ZernioID {
			accountID = a.ID
			break
		}
	}
	if accountID == "" {
		return p.terminal(ctx, post, "no_account_connected",
			fmt.Sprintf("no active Zernio account connected for platform %s", supported.ZernioID))
	}

	when := time.Now().UTC().Add(time.Minute)
	if post.ScheduledAt != nil {
		when = post.ScheduledAt.UTC()
	}
	// The instant (ScheduledFor) is the source of truth and stays UTC;
	// the workspace timezone (CON-78) is echoed so Zernio renders the
	// schedule in the operator's zone. Defaults to UTC when unset.
	_, tzName := settings.WorkspaceTimezone(ctx, p.Deps.SettingRepo)
	req := zernio.SubmitRequest{
		Content:      post.Content,
		Platforms:    []zernio.PlatformVariant{{Platform: supported.ZernioID, AccountID: accountID}},
		ScheduledFor: when,
		Timezone:     tzName,
	}

	appendLog(ctx, p.Deps, post.ID, models.PostLogEventZernioSubmit, post.Status, post.Status,
		"calling Zernio POST /posts", logs.MarshalCapped(req))

	apiStart := time.Now()
	job, submitErr := p.Deps.Client.Submit(ctx, req)
	jobs.ObserveZernioCall(time.Since(apiStart))
	if submitErr != nil {
		// 24h dedupe recovery: try to find the existing job by content
		// and adopt it as if submit had succeeded.
		if errors.Is(submitErr, zernio.ErrDuplicateContent) {
			recovered, ferr := p.Deps.Client.FindByContent(ctx, post.Content, 24*time.Hour)
			if ferr == nil && recovered != nil {
				appendLog(ctx, p.Deps, post.ID, models.PostLogEventZernioSubmit, post.Status, post.Status,
					"recovered Zernio job after 409 dedupe", logs.MarshalCapped(recovered))
				return p.persistSuccess(ctx, post, recovered, accountID)
			}
			return p.terminal(ctx, post, "zernio_dedupe_unrecoverable",
				"Zernio reported duplicate content but no matching job was found")
		}
		if zernio.IsTerminalAPIError(submitErr) {
			jobs.ZernioSubmitFailed.Add(1)
			return p.terminal(ctx, post, "zernio_rejected", submitErr.Error())
		}
		// Transient — let River retry per InsertOpts.
		jobs.ZernioSubmitRetried.Add(1)
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventTaskRetried, post.Status, post.Status,
			"transient Zernio error; River will retry", `{"error":"`+submitErr.Error()+`"}`)
		return submitErr
	}

	jobs.ZernioSubmitSucceeded.Add(1)
	return p.persistSuccess(ctx, post, job, accountID)
}

func (p *SubmitPostProcessor) persistSuccess(ctx context.Context, post *models.Post, job *zernio.Job, accountID string) error {
	post.Publisher = zernio.PublisherID
	post.PublisherPostID = job.ID
	post.PublisherStatus = string(job.Status)
	if err := p.Deps.PostRepo.Update(ctx, post); err != nil {
		return fmt.Errorf("submit: persist publisher_post_id: %w", err)
	}
	appendLog(ctx, p.Deps, post.ID, models.PostLogEventZernioSubmit, post.Status, post.Status,
		"Zernio submit succeeded; polling scheduled", logs.MarshalCapped(map[string]any{
			"publisher":         zernio.PublisherID,
			"publisher_post_id": job.ID,
			"publisher_status":  job.Status,
		}))
	p.recordPublish(ctx, post, accountID)
	p.Deps.ActivityRecorder.Record(ctx, activity.CategoryPublish, "publish_submitted",
		activity.WithEntity("post", post.ID),
		activity.WithSource(activity.SourceJob),
		activity.WithStatus("submitted"),
	)
	p.enqueuePoll(ctx, post)
	return nil
}

// recordPublish emits a CON-86 publish/schedule usage event (one post unit)
// carrying platform + social_account_id. The operation is schedule when the
// post has a future ScheduledAt, else publish. accountID is "" on the manual-
// retry path (not re-resolved there). Nil recorder = no-op; tenant is in ctx.
func (p *SubmitPostProcessor) recordPublish(ctx context.Context, post *models.Post, accountID string) {
	op := zernio.OpPublish
	if post.ScheduledAt != nil {
		op = zernio.OpSchedule
	}
	platform := ""
	if post.Platform != nil {
		if s := zernio.LookupSupportedBySqid(post.Platform.ID); s != nil {
			platform = s.ZernioID
		}
	}
	p.Deps.Recorder.Record(ctx, zernio.VendorZernio, "zernio_submit", vendors.MeterEvent{
		Model:           platform,
		Operation:       op,
		Usage:           vendors.Usage{vendors.KindPost: 1},
		Platform:        platform,
		SocialAccountID: accountID,
	})
}

func (p *SubmitPostProcessor) enqueuePoll(ctx context.Context, post *models.Post) {
	client, err := river.ClientFromContextSafely[*sql.Tx](ctx)
	if err != nil || client == nil {
		return
	}
	when := time.Now().UTC().Add(p.pollLead())
	if post.ScheduledAt != nil && post.ScheduledAt.After(time.Now()) {
		when = post.ScheduledAt.Add(p.pollLead())
	}
	if _, err := client.Insert(ctx, PollZernioStatusTask{PostID: post.ID}, insertOptsWithRequestID(ctx, &river.InsertOpts{ScheduledAt: when})); err != nil {
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventTaskFailed, post.Status, post.Status,
			"failed to enqueue poll_zernio_status", `{"error":"`+err.Error()+`"}`)
	}
}

func (p *SubmitPostProcessor) pollLead() time.Duration {
	if p.PollLeadTime > 0 {
		return p.PollLeadTime
	}
	return 30 * time.Second
}

// terminal marks the Post Failed and writes a terminal PostLog. The
// returned error is always nil — River must NOT retry once we've
// already moved the Post out of Scheduled.
func (p *SubmitPostProcessor) terminal(ctx context.Context, post *models.Post, reason, msg string) error {
	from := post.Status
	post.Status = models.PostStatusFailed
	post.FailureReason = reason + ": " + msg
	if err := p.Deps.PostRepo.Update(ctx, post); err != nil {
		return fmt.Errorf("submit: mark Failed: %w", err)
	}
	to := post.Status
	appendLog(ctx, p.Deps, post.ID, models.PostLogEventStateTransition, from, to,
		"submit terminally failed", logs.MarshalCapped(map[string]any{
			"reason":  reason,
			"message": msg,
		}))
	return nil
}

// appendLog is a tiny helper that keeps the writes uniform across all
// Zernio queues. Errors are swallowed (PostLog is best-effort) — same
// philosophy as the handler-side helper.
func appendLog(
	ctx context.Context,
	deps ZernioDeps,
	postID string,
	evt models.PostLogEventType,
	from,
	to models.PostStatus,
	summary,
	payload string,
) {
	if deps.PostLogRepo == nil {
		return
	}
	id, _ := models.NewID()
	if id == "" {
		return
	}
	fromCopy := from
	toCopy := to
	_ = deps.PostLogRepo.Append(ctx, &models.PostLog{
		ID:         id,
		PostID:     postID,
		EventType:  evt,
		Actor:      models.ActorSystem,
		FromStatus: &fromCopy,
		ToStatus:   &toCopy,
		Summary:    summary,
		Payload:    logs.SanitizeAndCap(payload),
	})
}

// MarshalSubmit is exported so tests can produce a payload byte slice
// matching what backlite would deserialize.
func MarshalSubmit(t SubmitPostTask) ([]byte, error) { return json.Marshal(t) }
