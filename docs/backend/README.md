# Backend Reference (for Claude)

> Comprehensive reference for the Go backend of the **Content Control Center** app.
> Audience: Claude (and humans) working on this codebase. The frontend (`web/`)
> treats this backend as a **fixed API contract** — this doc is the source of truth
> for that contract plus the internals behind it.
>
> Generated from a full read of `src/` on 2026-06-09. When code changes, update the
> relevant section here. The auto-generated OpenAPI spec lives at `docs/swagger.json`
> (swag annotations on handlers) but is **less complete** than this reference.

## How this doc is organized

| File | Covers |
|---|---|
| `README.md` (this file) | Architecture, boot sequence, **consolidated route table**, **consolidated env/config**, cross-cutting conventions |
| [`01-auth-users-secrets.md`](./01-auth-users-secrets.md) | Auth/session flow, users, settings, secrets (+ envelope encryption), health, first-run setup |
| [`02-content-bank-assets.md`](./02-content-bank-assets.md) | Content-bank assets, image upload, post attachments, object storage, upload/PDF pipelines |
| [`03-campaigns-posts-platforms.md`](./03-campaigns-posts-platforms.md) | Campaigns, campaign types & phases, **posts (status state machine)**, tags, platforms, platform/post-type validation |
| [`04-publishing-zernio-jobs.md`](./04-publishing-zernio-jobs.md) | Zernio integration, auto-publish allowlist, post logs, SSE events, Backlite job queues, end-to-end publish flow |
| [`05-ai-genkit-flows.md`](./05-ai-genkit-flows.md) | Genkit runtime split, content-plan flow, post-assistant flow, embedding + RAG, AI config |
| [`06-data-layer-schema.md`](./06-data-layer-schema.md) | DB setup, **full SQL schema (all tables)**, IDs, password hashing, custom column types, repository catalog |

---

## 1. Architecture at a glance

Single Go binary serving both the REST API and the embedded React SPA.

- **Web framework:** Fiber v2 (`gofiber/fiber/v2`).
- **ORM / DB:** Bun (`uptrace/bun`) over SQLite. Driver `ncruces/go-sqlite3` (pure Go, also pulls `modernc` per go.mod history). **Single connection** (`SetMaxOpenConns(1)`), WAL journal mode.
- **Background jobs:** Backlite (`mikestefanello/backlite`) — SQLite-native typed job queues sharing the same DB file.
- **AI:** Firebase Genkit (Go) with two providers — **Anthropic Claude** (generative flows) and the hosted **Gemini Embedding 2** API via the `googlegenai` plugin (embeddings/RAG).
- **Object storage:** S3-compatible (Cloudflare R2 / DO Spaces / AWS S3) via AWS SDK v2, path-style addressing. Optional — disabled when `STORAGE_ENDPOINT` is empty.
- **External publisher:** **Zernio** (`zernio.com/api/v1`) for actually posting to social platforms. Optional — disabled when `zernio_api_key` is absent.
- **Secrets at rest:** envelope encryption (AES-256-GCM, KEK on a mounted volume) for third-party API keys.
- **Module path:** `github.com/ogen-app/ogen`. The app is internally code-named **"ogen"**.

The compiled SPA (`web/dist`) is embedded via `//go:embed` (`web/embed.go`) and served by Fiber as a fallback for any non-API route (`index.html` is the SPA fallback for client-side routing). One process serves API + UI in production.

### Backend package layout (`src/`)

