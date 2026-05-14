package queues

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mikestefanello/backlite"
	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/jobs"
	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/postlog"
)

// ReconcileScheduledPostsQueue is the recurring sweeper queue
// (CON-69 §8). Runs every Every duration; finds Posts in Scheduled
// whose scheduled_at + Grace has passed without a terminal Zernio
// status, and forces them to Failed with a reason that distinguishes
// reconciliation_timeout from a Zernio-reported failure.
const ReconcileScheduledPostsQueue = "reconcile_scheduled_posts"

// ReconcileScheduledPostsTask is a marker payload — like the cleanup
// task, this queue carries no per-tick data.
type ReconcileScheduledPostsTask struct{}

func (ReconcileScheduledPostsTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{
		Name:    ReconcileScheduledPostsQueue,
		Timeout: 30 * time.Second,
		Retention: &backlite.Retention{
			Duration: 24 * time.Hour,
		},
	}
}

// FailureReasonReconciliationTimeout is the verbatim prefix every
// reconciliation-timeout failure carries on Post.FailureReason. The
// distinct prefix is the contract CON-69 §8 calls out — "explicitly
// distinguishes reconciliation-timeout from a Zernio-reported
// failure".
const FailureReasonReconciliationTimeout = "reconciliation_timeout"

// ReconcileScheduledPostsProcessor wires the sweep handler.
//
// The sweep runs in a tight loop: load N stuck posts in one query,
// transition each in its own transaction, log per-post.
type ReconcileScheduledPostsProcessor struct {
	DB     *bun.DB
	Repo   ReconcilePostRepo
	LogRepo ReconcileLogRepo
	Grace  time.Duration // how long after scheduled_at before timing out
	Every  time.Duration // self-reschedule cadence
	Limit  int           // max posts to process per tick (defaults to 100)
}

// ReconcilePostRepo is the narrow surface the sweeper needs out of
// the post repository. Defining it here keeps the queue test-friendly
// without dragging in the full PostRepository interface.
type ReconcilePostRepo interface {
	ListStuckScheduled(ctx context.Context, cutoff time.Time, limit int) ([]models.Post, error)
	UpdateStatusAndReason(ctx context.Context, postID string, status models.PostStatus, reason string) error
}

// ReconcileLogRepo is the narrow PostLog surface.
type ReconcileLogRepo interface {
	Append(ctx context.Context, entry *models.PostLog) error
}

// Process runs one sweep tick.
func (p *ReconcileScheduledPostsProcessor) Process(ctx context.Context, _ ReconcileScheduledPostsTask) error {
	if p.Grace <= 0 {
		p.Grace = time.Hour
	}
	if p.Limit <= 0 {
		p.Limit = 100
	}

	cutoff := time.Now().UTC().Add(-p.Grace)
	stuck, err := p.Repo.ListStuckScheduled(ctx, cutoff, p.Limit)
	if err != nil {
		return fmt.Errorf("reconcile: list stuck: %w", err)
	}

	for i := range stuck {
		post := &stuck[i]
		reason := fmt.Sprintf("%s: scheduled_at=%s elapsed=%s last_zernio_status=%q",
			FailureReasonReconciliationTimeout,
			fmtTime(post.ScheduledAt),
			fmtElapsedSince(post.ScheduledAt),
			post.ZernioStatus,
		)
		if err := p.Repo.UpdateStatusAndReason(ctx, post.ID, models.PostStatusFailed, reason); err != nil {
			log.Printf("reconcile: failed to mark post %s Failed: %v", post.ID, err)
			continue
		}
		from := models.PostStatusScheduled
		to := models.PostStatusFailed
		id, _ := models.NewID()
		_ = p.LogRepo.Append(ctx, &models.PostLog{
			ID:         id,
			PostID:     post.ID,
			EventType:  models.PostLogEventReconciliationTimeout,
			Actor:      models.ActorSystem,
			FromStatus: &from,
			ToStatus:   &to,
			Summary:    "reconciliation timeout: forcing Failed",
			Payload: postlog.SanitizeAndCap(postlog.MarshalCapped(map[string]any{
				"reason":              FailureReasonReconciliationTimeout,
				"scheduled_at":        post.ScheduledAt,
				"elapsed":             fmtElapsedSince(post.ScheduledAt),
				"last_zernio_status":  post.ZernioStatus,
				"zernio_post_id":      post.ZernioPostID,
			})),
		})
	}
	if len(stuck) > 0 {
		jobs.ReconciliationTimeouts.Add(int64(len(stuck)))
		log.Printf("reconcile: forced %d post(s) Failed due to reconciliation timeout", len(stuck))
	}

	// Self-reschedule for the next tick.
	if p.Every > 0 {
		if client := backlite.FromContext(ctx); client != nil {
			if _, err := client.Add(ReconcileScheduledPostsTask{}).Wait(p.Every).Save(); err != nil {
				return err
			}
		}
	}
	return nil
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return "<nil>"
	}
	return t.UTC().Format(time.RFC3339)
}

func fmtElapsedSince(t *time.Time) string {
	if t == nil {
		return "<nil>"
	}
	return time.Since(*t).Truncate(time.Second).String()
}
