---
name: add-river-job
description: Add a background job (River queue) to the Go app — args type, worker/processor, self-registration, transactional enqueue, periodic scheduling, and tests. Use when the user asks to add an async/background task, a queue worker, a recurring sweep, polling, or to offload work out of a request handler.
tools: Read, Edit, Write, Glob, Grep, Bash
---

# Add River Job Skill

Scaffold a background job on **River** (`riverqueue/river`), the Postgres-native
queue this app adopted in CON-87 (replacing the SQLite-only backlite). Follow
the conventions below exactly — the project already has six jobs to diff
against. Canonical references in `src/jobs/queues/`:

- `submit_post_to_zernio.go` — one-shot, retries, enqueues a *different* job
  kind (the first poll) from inside the worker.
- `poll_zernio_status.go` — one-shot that **self-reschedules** with a dynamic
  cadence via `river.JobSnooze`.
- `cancel_zernio_job.go` — minimal one-shot.
- `cleanup_post_logs.go`, `reconcile_scheduled_posts.go`,
  `refresh_zernio_analytics.go` — **periodic** sweeps (marker args, unique).
- `river.go` — the central plumbing: `Deps`, the self-registration registry,
  `periodicUniqueOpts()`, `PeriodicConfig`/`PeriodicJobs()`, and `Enqueuer`.

## Architecture (read first)

- River runs on **bun's shared `database/sql` pool** via the
  **`riverdatabasesql`** driver — deliberately *not* `riverpgxv5`. This is what
  lets `client.InsertTx(ctx, tx.Tx, args, nil)` join the **exact bun
  transaction** the app already opened, so an enqueue commits atomically with a
  DB write (e.g. a post status change + its submit enqueue). riverpgxv5 needs a
  `pgx.Tx` and could not share bun's `*sql.Tx`. The client type is
  `*river.Client[*sql.Tx]`.
- River's own tables (`river_job`, `river_leader`, `river_queue`, …) are
  installed by `jobs.MigrateRiver(ctx, db.DB)` at boot (in `server.New`, before
  the client is built).
- The job **payload** (`args`) is JSON-stored in `river_job`. Keep args tiny —
  carry ids, re-load the row inside the worker (never persist stale field
  copies in the queue).

## Step 0: Clarify scope

1. **Job kind** — a stable snake_case name (e.g. `submit_post_to_zernio`,
   `cleanup_post_logs`). This is the queue name AND `Kind()`.
2. **One-shot or periodic?**
   - *One-shot* — enqueued in response to something (a user action, another
     job). Has retry semantics.
   - *Periodic* — a recurring sweep on a fixed cadence (cleanup, reconcile,
     refresh). Uses a marker args struct + `PeriodicJob`, not self-rescheduling.
3. **Payload** — what ids does the worker need? (Keep it to ids.)
4. **Dependencies** — which repos / clients does the work need?
5. **Enqueue site** — who enqueues it, and must it be **transactional** (commit
   atomically with a DB change)?
6. **Self-rescheduling?** — does it poll / repeat with a dynamic delay? (Use
   `river.JobSnooze`, see Step 4.)

---

## Step 1: The job file (`src/jobs/queues/<kind>.go`)

A job is **one file** containing the args, the worker, an `init()` registrar,
and the `Process` logic. Adding a job = adding one file; there is **no central
worker list** to edit (registration is decentralized — see Step 3).