| Package | Responsibility |
|---|---|
| `config` | env-driven config (`envconfig`). See §3. |
| `database` | open SQLite, set pragmas/single-conn, run embedded migrations (Bun migrator). |
| `models` | Bun ORM models + helpers (`id.go` Sqids, `password.go` argon2id, `types.go` custom column types). |
| `repository` | thin data-access layer over Bun; handlers depend on these interfaces. |
| `handlers` | Fiber route handlers grouped by resource; each exposes `Register(app)`. |
| `server` | wires repos → handlers, middleware, jobs, genkit, zernio; mounts SPA. `server.New(...)`. |
| `secrets` | encrypted secret store + KEK bootstrap + env→DB migration. |
| `crypto/envelope` | AES-256-GCM envelope encryption primitives. |
| `storage` | S3-compatible object storage abstraction (+ `imageprobe`, `pdfprobe`). |
| `platforms` | platform attachment constraints + per-post-type structural validation rules. |
| `publishers` + `publishers/zernio` | the `Publisher` interface + the Zernio client, bootstrap, sync worker, reconcile. |
| `jobs` + `jobs/queues` | Backlite runtime + the five background task queues. |
| `eventhub` | in-process pub/sub powering the SSE `/api/events` stream. |
| `postlog` | post-log payload sanitization + size capping. |
| `genkit/flows/*` | Anthropic generative flows (content_plan, post_assistant) + embedding/PDF flows. |
| `embedding` | Gemini (`googlegenai`) embedding plugin + flow registration. |
| `pdf` | PDF text extraction, page chunking, thumbnail rendering. |

### Boot sequence (`cmd/server/main.go` → `server.New`)

1. `config.Load()` — env → `*config.Config`.
2. `database.New(DSN, Debug)` — open SQLite, single conn, optional debug query logging, `Ping`.
3. `database.Migrate(ctx, db)` — apply all pending embedded migrations (idempotent; runs every startup).
4. `secrets.InitCipher(KEKPath)` — load/generate the KEK; **boot fails on any KEK error**.
5. `secrets.NewStore(...)` + `secrets.MigrateFromEnv(...)` — seed `anthropic_api_key` / `zernio_api_key` from env into the encrypted store on first boot (**DB always wins over env**; missing secret never fails boot).
6. `webstatic.DistFS()` — load embedded SPA.
7. `server.New(ctx, db, staticFS, cfg, store)` — wire everything (repos, handlers, eventhub, Zernio runtime in background goroutines, Backlite job runtime + queues, two Genkit runtimes), mount the SPA, return `*fiber.App`.
8. `app.Listen(cfg.Addr)`.

---

## 2. Consolidated route table

All API routes are under `/api` (plus `/admin/backlite/*` and `/debug/vars` ops routes). Default listen address is **`:9001`** (`ADDR`).

**Auth model:** most routes require a valid session cookie via `RequireAuth` middleware. A handful are public or conditionally open during first-run setup (marked below). Error responses are JSON `{"error": "..."}`; `*fiber.Error` status codes are preserved by `defaultErrorHandler`.

### Public

| Method | Path | Auth | Notes |
|---|---|---|---|
| GET | `/api/health` | **public** | DB ping + secret decryptability report; `503` if DB down |
| POST | `/api/sessions` | **public** | Login → sets session cookie, `201` |
| DELETE | `/api/sessions` | cookie | Logout (reads cookie itself), `204` |
| GET | `/api/integrations/zernio/health` | **public** | Integration state snapshot (DB-only) |
| POST | `/api/tenants` | **public** | Self-service signup → creates tenant + first user + session cookie, `201` (CON-97) |

### Authenticated (require session)

