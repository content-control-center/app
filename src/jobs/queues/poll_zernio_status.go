package queues

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mikestefanello/backlite"

	"github.com/content-control-center/app/src/jobs"
	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/postlog"
	"github.com/content-control-center/app/src/publishers/zernio"
)

// PollZernioStatusQueue is the Backlite queue name (CON-69 §3, §7).
const PollZernioStatusQueue = "poll_zernio_status"

// PollZernioStatusTask carries the Ogen post id; the worker re-loads
// the row each cycle.
type PollZernioStatusTask struct {
	PostID string `json:"post_id"`
}

func (PollZernioStatusTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{
		Name:        PollZernioStatusQueue,
		MaxAttempts: 3,                // transient errors get a few retries
		Backoff:     30 * time.Second,
		Timeout:     20 * time.Second,
		Retention: &backlite.Retention{
			Duration:   7 * 24 * time.Hour,
			OnlyFailed: true,
		},
	}
}

// PollZernioStatusProcessor implements the recurring poll. The
// cadence per CON-69 §7: every 30s for the first 5 minutes after
// scheduled_at, then every 60s. Hard floor — never sooner than 5s.
type PollZernioStatusProcessor struct {
	Deps              ZernioDeps
	FastInterval      time.Duration // default 30s
	SlowInterval      time.Duration // default 60s
	FastWindow        time.Duration // default 5m after scheduled_at
}

// Process executes one poll cycle. Returns nil for both the
// "succeeded" and "self-rescheduled" cases; returns a non-nil error
// only on transient Zernio failures so Backlite retries per Backoff.
func (p *PollZernioStatusProcessor) Process(ctx context.Context, task PollZernioStatusTask) error {
	post, err := p.Deps.PostRepo.GetByID(ctx, task.PostID)
	if err != nil {
		return fmt.Errorf("poll: load post %s: %w", task.PostID, err)
	}
	if post.Status != models.PostStatusScheduled {
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventTaskSucceeded, post.Status, post.Status,
			"poll exited: post is no longer Scheduled", `{"reason":"status_changed"}`)
		return nil
	}
	if post.ZernioPostID == "" {
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventTaskFailed, post.Status, post.Status,
			"poll exited: zernio_post_id is empty", `{}`)
		return nil
	}

	apiStart := time.Now()
	job, statusErr := p.Deps.Client.Status(ctx, post.ZernioPostID)
	jobs.ObserveZernioCall(time.Since(apiStart))
	if statusErr != nil {
		// Transient → retry. Terminal API errors during polling are
		// odd (404 if Zernio dropped the row) — log once and stop;
		// the reconciler will eventually time out the post.
		if zernio.IsTerminalAPIError(statusErr) {
			jobs.ZernioPollFailed.Add(1)
			appendLog(ctx, p.Deps, post.ID, models.PostLogEventZernioPoll, post.Status, post.Status,
				"poll terminal API error; awaiting reconcile", `{"error":"`+statusErr.Error()+`"}`)
			return nil
		}
		jobs.ZernioPollRetried.Add(1)
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventZernioPoll, post.Status, post.Status,
			"poll transient error; backlite will retry", `{"error":"`+statusErr.Error()+`"}`)
		return statusErr
	}
	jobs.ZernioPollSucceeded.Add(1)

	// Persist Zernio's view of status so subsequent polls / debugging
	// can see what we last saw. Always write — cheap and useful.
	post.ZernioStatus = string(job.Status)
	if !job.Status.IsTerminal() {
		_ = p.Deps.PostRepo.Update(ctx, post)
		// Self-reschedule per cadence rule.
		p.scheduleNext(ctx, post)
		return nil
	}

	// Terminal state: published / failed / partial. Map to Ogen status
	// and persist per-platform results.
	from := post.Status
	switch job.Status {
	case zernio.JobStatusPublished:
		now := time.Now().UTC()
		post.PublishedAt = &now
		post.Status = models.PostStatusPublished
		results, _ := json.Marshal(job.Platforms)
		post.PublishedResults = string(results)
		if err := p.Deps.PostRepo.Update(ctx, post); err != nil {
			return fmt.Errorf("poll: persist Published: %w", err)
		}
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventStateTransition, from, post.Status,
			"Zernio reported published", postlog.MarshalCapped(map[string]any{
				"published_at": now,
				"platforms":    job.Platforms,
			}))
	case zernio.JobStatusFailed, zernio.JobStatusPartial:
		// `partial` = some platforms succeeded, others failed. Per
		// CON-69 we treat this as Failed for the MVP; per-platform
		// detail goes to the Post Log so the user can see what
		// landed and what didn't.
		post.Status = models.PostStatusFailed
		post.FailureReason = "zernio_terminal: " + string(job.Status)
		results, _ := json.Marshal(job.Platforms)
		post.PublishedResults = string(results)
		if err := p.Deps.PostRepo.Update(ctx, post); err != nil {
			return fmt.Errorf("poll: persist Failed: %w", err)
		}
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventStateTransition, from, post.Status,
			fmt.Sprintf("Zernio reported %s", job.Status), postlog.MarshalCapped(map[string]any{
				"zernio_status": job.Status,
				"platforms":     job.Platforms,
			}))
	default:
		// Defensive: unknown terminal state.
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventZernioPoll, post.Status, post.Status,
			"unknown Zernio terminal status — ignoring", `{"zernio_status":"`+string(job.Status)+`"}`)
	}
	return nil
}

func (p *PollZernioStatusProcessor) scheduleNext(ctx context.Context, post *models.Post) {
	client := backlite.FromContext(ctx)
	if client == nil {
		return
	}
	delay := p.intervalFor(post)
	if _, err := client.Add(PollZernioStatusTask{PostID: post.ID}).Wait(delay).Save(); err != nil {
		appendLog(ctx, p.Deps, post.ID, models.PostLogEventTaskFailed, post.Status, post.Status,
			"failed to self-reschedule poll", `{"error":"`+err.Error()+`"}`)
	}
}

func (p *PollZernioStatusProcessor) intervalFor(post *models.Post) time.Duration {
	fast := p.FastInterval
	if fast <= 0 {
		fast = 30 * time.Second
	}
	slow := p.SlowInterval
	if slow <= 0 {
		slow = 60 * time.Second
	}
	window := p.FastWindow
	if window <= 0 {
		window = 5 * time.Minute
	}
	if post.ScheduledAt == nil {
		return fast
	}
	if time.Since(*post.ScheduledAt) <= window {
		return fast
	}
	return slow
}
