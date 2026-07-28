package queues

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"time"

	"github.com/riverqueue/river"

	"github.com/ogen-app/ogen/src/activity"
	"github.com/ogen-app/ogen/src/jobs"
	"github.com/ogen-app/ogen/src/logging"
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

	// CON-122: upload the post's attachments to Zernio and reference them as
	// mediaItems. A failure here is transient (network / Zernio media endpoint)
	// so River retries; media isn't lost because the post stays Scheduled.
	mediaItems, mediaErr := p.buildMediaItems(ctx, post)
	if mediaErr != nil {
		jobs.ZernioSubmitRetried.Add(1)
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventTaskRetried, post.Status, post.Status,
			"transient error uploading media to Zernio; River will retry", `{"error":"`+mediaErr.Error()+`"}`)
		return mediaErr
	}

	req := zernio.SubmitRequest{
		Content:      post.Content,
		Platforms:    []zernio.PlatformVariant{{Platform: supported.ZernioID, AccountID: accountID}},
		ScheduledFor: when,
		Timezone:     tzName,
		MediaItems:   mediaItems,
	}

	appendLog(ctx, p.Deps, post.ID, models.PostLogEventZernioSubmit, post.Status, post.Status,
		"calling Zernio POST /posts", logs.MarshalCapped(req))

	apiStart := time.Now()
	job, submitErr := p.Deps.Client.Submit(ctx, req)
	jobs.ObserveZernioCall(time.Since(apiStart))
	if submitErr != nil {
		// 24h dedupe recovery (CON-129). Search the whole dedupe window across all
		// statuses, matching on the content we actually submitted (so it still
		// holds once the API flattens Markdown, CON-126).
		if errors.Is(submitErr, zernio.ErrDuplicateContent) {
			recovered, ferr := p.Deps.Client.FindByContent(ctx, req.Content, 24*time.Hour)
			switch {
			case ferr != nil:
				// Couldn't query Zernio to recover — transient; retry, which 409s
				// again and re-attempts recovery rather than dead-ending the post.
				jobs.ZernioSubmitRetried.Add(1)
				appendLog(ctx, p.Deps, post.ID, models.PostLogEventTaskRetried, post.Status, post.Status,
					"transient error locating dedupe match; River will retry", `{"error":"`+ferr.Error()+`"}`)
				return ferr
			case recovered != nil && !recovered.Status.IsTerminal():
				// Still-pending earlier job (almost always this post's own prior
				// attempt): adopt it and keep polling. Count it like the main
				// success path (persistSuccess doesn't, so this is exactly once).
				jobs.ZernioSubmitSucceeded.Add(1)
				appendLog(ctx, p.Deps, post.ID, models.PostLogEventZernioSubmit, post.Status, post.Status,
					"recovered pending Zernio job after 409 dedupe", logs.MarshalCapped(recovered))
				return p.persistSuccess(ctx, post, recovered, accountID)
			case recovered != nil:
				// Match is terminal (published/failed/partial): the content already
				// ran on Zernio in this window. Don't adopt a finished job — fail
				// with the cause named so the user can act.
				jobs.ZernioSubmitFailed.Add(1)
				return p.terminal(ctx, post, "zernio_duplicate_content",
					fmt.Sprintf("identical content was already %s on Zernio within the last 24h (job %s); edit the content to submit again",
						recovered.Status, recovered.ID))
			default:
				// Duplicate per Zernio, but no matching job located (content
				// normalisation mismatch, or beyond the search window). Name it.
				jobs.ZernioSubmitFailed.Add(1)
				return p.terminal(ctx, post, "zernio_duplicate_content",
					"Zernio rejected this as duplicate content submitted within the last 24h; edit the content or wait for the dedupe window to pass")
			}
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

// buildMediaItems uploads each of the post's attachments to Zernio (presign →
// PUT bytes → publicUrl) and returns the mediaItems array for the submit
// request, in position order, carrying altText (CON-122). Nil deps
// (Storage/PostAttachmentRepo) or no attachments ⇒ nil (a text-only post).
// Attachments whose MIME type Zernio doesn't accept are skipped, not fatal.
func (p *SubmitPostProcessor) buildMediaItems(ctx context.Context, post *models.Post) ([]map[string]any, error) {
	if p.Deps.Storage == nil || p.Deps.PostAttachmentRepo == nil {
		return nil, nil
	}
	atts, err := p.Deps.PostAttachmentRepo.ListByPostID(ctx, post.ID)
	if err != nil {
		return nil, fmt.Errorf("submit: list attachments: %w", err)
	}
	if len(atts) == 0 {
		return nil, nil
	}
	items := make([]map[string]any, 0, len(atts))
	for i := range atts {
		att := atts[i]
		mediaType := zernio.MediaType(att.MimeType)
		if mediaType == "" {
			slog.WarnContext(ctx, "skipping attachment: unsupported media type for Zernio",
				logging.AttrComponent, "jobs.submit", "post_id", post.ID, "attachment_id", att.ID, "mime", att.MimeType)
			continue
		}
		item, err := p.uploadMedia(ctx, &att, mediaType)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// uploadMedia streams one attachment from our storage to Zernio (presign + PUT)
// and returns its mediaItems descriptor {url, type, altText?}.
func (p *SubmitPostProcessor) uploadMedia(ctx context.Context, att *models.PostAttachment, mediaType string) (map[string]any, error) {
	rc, err := p.Deps.Storage.Download(ctx, att.S3Key)
	if err != nil {
		return nil, fmt.Errorf("submit: download attachment %s: %w", att.ID, err)
	}
	defer rc.Close()

	presign, err := p.Deps.Client.PresignMedia(ctx, path.Base(att.S3Key), att.MimeType)
	if err != nil {
		return nil, fmt.Errorf("submit: presign media %s: %w", att.ID, err)
	}
	if err := p.Deps.Client.UploadMedia(ctx, presign.UploadURL, att.MimeType, rc, att.SizeBytes); err != nil {
		return nil, fmt.Errorf("submit: upload media %s: %w", att.ID, err)
	}

	item := map[string]any{"url": presign.PublicURL, "type": mediaType}
	if att.AltText != "" {
		item["altText"] = att.AltText
	}
	return item, nil
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
