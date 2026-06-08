package queues

import (
	"context"
	"log"
	"time"

	"github.com/mikestefanello/backlite"

	"github.com/ogen-app/ogen/src/jobs"
	"github.com/ogen-app/ogen/src/repository"
)

// CleanupPostLogsQueue is the recurring task that drops Post Log
// entries older than the configured retention window (CON-69 §11).
// It re-enqueues itself from inside Process so the cadence is owned
// by the task itself — losing one tick to a crash doesn't permanently
// disable the sweeper, since the next boot's seed re-enqueue picks
// it up again.
const CleanupPostLogsQueue = "cleanup_post_logs"

// CleanupPostLogsTask is the typed payload for the queue. There's
// nothing to carry — the worker just looks at the cutoff window — but
// Backlite still requires a per-task type so we satisfy the interface
// with a marker struct.
type CleanupPostLogsTask struct{}

func (CleanupPostLogsTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{
		Name:    CleanupPostLogsQueue,
		Timeout: 30 * time.Second,
		Retention: &backlite.Retention{
			// Keep a day of completed sweeps in the Backlite UI for
			// quick "did the sweeper run?" checks.
			Duration: 24 * time.Hour,
		},
	}
}

// CleanupPostLogsProcessor wires the queue handler to the repository
// + retention window and the cadence. The handler deletes anything
// older than `now - retention`, then re-schedules itself `every`
// duration in the future.
type CleanupPostLogsProcessor struct {
	Repo      repository.PostLogRepository
	Retention time.Duration
	Every     time.Duration
}

// Process satisfies backlite.QueueProcessor. Self-rescheduling cadence
// keeps the sweeper alive without an external scheduler.
func (p *CleanupPostLogsProcessor) Process(ctx context.Context, _ CleanupPostLogsTask) error {
	if p.Retention > 0 && p.Repo != nil {
		cutoff := time.Now().UTC().Add(-p.Retention)
		n, err := p.Repo.DeleteOlderThan(ctx, cutoff)
		if err != nil {
			return err
		}
		if n > 0 {
			jobs.PostLogCleaned.Add(n)
			log.Printf("post_logs: cleanup removed %d entries older than %s", n, cutoff.Format(time.RFC3339))
		}
	}
	if p.Every > 0 {
		client := backlite.FromContext(ctx)
		if client != nil {
			if _, err := client.Add(CleanupPostLogsTask{}).Wait(p.Every).Save(); err != nil {
				return err
			}
		}
	}
	return nil
}
