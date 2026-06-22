package queues

import (
	"context"
	"log"
	"time"

	"github.com/riverqueue/river"

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
// River still requires a per-kind args type so we satisfy JobArgs with
// a marker struct.
type CleanupPostLogsTask struct{}

// Kind implements river.JobArgs.
func (CleanupPostLogsTask) Kind() string { return CleanupPostLogsQueue }

// InsertOpts sets per-kind defaults. The recurring cadence is owned by a
// River PeriodicJob (registered in server.go), so a single attempt is
// enough — a failed sweep is simply retried on the next periodic tick.
func (CleanupPostLogsTask) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1}
}

// CleanupPostLogsProcessor wires the queue handler to the repository
// + retention window. The handler deletes anything older than
// `now - retention`. Cadence is owned by a River PeriodicJob.
type CleanupPostLogsProcessor struct {
	Repo      repository.PostLogRepository
	Retention time.Duration
}

// Process deletes Post Log entries older than the retention window.
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
	return nil
}