```go
package queues

import (
	"context"
	"time"

	"github.com/riverqueue/river"
	// + repos / clients your Process needs
)

// <Kind>Queue is the River job-kind name.
const <Kind>Queue = "<kind>"

// <Kind>Task is the typed payload (args). Keep it to ids; the worker re-loads
// the row. River JSON-stores this in river_job.
type <Kind>Task struct {
	PostID string `json:"post_id"`
}

// Kind implements river.JobArgs.
func (<Kind>Task) Kind() string { return <Kind>Queue }

// InsertOpts sets per-kind defaults. MaxAttempts is the total attempt budget
// (1 = no retries). Backoff is River's default exponential-with-jitter — don't
// override NextRetry unless you need a fixed cadence.
func (<Kind>Task) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5}
}

// <Kind>Processor is the River worker. It embeds river.WorkerDefaults[T] and
// implements river.Worker directly; Process is the test seam Work delegates to.
type <Kind>Processor struct {
	river.WorkerDefaults[<Kind>Task]
	Deps ZernioDeps // or whatever this job needs
}

// Work is the River entrypoint; it delegates to Process.
func (p *<Kind>Processor) Work(ctx context.Context, job *river.Job[<Kind>Task]) error {
	return p.Process(ctx, job.Args)
}

// Timeout is the per-attempt context deadline.
func (p *<Kind>Processor) Timeout(*river.Job[<Kind>Task]) time.Duration {
	return 30 * time.Second
}

func init() {
	register(func(w *river.Workers, d Deps) {
		river.AddWorker(w, &<Kind>Processor{Deps: d.Zernio})
	})
}

// Process does the work. Return value semantics (see Step 4):
//   nil                -> job completed.
//   non-nil error      -> River retries per MaxAttempts + backoff.
//   river.JobSnooze(d) -> reschedule THIS job after d (no attempt consumed).
func (p *<Kind>Processor) Process(ctx context.Context, task <Kind>Task) error {
	// 1. Re-load the row by task.PostID.
	// 2. Bail quietly (return nil) if state changed underneath us.
	// 3. Do the work; persist; emit a PostLog if relevant.
	return nil
}
```

**Rules:**
- The processor **is** the worker (embed `river.WorkerDefaults[T]`). Don't add a
  separate wrapper type.
- Keep a `Process(ctx, args) error` method — it's the unit-test seam (tests call
  it directly with fakes, no DB / River client needed).
- Re-load the row in `Process`; never trust field copies from the args payload.
- Per-attempt timeout lives on the worker's `Timeout()`; `MaxAttempts` on the
  args' `InsertOpts()`.

---

## Step 2: Add the job's dependencies to `Deps`

Workers receive their deps from the single `queues.Deps` bundle in `river.go`.
If your job needs something not already there, add a field:

```go
// src/jobs/queues/river.go
type Deps struct {
	Zernio              ZernioDeps
	PostLogRetention    time.Duration
	ReconcileGrace      time.Duration
	// ... add yours; reuse Zernio's repos where possible (they're shared
	// instances — e.g. d.Zernio.PostRepo, d.Zernio.PostLogRepo).
}
```

`server.go` builds one `Deps` and passes it to `RegisterAll` (Step 6).

---

## Step 3: Self-registration (already wired)

Each job's `init()` appends a registrar to the package-level `registrars` slice
via `register(...)`. `RegisterAll(workers, deps)` iterates them. You do **not**
edit any central list — writing the `init()` in Step 1 is the whole
registration. (`river.AddWorker[T]` panics on a duplicate `Kind()`, which the
`register_test.go` smoke test catches.)

---

## Step 4: Return-value semantics & self-rescheduling

| Situation | Return |
|---|---|
| Done successfully | `nil` |
| Transient failure, want a retry | a non-nil `error` (counts as an attempt) |
| Terminal failure you already resolved (e.g. marked the post Failed) | `nil` — returning an error here would pointlessly retry. Optionally `river.JobCancel(reason)` to record a *cancelled* row with a reason. |
| Poll again later / dynamic cadence | `river.JobSnooze(delay)` |

**`river.JobSnooze(delay)`** reschedules the **same** job after `delay`. It does
**not** consume a retry attempt (it bumps `max_attempts`), so a poll can snooze
indefinitely while keeping its transient-error retry budget. Use it for
self-rescheduling cadence — see `poll_zernio_status.go`:

```go
if !job.Status.IsTerminal() {
	_ = p.Deps.PostRepo.Update(ctx, post) // persist last-seen status
	return river.JobSnooze(p.intervalFor(post))
}
```

Do **not** self-reschedule by inserting a fresh copy of the same job — that
creates a new `river_job` row each cycle and is the non-idiomatic anti-pattern
CON-87 removed.

