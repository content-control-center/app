package queues

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mikestefanello/backlite"

	"github.com/ogen-app/ogen/src/jobs"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/post_actions/postlog"
	"github.com/ogen-app/ogen/src/publishers/zernio"
	"github.com/ogen-app/ogen/src/settings"
)

// SubmitPostToZernioQueue is the Backlite queue name (CON-69 §3).
const SubmitPostToZernioQueue = "submit_post_to_zernio"

// SubmitPostTask carries the Ogen post id; the worker re-loads the
// row so we never persist stale field copies in the queue payload.
type SubmitPostTask struct {
	PostID string `json:"post_id"`
}

func (SubmitPostTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{
		Name:        SubmitPostToZernioQueue,
		MaxAttempts: 5,                // 4 retries (CON-69 §6 per-attempt logging)
		Backoff:     30 * time.Second, // exponential is preferable; backlite is linear
		Timeout:     30 * time.Second,
		Retention: &backlite.Retention{
			Duration:   30 * 24 * time.Hour,
			OnlyFailed: true,
			Data:       &backlite.RetainData{OnlyFailed: true},
		},
	}
}

// SubmitPostProcessor implements the queue handler. Held by the
// runtime; one instance per process.
type SubmitPostProcessor struct {
	Deps ZernioDeps
	// PollLeadTime is how far in advance of scheduled_at we begin
	// polling. Defaults to 30s when zero.
	PollLeadTime time.Duration
}

// Process runs one submit attempt. Returns a non-nil error to ask
// Backlite to retry; returns nil for both success and terminal
// failure (Post moved to Failed inside this method when the failure
// is terminal).
func (p *SubmitPostProcessor) Process(ctx context.Context, task SubmitPostTask) error {
	post, err := p.Deps.PostRepo.GetByID(ctx, task.PostID)
	if err != nil {
		return fmt.Errorf("submit: load post %s: %w", task.PostID, err)
	}
	if post.Status != models.PostStatusScheduled {
		// User cancelled or reconciliation moved this post; abort
		// quietly. The poll task does the same check.
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventTaskSucceeded, post.Status, post.Status,
			"submit aborted: post no longer Scheduled", `{"reason":"status_changed"}`)
		return nil
	}
	// Idempotency / manual retry path (CON-69 §10):
	//   - On a fresh submit, ZernioPostID is empty and we POST /posts.
	//   - On a manual retry of a previously-failed Post (the user
	//     moved Failed→ReadyForPublish, then ReadyForPublish→Scheduled),
	//     ZernioPostID is still set from the prior attempt. Calling
	//     Zernio's /posts/:id/retry endpoint reuses the same job
	//     identity so we don't create a duplicate Zernio post and
	//     can keep polling against the same id.
	if post.ZernioPostID != "" {
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventZernioRetry, post.Status, post.Status,
			"calling Zernio POST /posts/:id/retry", `{"zernio_post_id":"`+post.ZernioPostID+`"}`)
		job, retryErr := p.Deps.Client.Retry(ctx, post.ZernioPostID)
		if retryErr != nil {
			if zernio.IsTerminalAPIError(retryErr) {
				return p.terminal(ctx, post, "zernio_retry_rejected", retryErr.Error())
			}
			appendLog(ctx, p.Deps, post.ID, models.PostLogEventTaskRetried, post.Status, post.Status,
				"transient Zernio retry error; backlite will retry", `{"error":"`+retryErr.Error()+`"}`)
			return retryErr
		}
		return p.persistSuccess(ctx, post, job)
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
		"calling Zernio POST /posts", postlog.MarshalCapped(req))

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
					"recovered Zernio job after 409 dedupe", postlog.MarshalCapped(recovered))
				return p.persistSuccess(ctx, post, recovered)
			}
			return p.terminal(ctx, post, "zernio_dedupe_unrecoverable",
				"Zernio reported duplicate content but no matching job was found")
		}
		if zernio.IsTerminalAPIError(submitErr) {
			jobs.ZernioSubmitFailed.Add(1)
			return p.terminal(ctx, post, "zernio_rejected", submitErr.Error())
		}
		// Transient — let Backlite retry per QueueConfig.
		jobs.ZernioSubmitRetried.Add(1)
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventTaskRetried, post.Status, post.Status,
			"transient Zernio error; backlite will retry", `{"error":"`+submitErr.Error()+`"}`)
		return submitErr
	}

	jobs.ZernioSubmitSucceeded.Add(1)
	return p.persistSuccess(ctx, post, job)
}

func (p *SubmitPostProcessor) persistSuccess(ctx context.Context, post *models.Post, job *zernio.Job) error {
	post.ZernioPostID = job.ID
	post.ZernioStatus = string(job.Status)
	if err := p.Deps.PostRepo.Update(ctx, post); err != nil {
		return fmt.Errorf("submit: persist zernio_post_id: %w", err)
	}
	appendLog(ctx, p.Deps, post.ID, models.PostLogEventZernioSubmit, post.Status, post.Status,
		"Zernio submit succeeded; polling scheduled", postlog.MarshalCapped(map[string]any{
			"zernio_post_id": job.ID,
			"zernio_status":  job.Status,
		}))
	p.enqueuePoll(ctx, post)
	return nil
}

func (p *SubmitPostProcessor) enqueuePoll(ctx context.Context, post *models.Post) {
	client := backlite.FromContext(ctx)
	if client == nil {
		return
	}
	when := time.Now().UTC().Add(p.pollLead())
	if post.ScheduledAt != nil && post.ScheduledAt.After(time.Now()) {
		when = post.ScheduledAt.Add(p.pollLead())
	}
	if _, err := client.Add(PollZernioStatusTask{PostID: post.ID}).At(when).Save(); err != nil {
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
// returned error is always nil — Backlite must NOT retry once we've
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
		"submit terminally failed", postlog.MarshalCapped(map[string]any{
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
		Payload:    postlog.SanitizeAndCap(payload),
	})
}

// MarshalSubmit is exported so tests can produce a payload byte slice
// matching what backlite would deserialize.
func MarshalSubmit(t SubmitPostTask) ([]byte, error) { return json.Marshal(t) }
