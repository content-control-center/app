package queues

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/riverqueue/river"
)

// This file adapts the typed processors in this package to River
// (riverqueue/river) workers and exposes the enqueue surface the rest of
// the app uses. Each worker is a thin shell over a processor's Process
// method, so the heavily-tested processor logic is unchanged. Per-attempt
// timeouts live on the workers; MaxAttempts and scheduling defaults live on
// the args types' InsertOpts methods.

// --- Workers ---

type submitPostWorker struct {
	river.WorkerDefaults[SubmitPostTask]
	p *SubmitPostProcessor
}

func (w *submitPostWorker) Work(ctx context.Context, job *river.Job[SubmitPostTask]) error {
	return w.p.Process(ctx, job.Args)
}
func (w *submitPostWorker) Timeout(*river.Job[SubmitPostTask]) time.Duration { return 30 * time.Second }

type pollZernioStatusWorker struct {
	river.WorkerDefaults[PollZernioStatusTask]
	p *PollZernioStatusProcessor
}

func (w *pollZernioStatusWorker) Work(ctx context.Context, job *river.Job[PollZernioStatusTask]) error {
	return w.p.Process(ctx, job.Args)
}
func (w *pollZernioStatusWorker) Timeout(*river.Job[PollZernioStatusTask]) time.Duration {
	return 20 * time.Second
}

type cancelZernioJobWorker struct {
	river.WorkerDefaults[CancelZernioJobTask]
	p *CancelZernioJobProcessor
}

func (w *cancelZernioJobWorker) Work(ctx context.Context, job *river.Job[CancelZernioJobTask]) error {
	return w.p.Process(ctx, job.Args)
}
func (w *cancelZernioJobWorker) Timeout(*river.Job[CancelZernioJobTask]) time.Duration {
	return 20 * time.Second
}

type cleanupPostLogsWorker struct {
	river.WorkerDefaults[CleanupPostLogsTask]
	p *CleanupPostLogsProcessor
}

func (w *cleanupPostLogsWorker) Work(ctx context.Context, job *river.Job[CleanupPostLogsTask]) error {
	return w.p.Process(ctx, job.Args)
}
func (w *cleanupPostLogsWorker) Timeout(*river.Job[CleanupPostLogsTask]) time.Duration {
	return 30 * time.Second
}

type reconcileScheduledPostsWorker struct {
	river.WorkerDefaults[ReconcileScheduledPostsTask]
	p *ReconcileScheduledPostsProcessor
}

func (w *reconcileScheduledPostsWorker) Work(ctx context.Context, job *river.Job[ReconcileScheduledPostsTask]) error {
	return w.p.Process(ctx, job.Args)
}
func (w *reconcileScheduledPostsWorker) Timeout(*river.Job[ReconcileScheduledPostsTask]) time.Duration {
	return 30 * time.Second
}

type refreshZernioAnalyticsWorker struct {
	river.WorkerDefaults[RefreshZernioAnalyticsTask]
	p *RefreshZernioAnalyticsProcessor
}

func (w *refreshZernioAnalyticsWorker) Work(ctx context.Context, job *river.Job[RefreshZernioAnalyticsTask]) error {
	return w.p.Process(ctx, job.Args)
}
func (w *refreshZernioAnalyticsWorker) Timeout(*river.Job[RefreshZernioAnalyticsTask]) time.Duration {
	return 60 * time.Second
}

// WorkerSet bundles the six processors so server.go wires their
// dependencies in one place, then registers all workers at once.
type WorkerSet struct {
	Submit    *SubmitPostProcessor
	Poll      *PollZernioStatusProcessor
	Cancel    *CancelZernioJobProcessor
	Cleanup   *CleanupPostLogsProcessor
	Reconcile *ReconcileScheduledPostsProcessor
	Analytics *RefreshZernioAnalyticsProcessor
}

// Register adds every worker to the River workers registry. The analytics
// worker is always registered; whether it ever runs is governed by the
// presence of its PeriodicJob (see PeriodicJobs).
func (s WorkerSet) Register(workers *river.Workers) {
	river.AddWorker(workers, &submitPostWorker{p: s.Submit})
	river.AddWorker(workers, &pollZernioStatusWorker{p: s.Poll})
	river.AddWorker(workers, &cancelZernioJobWorker{p: s.Cancel})
	river.AddWorker(workers, &cleanupPostLogsWorker{p: s.Cleanup})
	river.AddWorker(workers, &reconcileScheduledPostsWorker{p: s.Reconcile})
	river.AddWorker(workers, &refreshZernioAnalyticsWorker{p: s.Analytics})
}

// PeriodicConfig carries the recurring-job cadences. The three recurring
// sweeps (cleanup, reconcile, analytics) are River periodic jobs rather
// than self-rescheduling tasks; River's scheduler owns the cadence and the
// chain can't die on a single failed tick.
type PeriodicConfig struct {
	CleanupEvery     time.Duration
	ReconcileEvery   time.Duration
	AnalyticsEvery   time.Duration
	IncludeAnalytics bool // only when the Zernio integration is configured
}

// PeriodicJobs builds the River periodic-job set. Each fires one interval
// after start (River's default; no RunOnStart), matching the old
// seed-with-Wait(every) behaviour.
func (cfg PeriodicConfig) PeriodicJobs() []*river.PeriodicJob {
	jobs := []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(cfg.CleanupEvery), func() (river.JobArgs, *river.InsertOpts) {
			return CleanupPostLogsTask{}, nil
		}, nil),
		river.NewPeriodicJob(river.PeriodicInterval(cfg.ReconcileEvery), func() (river.JobArgs, *river.InsertOpts) {
			return ReconcileScheduledPostsTask{}, nil
		}, nil),
	}
	if cfg.IncludeAnalytics {
		jobs = append(jobs, river.NewPeriodicJob(river.PeriodicInterval(cfg.AnalyticsEvery), func() (river.JobArgs, *river.InsertOpts) {
			return RefreshZernioAnalyticsTask{}, nil
		}, nil))
	}
	return jobs
}

// Enqueuer is the app-facing enqueue surface. It wraps the River client so
// callers (the schedule service, the posts handler) depend on a tiny method
// set rather than River directly. The client is generic over *sql.Tx
// because River runs on bun's shared database/sql pool — so InsertTx joins
// the exact bun transaction the schedule path opens.
type Enqueuer struct {
	Client *river.Client[*sql.Tx]
}

// EnqueueSubmitTx enqueues a submit task inside the given transaction, so
// the enqueue commits atomically with the post status change (CON-78 §9).
func (e *Enqueuer) EnqueueSubmitTx(ctx context.Context, tx *sql.Tx, postID string) error {
	if e == nil || e.Client == nil {
		return nil
	}
	_, err := e.Client.InsertTx(ctx, tx, SubmitPostTask{PostID: postID}, nil)
	return err
}

// EnqueueCancel enqueues a cancellation task (non-transactional).
func (e *Enqueuer) EnqueueCancel(ctx context.Context, postID string, target CancelTarget, actor string) error {
	if e == nil || e.Client == nil {
		return fmt.Errorf("jobs: enqueuer not configured")
	}
	_, err := e.Client.Insert(ctx, CancelZernioJobTask{PostID: postID, Target: target, Actor: actor}, nil)
	return err
}