**Enqueuing a *different* job kind from inside a worker** (e.g. submit → first
poll) *is* a genuine insert. Get the client from context:

```go
client, err := river.ClientFromContextSafely[*sql.Tx](ctx)
if err != nil || client == nil { return }
_, _ = client.Insert(ctx, PollZernioStatusTask{PostID: post.ID},
	&river.InsertOpts{ScheduledAt: when})
```

---

## Step 5: Enqueue surface (`Enqueuer`)

App code (handlers, services) must **not** import River. It depends on a narrow
interface implemented by `*queues.Enqueuer` (which wraps the River client).

**Transactional enqueue** (commit atomically with a DB write) — the whole point
of the riverdatabasesql driver. Add a method that calls `InsertTx` with the
caller's `*sql.Tx`:

```go
// src/jobs/queues/river.go
func (e *Enqueuer) Enqueue<Kind>Tx(ctx context.Context, tx *sql.Tx, postID string) error {
	if e == nil || e.Client == nil { return nil }
	_, err := e.Client.InsertTx(ctx, tx, <Kind>Task{PostID: postID}, nil)
	return err
}
```

Callers pass bun's underlying `*sql.Tx`, exposed as `tx.Tx` inside
`db.RunInTx(...)`:

```go
// in a service that already opened a bun transaction:
return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
	if _, err := tx.NewUpdate().Model(post).WherePK().Exec(ctx); err != nil {
		return err
	}
	return s.jobs.Enqueue<Kind>Tx(ctx, tx.Tx, post.ID) // commits or rolls back together
})
```

**Non-transactional enqueue** — `e.Client.Insert(ctx, <Kind>Task{...}, nil)`.

Define the consumer-side interface in the **consumer's** package (Go
convention), e.g. `schedule.SubmitEnqueuer` / `handlers.CancelEnqueuer`, with
just the method(s) that package needs — not the whole client.

---

## Step 6: Periodic jobs (recurring sweeps)

A recurring job is a **River PeriodicJob**, not a self-rescheduling task. River's
scheduler owns the cadence, so the chain can't die on one failed tick.

1. Marker args carry no per-tick data and use **`MaxAttempts: 1` +
   `periodicUniqueOpts()`**:

```go
func (<Kind>Task) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, UniqueOpts: periodicUniqueOpts()}
}
```

`periodicUniqueOpts()` (in `river.go`) uses **active states only**
(`Available/Pending/Running/Scheduled/Retryable`) so overlapping ticks can't
stack. **Never** use River's default `ByState` for a periodic marker — the
default includes `Completed`, which would block the next tick until River's job
cleaner removes the retained completed row (it breaks the cadence).

2. Register the schedule in `PeriodicConfig.PeriodicJobs()` (in `river.go`):

```go
river.NewPeriodicJob(river.PeriodicInterval(cfg.<Kind>Every),
	func() (river.JobArgs, *river.InsertOpts) { return <Kind>Task{}, nil }, nil),
```

Gate a conditional periodic job (e.g. only when an integration is configured)
behind a `PeriodicConfig` bool, like `IncludeAnalytics`.

3. The worker still self-registers (Step 1/3); a periodic job needs **both** the
   registered worker AND the `PeriodicJob` schedule.

---

## Step 7: Wire into server (`src/server/server.go`)

Already wired for the existing jobs; for reference the shape is:

```go
workers := river.NewWorkers()
queues.RegisterAll(workers, queues.Deps{ /* one bundle */ })

if err := jobs.MigrateRiver(ctx, db.DB); err != nil { return nil, err } // BEFORE NewClient

riverClient, err := river.NewClient[*sql.Tx](riverdatabasesql.New(db.DB), &river.Config{
	Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.JobWorkers}},
	Workers:      workers,
	PeriodicJobs: queues.PeriodicConfig{ /* cadences */ }.PeriodicJobs(),
})
enqueuer := &queues.Enqueuer{Client: riverClient}
// pass enqueuer to services/handlers via their narrow interfaces

riverClient.Start(ctx)
app.Hooks().OnShutdown(func() error {
	sctx, cancel := context.WithTimeout(context.Background(), cfg.JobShutdownTimeout)
	defer cancel()
	_ = riverClient.Stop(sctx)
	return nil
})
```

