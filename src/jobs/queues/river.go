package queues

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/publishers/zernio"
)

// This file holds the central River plumbing for the queue package: the
// dependency bundle, the self-registration registry, the periodic-job set,
// and the app-facing enqueue surface.
//
// Workers are NOT listed here. Each job is a processor (in its own file) that
// implements river.Worker directly (Work/Timeout next to its logic) and
// self-registers from an init() via register(). Adding a job therefore means
// adding one file — no central list to edit.

// Deps bundles every dependency the workers need. server.go builds one and
// passes it to RegisterAll; each worker's registrar picks what it needs. The
// cleanup/reconcile repos are the same instances already held by the Zernio
// bundle, so they aren't duplicated here.
type Deps struct {
	Zernio              ZernioDeps
	PostLogRetention    time.Duration
	ReconcileGrace      time.Duration
	AnalyticsSettings   zernio.SettingsStore
	AnalyticsHub        eventhub.Hub
	AnalyticsWindowDays int
	// CON-102: eager per-tenant Zernio profile provisioning. The bootstrap job
	// reuses the connect-link handler's Bootstrapper (tenant-scoped profile
	// create/adopt + settings write) and Integration (enabled/state gating).
	ProfileBootstrapper *zernio.Bootstrapper
	Integration         *zernio.Integration

	// CON-103: the process_pdf worker's dependencies (pdf-service client,
	// embedder, storage, asset repos). A nil Client (no PDF_SERVICE_ADDR) makes
	// the job a no-op.
	PDF PDFDeps
}

// registrars is appended to by each worker file's init(). A job is registered
// simply by existing in this package.
var registrars []func(*river.Workers, Deps)

// register records a worker registrar. Called from each job file's init().
func register(fn func(*river.Workers, Deps)) {
	registrars = append(registrars, fn)
}

// RegisterAll adds every self-registered worker to the River registry.
func RegisterAll(workers *river.Workers, deps Deps) {
	for _, fn := range registrars {
		fn(workers, deps)
	}
}

