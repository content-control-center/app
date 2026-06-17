# Publishing Pipeline: Zernio, Auto-Publish, Jobs, Post Logs, Events (SSE)

Covers `src/handlers/{zernio,auto_publish_allowlist,post_logs,events}.go`,
`src/server/zernio.go`, `src/publishers/**`, `src/jobs/**`, `src/eventhub/**`,
`src/postlog/postlog.go`, and the related models.

The publishing subsystem turns Ogen posts into live social posts via the external
**Zernio** service (`zernio.com/api/v1`). Two platform-ID vocabularies coexist: Zernio's
wire IDs (`twitter`, `linkedin`, …) and Ogen's local platform Sqids; the allowlist
(`src/publishers/zernio/platforms.go`) maps between them.

---

## 1. Configuration & secrets

See [README §3](./README.md#3-configuration--environment-variables) for `ZERNIO_*`,
`BACKLITE_*`, `RECONCILE_GRACE`, `POSTLOG_RETENTION_DAYS`.

- Secret **`zernio_api_key`** (`secrets.NameZernioAPIKey`) — resolved on **every** outbound request via a `KeyResolver`, so rotation through the secrets API takes effect without restart. Empty key at boot → integration disabled.
- Settings keys (table `settings`, `zernio.` prefix, 30s TTL cache): `zernio.profile_id`, `zernio.profile_name`, `zernio.profile_created_at`, `zernio.profile_meta`, `zernio.last_sync_at`, `zernio.last_sync_status`. Managed profile: `ManagedProfileName = "Ogen integration"`.

---

## 2. The Zernio client (`src/publishers/zernio/{client,posts,profiles,accounts}.go`)

`zernio.Client` wraps Zernio's REST API; bearer key fetched per request. `NewClient`
returns **nil** when the resolver is nil (= disabled sentinel). Non-2xx → typed
`*APIError{Status, Message}` (body capped 4 KB, secrets stripped). Helpers:
`IsStatus`, `IsTerminalAPIError` (4xx except 429), `IsTransientAPIError` (5xx/429/network).

| Method | Zernio call | Notes |
|---|---|---|
| `Ping` | `GET /profiles` | boot key validation |
| `ListProfiles` / `GetProfile` / `CreateProfile` | `/profiles[...]` | |
| `CreateConnectLink(profileID, platform)` | `GET /connect/{platform}?profileId=…[&redirect_url=…]` | returns `authUrl` |
| `ListAccounts(profileID)` | `GET /accounts?profileId=…` | client re-filters by profile |
| `Submit(SubmitRequest)` | `POST /posts` | returns `*Job`; **409 → `ErrDuplicateContent`** (24h dedupe) |
| `Status(jobID)` | `GET /posts/{id}` | → `*Job` |
| `Cancel(jobID)` | `DELETE /posts/{id}` | 404/409 → `ErrAlreadyPublished` (no-op) |
| `Retry(jobID)` | `POST /posts/{id}/retry` | re-attempt existing job |
| `FindByContent(content, lookback)` | `GET /posts?status=scheduled&…` | dedupe recovery |

`JobStatus` enum: `draft`, `scheduled`, `published`, `failed`, `partial`
(`IsTerminal()` → published/failed/partial). `Integration` state: `disabled`, `degraded`, `ok`.

---

## 3. Boot wiring — `initZernio` (`src/server/zernio.go`)

Never blocks boot on Zernio reachability:
1. Wrap settings in a 30s-TTL cached store shared by handlers/bootstrap/worker.
2. Read `zernio_api_key`. **Missing → disabled** (nil Client/Bootstrapper/Worker/RateLimiter).
3. Else build Client → Integration → Bootstrapper → Worker → RateLimiter.
4. Two background goroutines (cancelable ctx):
   - **`warmupZernio`**: `Ping` → 401 ⇒ `StateDisabled` (admin must `/profile/repair`); transport/5xx ⇒ `StateDegraded`; 200 ⇒ degraded then `bootstrapper.Run` (→ `StateOK`).
   - **`worker.Run`**: account-sync loop.
5. `shutdown()` (Fiber `OnShutdown`) cancels ctx, waits ≤2s.

**Profile bootstrap** (`bootstrap.go`): idempotent; ensures exactly one profile named
`Ogen integration` exists (adopt stored/oldest match, else create). Backoff `{1s,2s,4s,8s}`;
401/403 short-circuit → `StateDisabled`.

**Sync worker** (`worker.go`, `reconcile.go`): mirrors Zernio accounts into the
`social_accounts` table. Per tick: list remote + local → pure `Reconcile()` →
`ApplyPlan(upserts, softDeletes)` in one tx → publish per-change events → write
`last_sync_at`/`last_sync_status`. Decisions: attached/revived/updated/disconnected
(soft-delete). Cadence honors rate-limit backoff (429 → ×2, cap 5m), then fast window
(`FastUntil`), then floors. **401 → `StateDisabled`, loop exits** until restart/repair.
`TriggerNow()` coalesces an immediate tick.

---

## 4. HTTP API

### 4.1 Zernio integration (`/api/integrations/zernio`)

`GET /health` is **public**; the rest require `auth`.

- **`GET /health`** → `200` `{enabled, state, profileId?, lastSyncAt?, lastSyncStatus?, accountCount}` (DB-only, no Zernio call).
- **`GET /platforms`** (deprecated; prefer `GET /api/platforms`) → `200` `{platforms:[{id:<ZernioID>, label, supportedPostTypes:[]}]}`.
- **`POST /connect-links`** — body `{platform}` (Zernio wire ID, allowlisted). → `200` `{platform, connectUrl, expiresAt}`. Calls `CreateConnectLink`; `BumpFastUntil(now+10m)` to tighten sync. Codes: `400` invalid_platform/body; `409 integration_disabled`; `503 integration_degraded` (state≠ok or no profile); `429 rate_limited` (30/min/instance, `Retry-After`); `502` on Zernio APIError.
- **`GET /accounts`** → `200` `{accounts:[{id, platform, username, displayName, avatarUrl, isActive, connectedAt, lastSyncedAt}], lastSyncAt, lastSyncStatus}` (DB-only). `409` if disabled.
- **`POST /sync`** → `202` `{previousLastSyncAt}`; `worker.TriggerNow()`. `409` if disabled.
- **`POST /profile/repair`** → re-runs `bootstrapper.Run` → `202` `{state}`; `409` disabled, `500` on error.

### 4.2 Auto-publish allowlist (`/api/auto-publish-allowlist`, `auth`)

Stores **Zernio wire identifiers** (e.g. `linkedin`). Decides auto-publish (`scheduled`)
vs manual (`scheduled_for_manual_publishing`). Empty by default (opt-in).
- **`GET /`** → `200` `{platforms:[]}`.
- **`PUT /`** — body `{platforms:[]}` — **replace, not append** (`[]` clears). Each value must satisfy `zernio.LookupSupportedPlatform` → else `400 platform "x" is not in the Ogen-supported set`.

Model `AutoPublishAllowlistEntry` (table `auto_publish_allowlist`): `platform_id` (pk), `created_at`.

### 4.3 Post logs (`auth`, read-only)
Writes happen at each originating event site, not via an API.
- **`GET /api/posts/:post_id/log`** — verifies post exists (`404`); query `limit` (default 500). → `200 []PostLog` **chronological (oldest first)**.
- **`GET /api/post-logs`** — query (all optional): `post_id`, `event_type`, `actor`, `since`/`until` (RFC3339), `limit` (default 200). → `200 []PostLog` **newest first**; `400` on bad RFC3339 / negative limit.

`models.PostLog` (table `post_logs`): `id`, `post_id`, `event_timestamp`, `event_type`,
`actor` (user id or `"system"`), `from_status?`, `to_status?`, `payload`, `summary`.

**`PostLogEventType` vocabulary:** `state_transition`, `state_transition_blocked`,
`validation_passed`, `validation_failed`, `allowlist_decision`, `task_enqueued`,
`task_started`, `task_succeeded`, `task_failed`, `task_retried`, `task_panicked`,
`zernio_submit`, `zernio_poll`, `zernio_cancel`, `zernio_retry`, `reconciliation_timeout`,
`user_schedule`, `user_cancel`, `user_retry`.

**Sanitization** (`src/postlog/postlog.go`): `MaxPayloadBytes = 64<<10`. `Sanitize`
redacts JSON values for keys matching `api_key|access_key|secret|password|token|
authorization|x-zernio-signature|webhook_secret` and `Authorization: Bearer <tok>`.
`SanitizeAndCap` redacts then truncates on a UTF-8 boundary.

### 4.4 Server-Sent Events — `GET /api/events` (`auth`)
Streams eventhub events to the authenticated user.
- Query **`topics`** (required, comma-separated). Forms: `all`; exact `kind:id`; prefix `kind:*`. Blank → `400`.
- `Last-Event-ID` accepted and **ignored** (reserved for replay).
- Response `text/event-stream`; frames `id:`/`event:`/`data:` (type falls back to `message`).
- **Heartbeat** `: ping` immediately and every **20s**; session rechecked on each heartbeat (logout/expiry closes the stream).
- **Backpressure:** at-most-once; buffer overflow closes the channel → client must reconnect + reconcile via REST.
- Codes: `400` (no topics), `401`, `429` (`ErrTooManySubscribers`, default 10/user).

---

## 5. Eventhub (`src/eventhub/{eventhub,topic}.go`)

In-process pub/sub. `Event{ID, Topic, Type, Payload, CreatedAt, UserID}` (UserID `""` =
public, else only that user's subscribers). `Hub.Publish` never blocks (slow subscribers
disconnected). Two filters: topic match (`all`/exact/`kind:*`) + authorization. Defaults:
`BufferSize=128`, `MaxSubscribersPerUser=10`.

**Zernio sync events** (from the worker):
- Sync-wide on `zernio:sync`: `zernio.sync.ok` / `zernio.sync.failed` (payload `{summary}`).
- Per-account on `entity:zernio_account:<id>`: `zernio.account.{attached,attach_failed,updated,disconnected,revived}` (payload `{id, platform, username, displayName, isActive[, error]}`).

(AI flows also publish to `entity:campaign:<id>` and `entity:post:<id>` — see
[`05-ai-genkit-flows.md`](./05-ai-genkit-flows.md).)

---

## 6. Job runtime (`src/jobs/{backlite,metrics}.go`)

`jobs.Runtime` wraps a single **Backlite** client over the app's `*sql.DB`. `jobs.New`
installs the schema (idempotent). `Register(queue)`, `Start(ctx)`, `Shutdown(ctx)` (drains
≤ `ShutdownTimeout`). All five queues are registered in `server.go`; drained via Fiber
`OnShutdown`.

**expvar metrics** (`GET /debug/vars`, auth-gated): `ogen_jobs_zernio_submit_*`,
`ogen_jobs_zernio_poll_*`, `ogen_jobs_zernio_cancel_*`, `ogen_jobs_reconciliation_timeouts`,
`ogen_jobs_postlog_*`, `ogen_jobs_zernio_latency_ms_avg`. Backlite ops UI at
`/admin/backlite` (auth-gated).

`ZernioDeps` (shared bundle): `PostRepo`, `PostLogRepo`, `PostAttachmentRepo`,
`SocialAccountRepo`, `Client`, `ProfileID` (lazy resolver of `zernio.profile_id`).

---

## 7. The five queues (`src/jobs/queues/*.go`)

### 7.1 `submit_post_to_zernio` — `SubmitPostTask{post_id}`
`MaxAttempts:5`, `Backoff:30s`, `Timeout:30s`. Enqueued by `routeAndPersistSchedule` (only when allowlisted), joined to the same tx as the status update + logs.
1. Abort quietly if post no longer `Scheduled`.
2. Manual-retry path (`ZernioPostID != ""`) → `Client.Retry` (`zernio_retry` log); terminal → fail, transient → retry.
3. Fresh submit: resolve platform/profile/active account → build `SubmitRequest` (`ScheduledFor = scheduled_at` or now+1m, `Timezone:"UTC"`). `Client.Submit`. **409 → `FindByContent(24h)`** recovery or terminal. Terminal APIError → post `failed`; transient → retry.
4. `persistSuccess`: set `ZernioPostID`/`ZernioStatus`, log `zernio_submit`, **enqueue `poll_zernio_status`** at `scheduled_at + PollLeadTime`(30s).

Terminal failures move the post to **`failed`** with `FailureReason = "<reason>: <msg>"` + `state_transition` log; returns nil error so Backlite stops retrying.

### 7.2 `poll_zernio_status` — `PollZernioStatusTask{post_id}`
`MaxAttempts:3`, `Backoff:30s`, `Timeout:20s`. Enqueued by submit, then **self-reschedules** (`intervalFor`: 30s within 5m of `scheduled_at`, 60s after).
1. Exit if post not `Scheduled` or no `ZernioPostID`.
2. `Client.Status`. Terminal APIError (e.g. 404) → log, stop (reconciler handles it). Transient → retry. Success → persist `ZernioStatus`.
3. Non-terminal → update + reschedule.
4. **Terminal:** `published` → post **`published`** (set `PublishedAt`, `PublishedResults=json(platforms)`); `failed`/`partial` → post **`failed`** (`FailureReason="zernio_terminal: <status>"`, `PublishedResults` saved). (`partial` treated as Failed for MVP; per-platform detail in the log.) Each writes `state_transition`.

### 7.3 `cancel_zernio_job` — `CancelZernioJobTask{post_id, target, actor}`
`MaxAttempts:3`, `Backoff:30s`, `Timeout:20s`. Enqueued by `POST /api/posts/:id/cancel`. Target ∈ `ready_for_publish`/`draft`.
1. Abort if not `Scheduled`. No `ZernioPostID` → transition locally.
2. `Client.Cancel`. Success → log + transition. **`ErrAlreadyPublished`** → no local transition (next poll lands `published`). Terminal APIError → leave in `Scheduled` (user retries). Transient → retry.
3. `transition` honors `CanTransition` (else `state_transition_blocked`); logs attributed to the requesting **actor**.

### 7.4 `reconcile_scheduled_posts` — `ReconcileScheduledPostsTask{}`
`Timeout:30s`. Registered `Grace=RECONCILE_GRACE`, `Every=5m`; seeded once at boot, self-reschedules. `ListStuckScheduled(now-Grace, 100)` → each forced **`failed`** with reason prefixed **`reconciliation_timeout`** (`scheduled_at`, `elapsed`, `last_zernio_status`). The safety net for dead poll tasks / 404'd jobs.

### 7.5 `cleanup_post_logs` — `CleanupPostLogsTask{}`
`Timeout:30s`. Registered `Retention=POSTLOG_RETENTION_DAYS*24h`, `Every=1h`; seeded once at boot, self-reschedules. `DeleteOlderThan(now-Retention)`. `POSTLOG_RETENTION_DAYS=0` disables deletion.

---

## 8. End-to-end publishing flow

**Prerequisite — connect an account:** `POST /connect-links` → user authorizes on Zernio →
sync worker mirrors the account into `social_accounts` (`zernio.account.attached` events).

**Publishing a post:**
1. **`Draft → ReadyForPublish`** (`PUT /api/posts/:id`): attachment + per-platform validation gate; fail → `422` + `validation_failed`; pass → `validation_passed`.
2. **`ReadyForPublish → Scheduled` (allowlist decision):** `routeAndPersistSchedule` maps platform Sqid → Zernio ID, checks `allowlistRepo.Contains`.
   - **Allowlisted → `scheduled`** + `submit_post_to_zernio` enqueued **in the same tx** as the status update + `allowlist_decision`/`state_transition` logs.
   - **Not allowlisted → `scheduled_for_manual_publishing`** (no submit task; user publishes manually later).
3. **Submit:** `submit` calls `POST /posts` (or `/retry` for manual retries; 409 dedupe recovery), stores `ZernioPostID`/`ZernioStatus`, logs `zernio_submit`, schedules the first poll.
4. **Poll:** `poll_zernio_status` calls `GET /posts/:id` (30s/5m → 60s), self-rescheduling while non-terminal.
5. **Resolution:** Zernio `published` → post **`published`**; `failed`/`partial` → post **`failed`**. Each writes `state_transition`.
6. **Reconcile sweeper:** every 5m, any `Scheduled` post past `scheduled_at + RECONCILE_GRACE` with no terminal status → forced **`failed`** (`reconciliation_timeout`).
7. **Cancellation:** `POST /api/posts/:id/cancel` (only `Scheduled`) → `cancel_zernio_job`. Success → post moves to the chosen target. Race (`ErrAlreadyPublished`) → no-op, next poll lands `published`.
8. **Manual retry:** user moves `Failed → ReadyForPublish` (`user_retry`) → `Scheduled`; since `ZernioPostID` persists, submit hits `/retry` (no duplicate).

**SSE notification:** status changes are audited in `post_logs` (read via `/api/posts/:id/log`,
`/api/post-logs`); account/sync changes push over `/api/events`. The frontend subscribes
to topic filters and reconciles via REST on reconnect/backpressure.

---

## Models

- `models.SocialAccount` (table `social_accounts`): see [`06-data-layer-schema.md`](./06-data-layer-schema.md). PK = Zernio accountId; soft-delete via `deleted_at`.
- `models.AutoPublishAllowlistEntry` (table `auto_publish_allowlist`): `platform_id` (pk, Zernio wire id), `created_at`.
- `models.PostLog` (table `post_logs`): see §4.3.