If you added a new periodic cadence or `Deps` field, update the
`queues.Deps{...}` / `PeriodicConfig{...}` literals here. New one-shot jobs
usually need **no** server.go edit (they self-register).

---

## Step 8: Tests (`src/jobs/queues/<kind>_test.go`)

Plain Go `testing` (the queue tests are not Ginkgo). Test `Process` **directly**
with fake repos — no DB, no River client.

```go
func Test<Kind>DoesX(t *testing.T) {
	deps, postRepo, _ := makeDeps(stub, nil) // see existing _test.go helpers
	post := seedScheduledPost(postRepo)
	postRepo.put(post)

	proc := &queues.<Kind>Processor{Deps: deps}
	if err := proc.Process(context.Background(), queues.<Kind>Task{PostID: post.ID}); err != nil {
		t.Fatalf("process: %v", err)
	}
	// assert persisted state via postRepo.GetByID
}
```

For a **self-rescheduling / poll** job, assert the snooze:

```go
err := proc.Process(ctx, queues.<Kind>Task{PostID: post.ID})
var snooze *river.JobSnoozeError
if !errors.As(err, &snooze) {
	t.Fatalf("expected a river JobSnooze, got %v", err)
}
```

The package's `register_test.go` already asserts every job self-registers
(`len(registrars)`) and `RegisterAll` wires them without a duplicate-kind panic
— **bump its expected count** when you add a job.

Run: `go test ./src/jobs/...`

---

## Gotchas (CON-87 lessons — read before you start)

1. **`riverdatabasesql`, not `riverpgxv5`.** Only the database/sql driver lets
   River share bun's `*sql.Tx`, which is what makes the transactional enqueue
   atomic. Don't "upgrade" to riverpgxv5 without a plan for the shared tx.
2. **Periodic `UniqueOpts` must exclude `Completed`.** Use `periodicUniqueOpts()`
   (active states only). River's default `ByState` includes `Completed` and
   would block the next periodic tick until the job cleaner runs — silently
   killing the cadence.
3. **`JobSnooze` for polling, not a fresh `Insert`.** Snooze reuses the same job
   and doesn't consume a retry attempt. A fresh insert spawns a new row each
   cycle.
4. **Terminal failure returns `nil`.** Once you've resolved state (e.g. marked
   the post Failed), returning an error just retries pointlessly. Return `nil`
   (or `river.JobCancel(reason)` for an explicit cancelled record).
5. **Args are JSON in `river_job`.** Keep them to ids and re-load; don't smuggle
   large/stale payloads.
6. **`MigrateRiver` runs before `NewClient`.** Already true in `server.New`; keep
   it that way for any new entrypoint (a test harness that exercises jobs must
   call it too — `pgtest.MustDB` does).
7. **One default queue.** All kinds share `river.QueueDefault` with
   `cfg.JobWorkers` workers. Only add a separate queue if a kind needs isolated
   concurrency.
8. **Self-registration via `init()`.** Don't reintroduce a central worker list;
   add the `init()` registrar in the job file and (for periodic) the schedule in
   `PeriodicJobs()`.

---

## Checklist

- Job file created (`src/jobs/queues/<kind>.go`): const + args (`Kind`,
  `InsertOpts`) + processor (embeds `WorkerDefaults`, `Work`, `Timeout`,
  `Process`) + `init()` registrar.
- New deps (if any) added to `queues.Deps` and populated in `server.go`.
- Enqueue: transactional `Enqueue<Kind>Tx` and/or non-tx method on `Enqueuer`;
  consumer interface defined in the consumer package.
- Periodic only: `periodicUniqueOpts()` on the marker; schedule added to
  `PeriodicConfig.PeriodicJobs()` (+ cadence field + server.go literal).
- Tests: `Process`-level unit test (+ snooze assertion if self-rescheduling);
  bumped `register_test.go` count.
- `go test ./src/jobs/...` green.
