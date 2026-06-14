package queues

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mikestefanello/backlite"

	"github.com/ogen-app/ogen/src/jobs"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/post_actions/postlog"
	"github.com/ogen-app/ogen/src/publishers/zernio"
)

// CancelZernioJobQueue is the Backlite queue name (CON-69 §3, §9).
const CancelZernioJobQueue = "cancel_zernio_job"

// CancelTarget is what the user wants the Post moved to once
// cancellation succeeds — either back to the editing state
// (ReadyForPublish) or all the way to Draft.
type CancelTarget string

const (
	CancelTargetReadyForPublish CancelTarget = "ready_for_publish"
	CancelTargetDraft           CancelTarget = "draft"
)

// CancelZernioJobTask carries the post id and the user's chosen
// post-cancel landing state.
type CancelZernioJobTask struct {
	PostID string       `json:"post_id"`
	Target CancelTarget `json:"target"`
	// Actor is the user id that requested cancellation; recorded on
	// the PostLog entries this task emits so the audit trail traces
	// back to the human, not the queue runner.
	Actor string `json:"actor"`
}

func (CancelZernioJobTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{
		Name:        CancelZernioJobQueue,
		MaxAttempts: 3,
		Backoff:     30 * time.Second,
		Timeout:     20 * time.Second,
		Retention: &backlite.Retention{
			Duration: 7 * 24 * time.Hour,
		},
	}
}

// CancelZernioJobProcessor implements the queue handler.
type CancelZernioJobProcessor struct {
	Deps ZernioDeps
}

// Process executes one cancellation attempt.
//
// Outcome matrix:
//   - Cancel succeeds → Post transitions to the chosen target.
//   - Cancel returns ErrAlreadyPublished → no transition; the next
//     poll cycle will land Published per the normal success path.
//   - Cancel returns transient error → return error so Backlite
//     retries.
//   - Cancel returns terminal error → log and leave Post in
//     Scheduled; user sees a stuck post and can try again. Per
//     CON-69 §9 we surface the failure rather than guessing.
func (p *CancelZernioJobProcessor) Process(ctx context.Context, task CancelZernioJobTask) error {
	post, err := p.Deps.PostRepo.GetByID(ctx, task.PostID)
	if err != nil {
		return fmt.Errorf("cancel: load post %s: %w", task.PostID, err)
	}
	if post.Status != models.PostStatusScheduled {
		// User raced themselves, or the poller landed Published first.
		appendLogActor(ctx, p.Deps, post.ID, task.Actor, models.PostLogEventTaskSucceeded, post.Status, post.Status,
			"cancel aborted: post is no longer Scheduled", `{"reason":"status_changed"}`)
		return nil
	}
	if post.ZernioPostID == "" {
		// Submit never landed; just transition locally per the user's
		// choice. Don't call Zernio.
		return p.transition(ctx, post, task)
	}

	apiStart := time.Now()
	cancelErr := p.Deps.Client.Cancel(ctx, post.ZernioPostID)
	jobs.ObserveZernioCall(time.Since(apiStart))
	if cancelErr == nil {
		jobs.ZernioCancelSucceeded.Add(1)
		appendLogActor(ctx, p.Deps, post.ID, task.Actor, models.PostLogEventZernioCancel, post.Status, post.Status,
			"Zernio cancel succeeded", `{}`)
		return p.transition(ctx, post, task)
	}
	if errors.Is(cancelErr, zernio.ErrAlreadyPublished) {
		// Race: Zernio published before we could cancel. Don't move
		// the Post locally — let the next poll land Published.
		jobs.ZernioCancelAlreadyPublished.Add(1)
		appendLogActor(ctx, p.Deps, post.ID, task.Actor, models.PostLogEventZernioCancel, post.Status, post.Status,
			"cancel race: Zernio reports already published", `{"resolution":"poll_will_land_published"}`)
		return nil
	}
	if zernio.IsTerminalAPIError(cancelErr) {
		jobs.ZernioCancelFailed.Add(1)
		appendLogActor(ctx, p.Deps, post.ID, task.Actor, models.PostLogEventZernioCancel, post.Status, post.Status,
			"Zernio cancel terminally failed", `{"error":"`+cancelErr.Error()+`"}`)
		return nil
	}
	// Transient — retry per Backlite config.
	appendLogActor(ctx, p.Deps, post.ID, task.Actor, models.PostLogEventTaskRetried, post.Status, post.Status,
		"transient cancel error; backlite will retry", `{"error":"`+cancelErr.Error()+`"}`)
	return cancelErr
}

func (p *CancelZernioJobProcessor) transition(ctx context.Context, post *models.Post, task CancelZernioJobTask) error {
	from := post.Status
	to := models.PostStatusReadyForPublish
	if task.Target == CancelTargetDraft {
		to = models.PostStatusDraft
	}
	if !from.CanTransition(to) {
		appendLogActor(ctx, p.Deps, post.ID, task.Actor, models.PostLogEventStateTransitionBlocked, from, to,
			"cancel target rejected by state machine", `{"reason":"invalid_transition"}`)
		return nil
	}
	post.Status = to
	if err := p.Deps.PostRepo.Update(ctx, post); err != nil {
		return fmt.Errorf("cancel: persist new status: %w", err)
	}
	appendLogActor(ctx, p.Deps, post.ID, task.Actor, models.PostLogEventStateTransition, from, to,
		"post cancelled and transitioned", postlog.MarshalCapped(map[string]any{
			"target": string(to),
		}))
	return nil
}

// appendLogActor is the same as appendLog but lets a queue attribute
// the entry to a specific user (the cancellation requester) rather
// than the system actor.
func appendLogActor(
	ctx context.Context,
	deps ZernioDeps,
	postID string,
	actor string,
	evt models.PostLogEventType,
	from, to models.PostStatus,
	summary, payload string,
) {
	if deps.PostLogRepo == nil {
		return
	}
	id, _ := models.NewID()
	if id == "" {
		return
	}
	if actor == "" {
		actor = models.ActorSystem
	}
	fromCopy := from
	toCopy := to
	_ = deps.PostLogRepo.Append(ctx, &models.PostLog{
		ID:         id,
		PostID:     postID,
		EventType:  evt,
		Actor:      actor,
		FromStatus: &fromCopy,
		ToStatus:   &toCopy,
		Summary:    summary,
		Payload:    postlog.SanitizeAndCap(payload),
	})
}
