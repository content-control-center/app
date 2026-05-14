# Auto-publishing a post via Zernio

How a Post moves from `ReadyForPublish` → `Scheduled` → `Published` when the
target platform is allowed to auto-publish via [Zernio](https://zernio.com/),
our outbound publishing aggregator.

This is the CON-69 happy path. Cancel, manual retry, and reconciliation are
covered at the end.

---

## TL;DR

1. User PUTs a post with `status: scheduled`. The handler decides whether
   Zernio is allowed to publish for this platform (allowlist lookup). The
   status change, the audit entries, and (if allowed) the submit Backlite task
   are written in **one SQLite transaction**.
2. Backlite picks up `submit_post_to_zernio`. It validates the post, calls
   `POST /posts` on Zernio, stores the returned job ID, and enqueues a poll.
3. Backlite picks up `poll_zernio_status` close to `scheduled_at`. It polls
   Zernio at 30s cadence (60s after a 5-minute window), and when Zernio
   reports a terminal state (`published`, `failed`, `partial`), it lands the
   Post and writes the audit entry.
4. A separate `reconcile_scheduled_posts` task sweeps for stuck Scheduled
   posts (grace exceeded) and forces them to `Failed` so nothing silently
   hangs.

---

## The decision: who gets to auto-publish?

`ReadyForPublish → Scheduled` is the *only* user-driven transition that
consults the auto-publish allowlist. The allowlist is keyed by Zernio's wire
ID for the platform (`"linkedin"`, `"x"`, etc.), not our internal Sqid.

```
            user PUTs status=scheduled
                       │
                       ▼
        allowlistRepo.Contains(zernio_id)?
                ┌──────┴───────┐
              yes              no
                │              │
                ▼              ▼
          Scheduled    ScheduledForManualPublish
        (auto path)    (manual; no Backlite work)
```

Both states are valid Post statuses; the difference is that
`ScheduledForManualPublish` does **not** enqueue any Zernio work. From there,
publishing is a manual action outside this flow.

The chosen status is also surfaced to the client via the
`X-Auto-Publish-Decision` response header — useful for UI ("we scheduled
this" vs "we parked this for manual approval").

---

## Step 1 — HTTP: `PUT /api/posts/:id`

Entry point: `(*PostsHandler).Update` in `src/handlers/posts.go`.

When the incoming transition is `ReadyForPublish → Scheduled` *and* all
scheduling deps are wired (allowlist repo, Backlite client, Bun DB), control
flows into `routeAndPersistSchedule`. Otherwise the handler takes the
default save path.

Inside `routeAndPersistSchedule`, exactly three writes happen in a single
`db.RunInTx`:

| # | Write | Notes |
|---|---|---|
| 1 | `UPDATE posts SET status=…` | Status set to `Scheduled` or `ScheduledForManualPublish`. |
| 2 | `INSERT post_log` × 2 | `allowlist_decision` (with payload `{platform, zernio_platform, auto_publish, chosen_status}`) and `state_transition`. |
| 3 | `INSERT backlite_tasks` (`SubmitPostTask`) | **Only** when `auto_publish=true`. Joins the same `*sql.Tx` via `jobsClient.Add(...).Tx(tx.Tx).Save()`. |

If any one write fails, all three roll back together. The user never sees a
status change without the matching audit row or queued task, and Backlite
never has an orphan task pointing to a post that didn't actually transition.

The HTTP response is `200 OK` with the fully hydrated Post (re-fetched after
the transaction).

---

## Step 2 — Backlite: `submit_post_to_zernio`

File: `src/jobs/queues/submit_post_to_zernio.go`.

Task payload is minimal — just the Post Sqid. The processor reloads the post
from the repo to avoid acting on stale field copies.

The handler does, in order:

1. **Guard the status.** If the post is no longer `Scheduled` (user cancelled,
   reconciler intervened), it logs a no-op and returns success.
2. **Idempotency branch.** If `post.ZernioPostID` is already set, this is a
   manual retry of a previously-submitted post — call Zernio's
   `POST /posts/:id/retry` instead of creating a new job.
3. **Validate the publishing context.** Resolve the post's platform via
   `zernio.LookupSupportedBySqid`, find the connected social account for that
   Zernio platform, ensure a Zernio profile is wired. Each failure mode has
   its own terminal reason code (`missing_platform`, `unsupported_platform`,
   `no_profile`, `no_account_connected`, `integration_disabled`).
4. **Build the Zernio submit request** with content, platform variant, the
   scheduled time (`post.ScheduledAt` or now+1m), and `UTC`.
5. **Call `zernio.Client.Submit`** (`POST /posts`). On success: persist
   `ZernioPostID` and `ZernioStatus` to the Post, log `zernio_submit` with
   the new IDs, and **enqueue the poll task** scheduled to run shortly before
   `scheduled_at` (`PollLeadTime`, defaults to 30s).

### Error handling

Each Zernio call goes through `zernio.Client`, which classifies errors as
terminal (don't retry) vs transient (let Backlite retry per the queue's
`MaxAttempts`/`Backoff`). Two terminal cases are interesting:

- **`ErrDuplicateContent` (409)** — Zernio rejects a submit that looks like a
  resubmit of recent content. We attempt recovery via `Client.FindByContent`
  (24h window). If we find the prior job, we treat it as success and persist
  its ID. If not, we mark the post `Failed` with reason
  `zernio_dedupe_unrecoverable`.
- **Other terminal API errors** — mark the post `Failed` with reason
  `zernio_rejected: <message>`. Backlite stops retrying because the processor
  returned `nil`.

The terminal path is centralized in `SubmitPostProcessor.terminal`:

- Set `post.Status = Failed`, write `post.FailureReason = "<reason>: <msg>"`.
- Append a `state_transition` PostLog entry capturing the reason.
- Return `nil` so Backlite stops retrying.

Transient errors return the error so Backlite can retry per the queue config,
and a `task_retried` audit entry is written each time.

---

## Step 3 — Backlite: `poll_zernio_status`

File: `src/jobs/queues/poll_zernio_status.go`.

The poll task is self-rescheduling. Cadence is bimodal:

- **Fast window** — 30s polls for the first 5 minutes after `scheduled_at`,
  when the publication is most likely to land.
- **Slow window** — 60s polls thereafter.

Each invocation:

1. Reloads the Post. If it's no longer `Scheduled`, exits cleanly (user cancelled,
   already terminal).
2. Calls `zernio.Client.Status(post.ZernioPostID)`.
3. Writes back `post.ZernioStatus` even for non-terminal states (cheap; aids
   debugging via `/debug/vars` and `post_log` payloads).
4. If terminal:
   - **`published`** → `post.Status = Published`,
     `post.PublishedAt = now`, `post.PublishedResults = <per-platform JSON>`,
     `state_transition` log with summary `"Zernio reported published"`.
   - **`failed` / `partial`** → `post.Status = Failed`,
     `post.FailureReason = "zernio_terminal: <status>"`, store per-platform
     results so the UI can show what succeeded and what didn't.
5. If non-terminal, self-enqueue the next poll per the cadence above and return.

Transient errors from Zernio retry per the queue config. Terminal API errors
during polling do **not** mark the post Failed — they bail out and rely on
the reconciler (step 4) to time the post out if Zernio truly never recovers.
This avoids spurious Failed states when Zernio has a short outage.

---

## Step 4 — Backlite: `reconcile_scheduled_posts` (safety net)

File: `src/jobs/queues/reconcile_scheduled_posts.go`.

A recurring task (default cadence: `Every`, e.g. hourly) that asks one
question: *is there a Scheduled post whose `scheduled_at + Grace` has
elapsed without resolving?* If so, force it to `Failed` with reason
`reconciliation_timeout: scheduled_at=… elapsed=… last_zernio_status=…` and
emit a `reconciliation_timeout` PostLog entry.

The prefix is deliberate — it distinguishes "Zernio said no" from "we never
heard back" in `post.FailureReason`. That matters when triaging incidents.

The reconciler runs independently of submit/poll. It is the only thing that
guarantees a stuck Scheduled post will eventually leave that state.

---

## Cancellation: `POST /api/posts/:id/cancel`

Out-of-band from the publish flow but interleaves with it.

`(*PostsHandler).Cancel` enqueues `CancelZernioJobTask` (HTTP returns
`202 Accepted` immediately). It does *not* synchronously change the Post's
status — the post stays `Scheduled` until Zernio confirms cancellation.

The processor in `src/jobs/queues/cancel_zernio_job.go` handles four
outcomes:

| Zernio result | Post outcome |
|---|---|
| Cancel succeeded | Post transitions to the requested target (`ready_for_publish` or `draft`); `zernio_cancel` and `state_transition` logged. |
| **Race: already published** (`ErrAlreadyPublished`) | No transition. The next poll cycle lands `Published` per the normal path. Logged as `cancel race: Zernio reports already published`. |
| Terminal API error | Post stays `Scheduled`. The user can retry from the UI. |
| Transient error | Backlite retries per queue config. |

If `post.ZernioPostID` is empty (cancel arrived before submit ran), we skip
the Zernio call and just transition locally.

---

## State machine touched by this flow

Defined in `src/models/post.go` (`ValidPostTransitions`). The transitions
involved here:

```
Draft ─────────────► ReadyForPublish ─────► Scheduled ─────► Published
                                          │              │
                                          │              ├─► Failed
                                          │              ├─► ReadyForPublish (cancel)
                                          │              └─► Draft (cancel)
                                          │
                                          └─► ScheduledForManualPublish
                                                  ├─► Published
                                                  └─► NotPublished

Failed ─► ReadyForPublish (manual retry)
```

Every transition attempted by a handler or queue processor goes through
`Post.Status.CanTransition`. Rejected transitions emit a
`state_transition_blocked` audit entry and surface a 400 to the caller.

---

## Audit trail: PostLog events along the way

Every meaningful step writes a row to `post_log`. For a textbook
auto-publish run, the chronological sequence is:

| Order | Event type | Where it fires | Summary |
|---|---|---|---|
| 1 | `allowlist_decision` | `routeAndPersistSchedule` | `auto-publish allowlist decision` |
| 2 | `state_transition` | `routeAndPersistSchedule` | `status changed via PUT /api/posts/:id (schedule)` |
| 3 | `zernio_submit` | `SubmitPostProcessor` (pre-call) | `calling Zernio POST /posts` |
| 4 | `zernio_submit` | `SubmitPostProcessor` (success) | `Zernio submit succeeded; polling scheduled` |
| 5 | `state_transition` | `PollZernioStatusProcessor` (terminal) | `Zernio reported published` |

Variations:

- **Submit failed terminally** — entry 4 becomes a `state_transition` to
  `Failed` with payload `{reason, message}`.
- **Submit retried transiently** — interleave `task_retried` entries between
  3 and 4.
- **User cancelled mid-flight** — `user_cancel` then `zernio_cancel` then a
  `state_transition` to the cancel target.
- **Reconciliation kicked in** — `reconciliation_timeout` replaces entry 5.

The audit trail is best-effort: PostLog write failures during the
non-transactional steps (submit, poll, cancel) are swallowed so logging never
fails the operation it's describing.

---

## Code map

| Concern | File |
|---|---|
| HTTP entry, allowlist routing, transactional triple-write | `src/handlers/posts.go` (`Update`, `routeAndPersistSchedule`) |
| Cancellation endpoint | `src/handlers/posts.go` (`Cancel`) |
| Zernio submit task | `src/jobs/queues/submit_post_to_zernio.go` |
| Zernio poll task | `src/jobs/queues/poll_zernio_status.go` |
| Reconciliation safety net | `src/jobs/queues/reconcile_scheduled_posts.go` |
| Cancel task | `src/jobs/queues/cancel_zernio_job.go` |
| Shared queue deps | `src/jobs/queues/zernio_deps.go` |
| Zernio HTTP client | `src/publishers/zernio/client.go`, `…/posts.go` |
| Platform Sqid → Zernio ID mapping | `src/publishers/zernio/platforms.go` (`LookupSupportedBySqid`) |
| Allowlist repository | `src/repository/auto_publish_allowlist.go` |
| State machine | `src/models/post.go` (`ValidPostTransitions`, `CanTransition`) |
| PostLog event constants | `src/models/post_log.go` |