| Resource | Routes |
|---|---|
| **Current user** | `GET /api/current_user` |
| **Users** | `POST /api/users` (joins caller's tenant), `GET /api/users`, `GET/PUT/DELETE /api/users/:id` (PUT/DELETE are self-only) |
| **Settings** | `GET /api/settings`, `GET/PUT/DELETE /api/settings/:key` |
| **Secrets** | `GET /api/secrets`, `GET/PUT/DELETE /api/secrets/:name` (names: `anthropic_api_key`, `zernio_api_key`; values write-only) |
| **Content-bank assets** | `GET/POST /api/content-bank/assets`, `POST /api/content-bank/assets/upload`, `GET/PUT/DELETE /api/content-bank/assets/:id` |
| **Images** | `POST /api/images` (multipart, inline editor images) |
| **Tags** | `GET/POST /api/tags`, `GET/PUT/DELETE /api/tags/:id` |
| **Campaigns** | `GET/POST /api/campaigns`, `GET/PUT/DELETE /api/campaigns/:id`, `POST /api/campaigns/:id/generate-draft` (AI/SSE), `GET /api/campaigns/:campaign_id/posts` |
| **Campaign types** | `GET/POST /api/campaign_types`, `GET/PUT/DELETE /api/campaign_types/:id`, `POST /api/campaign_types/:id/clone`, `POST /api/campaign_types/:id/phases`, `PUT/DELETE /api/campaign_types/:id/phases/:phase_id` |
| **Platforms** | `GET/POST /api/platforms`, `GET/PUT/DELETE /api/platforms/:id`, `GET /api/platforms/:id/post-type-rules` |
| **Posts** | `GET/POST /api/posts`, `GET/PUT/DELETE /api/posts/:id`, `POST /api/posts/:id/assistant` (AI/SSE), `GET /api/posts/:id/messages`, `GET/POST /api/posts/:id/versions`, `POST /api/posts/:id/cancel` |
| **Post attachments** | `GET/POST /api/posts/:post_id/attachments`, `GET/PATCH/DELETE /api/posts/:post_id/attachments/:id` |
| **Post logs** | `GET /api/posts/:post_id/log`, `GET /api/post-logs` |
| **Zernio integration** | `GET /api/integrations/zernio/platforms`, `POST /api/integrations/zernio/connect-links`, `GET /api/integrations/zernio/accounts`, `POST /api/integrations/zernio/sync`, `POST /api/integrations/zernio/profile/repair` |
| **Auto-publish allowlist** | `GET/PUT /api/auto-publish-allowlist` |
| **Events (SSE)** | `GET /api/events?topics=...` |
| **Ops** | `GET/ALL /admin/backlite[/*]` (Backlite UI), `GET /debug/vars` (expvar) |

> The Zernio integration routes are only registered when a `zernio_api_key` is configured at boot. When storage is disabled, image/attachment upload endpoints return `503 "storage not configured"`.

---

## 3. Configuration / environment variables

All config is env-driven via `kelseyhightower/envconfig` (`src/config/config.go`). Defaults shown.

### Core / server

| Env | Default | Purpose |
|---|---|---|
| `ADDR` | `:9001` | Listen address |
| `DATABASE_DSN` | `file:data/app.db?cache=shared&_pragma=journal_mode(WAL)` | SQLite DSN |
| `DEBUG` | `false` | Dev mode: disables `Secure` flag on session cookies (HTTP localhost) + enables Bun query logging |
| `SESSION_COOKIE_NAME` | `c3_session` | Session cookie name |

### AI (Anthropic + content-plan)

| Env | Default | Purpose |
|---|---|---|
| `ANTHROPIC_API_KEY` | `""` | Bootstrap key; live key read from encrypted secret `anthropic_api_key` (rotating it hot-rebuilds the Genkit runtime) |
| `MODEL_ID` | `claude-sonnet-4-5-20250929` | Claude model (used as `anthropic/<MODEL_ID>`) |
| `MAX_OUTPUT_TOKENS` | `64000` | `max_tokens` for model calls |
| `MAX_ASSET_CONTEXT` | `15` | No-embedder fallback cap on asset count for content-plan context |
| `MAX_CONTEXT_CHARS` | `10000` | Char budget for packed asset chunks (content-plan) |
| `MAX_POSTS_PER_BATCH` | `30` | Posts per batched content-plan model call |
| `MAX_PARALLEL_BATCHES` | `5` | Concurrent content-plan batches |
| `GEMINI_API_KEY` | empty | Gemini Embedding 2 key (also settable via `PUT /api/secrets/gemini_api_key`; empty disables embedding/RAG) |
| `EMBED_MODEL` / `EMBED_DIMENSIONS` | `gemini-embedding-2` / `3072` | Embedding model + vector width |
| `GENKIT_ENV` | (unset) | Genkit dev-mode toggle (only logged at boot) |

### Object storage (S3-compatible)

| Env | Default | Purpose |
|---|---|---|
| `STORAGE_ENDPOINT` | `""` | S3 endpoint. **Empty disables all uploads** |
| `STORAGE_REGION` | `auto` | Region |
| `STORAGE_ACCESS_KEY` / `STORAGE_SECRET_KEY` | `""` | Static credentials |
| `STORAGE_BUCKET` | `""` | Bucket |
| `STORAGE_PUBLIC_URL` | `""` | CDN/public base URL for returned object URLs |

### Zernio publishing integration

| Env | Default | Purpose |
|---|---|---|
| `ZERNIO_API_KEY` | `""` | Bootstrap key; live key is secret `zernio_api_key`. **Empty disables the integration** |
| `ZERNIO_BASE_URL` | `https://zernio.com/api/v1` | REST base (includes `/api/v1`) |
| `ZERNIO_HTTP_TIMEOUT` | `15s` | Per-request timeout |
| `ZERNIO_SYNC_INTERVAL` | `30s` | Account-sync cadence (floor 10s) |
| `ZERNIO_SYNC_INTERVAL_FAST` | `5s` | Fast cadence after a connect link (floor 5s) |
| `ZERNIO_REDIRECT_URL` | `""` | Optional post-OAuth landing URL |

### Background jobs (Backlite) + lifecycle

| Env | Default | Purpose |
|---|---|---|
| `BACKLITE_WORKERS` | `4` | Worker-pool size |
| `BACKLITE_RELEASE_AFTER` | `5m` | Per-task lease window |
| `BACKLITE_CLEANUP_INTERVAL` | `1h` | Completed-row trim cadence |
| `BACKLITE_SHUTDOWN_TIMEOUT` | `30s` | Graceful drain wait |
| `RECONCILE_GRACE` | `1h` | Scheduled-post → Failed timeout window |
| `POSTLOG_RETENTION_DAYS` | `90` | Post-log retention (`0` disables cleanup) |

### Secrets / encryption

| Env | Default | Purpose |
|---|---|---|
| `OGEN_KEK_PATH` | `./kek` | Directory holding the KEK file (`<path>/kek.v1`); Docker mounts `/var/lib/ogen/keys` |

---

## 4. Cross-cutting conventions

- **Error shape:** all error responses are `{"error": "<message>"}`. The publish-gate `422` adds a `platform_validation` field. SSE streams convey errors as an `error` *event* (HTTP status is already `200` once the stream opened).
- **Auth:** `RequireAuth(sessionRepo, cookieName)` reads the session cookie, validates the session row + expiry, and stores `*models.Session` in `c.Locals("session")`. Missing/invalid/expired → `401`. See [`01-auth-users-secrets.md`](./01-auth-users-secrets.md).
- **IDs:** most PKs are random **Sqids** (`models.NewID()`); sessions use a random 32-byte token; `social_accounts.id` mirrors Zernio's accountId; `secret.id` is an integer rowid. See [`06-data-layer-schema.md`](./06-data-layer-schema.md).
- **Validation:** request bodies parsed with `c.BodyParser` and validated with `go-playground/validator/v10`; failures → `400` with a flattened message (`src/handlers/validate.go`).
- **Passwords:** argon2id, PHC-encoded (`models.HashPassword` / `VerifyPassword`). The docstring's "bcrypt" mention is historical.
- **JSON columns:** `StringSlice`, `CampaignPlatforms`, `PostTypeMap`, `ImageConstraints`, `PDFConstraints` are stored as TEXT/JSON via `driver.Valuer`/`sql.Scanner`.
- **Markdown vs BlockNote:** post/asset `content` is **Markdown** server-side; the frontend converts to/from BlockNote JSON.
- **No DB-level enum CHECK on post status** — the post status state machine is enforced in code (`models.ValidPostTransitions`). Assets/versions/messages DO have CHECK constraints.
- **Background work survives client disconnect:** AI flows persist drafts as they go; publishing runs entirely in Backlite jobs; SSE is best-effort notification, REST is the source of truth.

---

## 5. Build / run / test (from `Makefile`, `CLAUDE.md`)

- `make build` — build SPA → `web/dist`, then compile Go (`./server`). `web/dist` must exist (embedded).
- `make run` — live-reload server via `air`.
- `make test` — Ginkgo v2 + Gomega, `--randomize-all --randomize-suites -race -procs=2` (tests must be parallel-safe & order-independent).
- `make openapi` — regenerate `docs/` from swag annotations.
- Single package: `ginkgo ./src/handlers` or `go test ./src/handlers`.