// periodicUniqueOpts is the uniqueness applied to the periodic markers
// (cleanup/reconcile/analytics): a new tick is skipped only while a previous
// one is still ACTIVE. Completed/cancelled/discarded are deliberately excluded
// — including them (River's default ByState) would block the next tick until
// the job cleaner removes the retained completed row, breaking the cadence.
func periodicUniqueOpts() river.UniqueOpts {
	return river.UniqueOpts{
		ByState: []rivertype.JobState{
			rivertype.JobStateAvailable,
			rivertype.JobStatePending,
			rivertype.JobStateRunning,
			rivertype.JobStateScheduled,
			rivertype.JobStateRetryable,
		},
	}
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

// PeriodicJobs builds the River periodic-job set. Every job runs once on
// start (RunOnStart) and then every interval. Without RunOnStart the first
// tick fires only one full interval after the client starts, and each process
// restart re-bases that timer from zero — so a long-interval sweep (analytics
// at 30m, cleanup at 1h) can be starved indefinitely in a frequently-restarting
// environment (dev live-reload, crash-loops, rapid deploys) and never fire its
// first tick. RunOnStart makes each boot deterministically produce one tick;
// the active-state UniqueOpts dedupe overlapping inserts across a fast restart,
// and every sweep is idempotent (upserts) or a no-op when there's nothing to
// do, so the extra boot tick is harmless in a stable process too.
func (cfg PeriodicConfig) PeriodicJobs() []*river.PeriodicJob {
	runOnStart := &river.PeriodicJobOpts{RunOnStart: true}
	jobs := []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(cfg.CleanupEvery), func() (river.JobArgs, *river.InsertOpts) {
			return CleanupPostLogsTask{}, nil
		}, runOnStart),
		river.NewPeriodicJob(river.PeriodicInterval(cfg.ReconcileEvery), func() (river.JobArgs, *river.InsertOpts) {
			return ReconcileScheduledPostsTask{}, nil
		}, runOnStart),
	}
	if cfg.IncludeAnalytics {
		jobs = append(jobs, river.NewPeriodicJob(river.PeriodicInterval(cfg.AnalyticsEvery), func() (river.JobArgs, *river.InsertOpts) {
			return RefreshZernioAnalyticsTask{}, nil
		}, runOnStart))
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

// requestIDMetadata returns River job metadata carrying the originating request
// id (CON-107) when one is present on ctx, so a job's logs correlate back to the
// HTTP request that enqueued it. Returns nil for system/periodic enqueues that
// have no request id (River treats nil as empty metadata).
func requestIDMetadata(ctx context.Context) []byte {
	rid, ok := logging.RequestIDFrom(ctx)
	if !ok {
		return nil
	}
	b, err := json.Marshal(map[string]string{logging.AttrRequestID: rid})
	if err != nil {
		return nil
	}
	return b
}

// insertOptsWithRequestID merges the originating request id into opts as job
// metadata (creating opts if nil). When ctx carries no request id, opts is
// returned unchanged.
func insertOptsWithRequestID(ctx context.Context, opts *river.InsertOpts) *river.InsertOpts {
	meta := requestIDMetadata(ctx)
	if meta == nil {
		return opts
	}
	if opts == nil {
		opts = &river.InsertOpts{}
	}
	opts.Metadata = meta
	return opts
}

// WithJobRequestID re-attaches the originating request id (from the job row's
// metadata) to a worker context so the job's log lines carry the same
// request_id as the enqueuing request. Tenant attachment stays the worker's
// responsibility (tenantctx.With). Takes the *rivertype.JobRow (not the
// promoted job.Metadata field) so it is nil-safe: unit tests that construct a
// river.Job without a JobRow, and jobs enqueued without metadata, both return
// ctx unchanged rather than panicking.
func WithJobRequestID(ctx context.Context, jr *rivertype.JobRow) context.Context {
	if jr == nil || len(jr.Metadata) == 0 {
		return ctx
	}
	var m struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(jr.Metadata, &m); err == nil && m.RequestID != "" {
		ctx = logging.WithRequestID(ctx, m.RequestID)
	}
	return ctx
}

// EnqueueSubmitTx enqueues a submit task inside the given transaction, so
// the enqueue commits atomically with the post status change (CON-78 §9).
func (e *Enqueuer) EnqueueSubmitTx(ctx context.Context, tx *sql.Tx, postID string) error {
	if e == nil || e.Client == nil {
		return nil
	}
	_, err := e.Client.InsertTx(ctx, tx, SubmitPostTask{PostID: postID}, insertOptsWithRequestID(ctx, nil))
	return err
}

// EnqueueBootstrapProfileTx enqueues an eager Zernio profile-provisioning task
// inside the given transaction, so it commits atomically with the tenant insert
// (CON-102 §6 FR2): a rolled-back signup creates no job, a committed one
// durably queues exactly one. A nil enqueuer (Zernio queue unwired) is a no-op —
// the lazy on-connect bootstrap remains the fallback.
func (e *Enqueuer) EnqueueBootstrapProfileTx(ctx context.Context, tx *sql.Tx, tenantID string) error {
	if e == nil || e.Client == nil {
		return nil
	}
	_, err := e.Client.InsertTx(ctx, tx, BootstrapZernioProfileTask{TenantID: tenantID}, insertOptsWithRequestID(ctx, nil))
	return err
}

// EnqueueProcessPDFTx enqueues a PDF-ingestion task inside the given
// transaction, so it commits atomically with the asset insert (CON-103): a
// committed upload always has a job, a rolled-back one never does. The worker
// re-reads original.pdf from storage, so the bytes are not in the args. Takes
// primitives so the handler can depend on a narrow interface, not this package.
func (e *Enqueuer) EnqueueProcessPDFTx(ctx context.Context, tx *sql.Tx, assetID, tenantID, originalName, mimeType string) error {
	if e == nil || e.Client == nil {
		return nil
	}
	_, err := e.Client.InsertTx(ctx, tx, ProcessPDFTask{
		AssetID:      assetID,
		TenantID:     tenantID,
		OriginalName: originalName,
		MimeType:     mimeType,
	}, insertOptsWithRequestID(ctx, nil))
	return err
}

// EnqueueCancel enqueues a cancellation task (non-transactional).
func (e *Enqueuer) EnqueueCancel(ctx context.Context, postID string, target CancelTarget, actor string) error {
	if e == nil || e.Client == nil {
		return fmt.Errorf("jobs: enqueuer not configured")
	}
	_, err := e.Client.Insert(ctx, CancelZernioJobTask{PostID: postID, Target: target, Actor: actor}, insertOptsWithRequestID(ctx, nil))
	return err
}
