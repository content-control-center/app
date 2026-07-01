# Ogen Community Edition

[![Codacy Badge](https://app.codacy.com/project/badge/Grade/d5caed68fdc94fb49a1e52ea198d51b7)](https://app.codacy.com/gh/ogen-app/ogen/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_grade)
[![Semgrep](https://github.com/ogen-app/ogen/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/ogen-app/ogen/actions/workflows/docker-publish.yml)

> [!WARNING]  
> Use at your own risk. Ogen Community Edition is the open source community edition of Ogen and it is intended for users who want to self-host their own Ogen Community Edition instance. It is strictly recommended for personal, non-production use. Please review all installation and configuration steps carefully. Self-hosting requires advanced knowledge of server administration, database management, and securing sensitive data. Proceed only if you are comfortable with these responsibilities.
 
> [!TIP]
> For any commercial and enterprise-ready scheduling infrastructure - Ogen Enterprise Edition will be available in Q3 2026 

Ogen Community Edition is a single-tenant content
operations app that helps a creator plan, draft, and publish social
content end-to-end:

- **Plan** campaigns and individual posts with AI assistance grounded in your own content bank.
- **Draft** with a Markdown-first editor backed by an LLM assistant that can search assets, rewrite posts, and produce platform-specific variants.
- **Publish** automatically to LinkedIn, X, Facebook, Instagram, Threads, and YouTube via the [Zernio](https://zernio.com) broker — durable background jobs handle submission, polling, cancellation, and reconciliation, with a per-post audit log capturing every state change.

The backend is a Go 1.26 monolith (Fiber + Bun + Postgres). The
React + Vite SPA lives in its own repository,
[`ogen-app/ui`](https://github.com/ogen-app/ui), and deploys separately
(CON-98) — this repository is the API only.

---

## Capabilities

| Area | What's there |
|------|--------------|
| **Campaigns & posts** | Campaign types, campaigns, posts, post versions, post assistant (SSE-streamed Claude conversations), per-platform validation. |
| **Content bank** | Markdown notes and PDF assets with chunking, embeddings (Llama via `llama-genkit-embedder`), and similarity search used to ground the assistant. |
| **Post attachments** | Image uploads (JPEG/PNG/WebP/GIF) and PDF uploads with first-page thumbnail rendering, all backed by S3-compatible object storage (MinIO locally, Cloudflare R2 in deployment). Per-platform constraints (size/format/animated/page-count) surface as soft validation warnings. |
| **Zernio auto-publish** | `Publisher` abstraction with Zernio as the first concrete implementation. Submit / poll / cancel / retry queues are typed Backlite tasks; a reconciliation sweeper guards against stuck `Scheduled` posts. |
| **Post Log** | An auditable history table that records every meaningful operation against a post — state transitions, validation outcomes, allowlist decisions, background-task lifecycle, Zernio API interactions, reconciliation timeouts, user actions. Sanitized of secrets, capped at 64 KB per entry, configurable retention. |
| **Background jobs** | [Backlite](https://github.com/mikestefanello/backlite) on the same SQLite file — no external broker. Worker pool drains gracefully on shutdown; admin UI mounted at `/admin/backlite`. |
| **Secrets** | Envelope-encrypted at rest (per-secret DEK wrapped by a KEK on disk). Rotatable through the secrets API without restart. |
| **Observability** | `expvar` counters at `/debug/vars` for queue lifecycles, Zernio API latency, reconciliation timeouts, and Post Log retention activity. Backlite's UI shows running / upcoming / completed / failed tasks per queue. |

---

## Architecture

```
┌────────────────────────────┐         ┌────────────────────────────────────┐
│  React SPA (ogen-app/ui)   │◀───────▶│  Fiber HTTP API                    │
│  • separate deploy         │  REST   │  • Handlers (src/...)              │
│  • Markdown editor         │  + SSE  │  • Bun + SQLite (WAL)              │
└────────────────────────────┘         │  • Backlite worker pool            │
                                        │  ┌──────────────────────────────┐ │
                                        │  │ Genkit runtime               │ │
                                        │  │  Anthropic instance (rotat.) │ │
                                        │  │  Llama instance (stable)     │ │
                                        │  └──────────────────────────────┘ │
                                        └─┬─────────┬──────────┬──────────┬─┘
                                          │         │          │          │
                          ┌───────────────▼──┐ ┌────▼─────┐ ┌──▼──────┐ ┌─▼──────────────┐
                          │ Anthropic Claude │ │ Llama    │ │ MinIO   │ │ Zernio API     │
                          │ API (LLM)        │ │ embed    │ │ / R2    │ │ (cross-network │
                          │ • content plans  │ │ server   │ │ blob    │ │  publishing,   │
                          │ • post assistant │ │ (vector  │ │ storage │ │  status,       │
                          │                  │ │  embeds) │ │ (images,│ │  cancel,       │
                          │                  │ │ • assets │ │  PDFs)  │ │  reconcile)    │
                          │                  │ │ • PDFs   │ │         │ │                │
                          └──────────────────┘ └──────────┘ └─────────┘ └────────────────┘
                                  ▲                ▲
                                  │                │
                          consumed by         consumed by
                       Genkit Anthropic    Genkit Llama
                          instance            instance
```

**Foundational models in use**

| Model | Plugin | Used by | Hot-rotatable? |
|-------|--------|---------|----------------|
| **Anthropic Claude** (Sonnet 4.5 default; configurable via `MODEL_ID`) | `genkit/anthropic` | `generateContentPlan`, `postAssistant` | Yes — `PUT /api/secrets/anthropic_api_key` rebuilds the Anthropic Genkit instance without restart. |
| **Llama embedding model** (`embeddinggemma-300m` via `llama-genkit-embedder`) | `llama-genkit-embedder` | `embedAsset`, `processPDF` (chunk embedding for vector search) | No — pinned at boot since the embedder URL doesn't rotate; rebuild the model server image to upgrade. |

### Layered Go layout

```
cmd/server/        # main entrypoint; loads config, opens DB, runs server.New

src/
  config/          # envconfig-loaded Config struct
  database/        # bun.DB factory + migrations
  models/          # bun-tagged record structs (single source of truth for the schema)
  repository/      # narrow per-aggregate persistence layer
  handlers/        # Fiber HTTP handlers + swag annotations
  server/          # server.New: wires repos → handlers → background workers → routes
  jobs/            # Backlite runtime + expvar metrics
  jobs/queues/     # typed Backlite queues (submit/poll/cancel/reconcile/cleanup)
  publishers/      # Publisher abstraction; publishers/zernio is the concrete impl
  platforms/       # per-platform attachment validation rules
  postlog/         # PostLog payload helpers (Capper + Sanitize)
  secrets/         # envelope-encrypted secret store
  storage/         # S3-compatible blob storage; subpackages probe images/PDFs
  pdf/             # PDF text extraction + thumbnail rendering (poppler-utils)
  embedding/       # llama-genkit embedding flows + chunk repo
  genkit/          # Anthropic-backed Genkit flows (assistant, content plan)
  eventhub/        # in-process SSE event hub
  integration/     # `//go:build integration` suite running against MinIO + llama
```

### Persistence

All data lives in a single SQLite file (WAL journal mode). Bun is
the ORM; migrations live under `src/database/migrations/` and run on
every boot. Backlite uses the same `*sql.DB`, so background-job
state, Post Log entries, and domain rows can co-commit in one
transaction (used for the auto-publish schedule decision).

---

## AI flows (Genkit)

All AI-powered features are implemented as [Firebase Genkit](https://firebase.google.com/docs/genkit) flows under `src/genkit/flows/`. There are two long-lived Genkit instances at runtime:

- **Embedding instance** — built once at boot with the [`llama-genkit-embedder`](https://github.com/alephbet-ai/llama-genkit-embedder) plugin. Stable for the lifetime of the process; the Llama embed-server URL doesn't rotate.
- **Anthropic instance** — owned by `gkRuntime` (see `src/server/genkit_runtime.go`). Hot-rebuildable so a `PUT /api/secrets/anthropic_api_key` rotation rebuilds the instance and re-registers all Anthropic flows without restarting the server. When no key is set, the runtime stays nil and the consuming handlers return 503 via the `IsAnthropicAvailable` predicate.

| Flow | Plugin | Purpose | Triggered by |
|------|--------|---------|--------------|
| `embedAsset` | Llama | Chunks an asset's Markdown content (paragraph-aware, with overlap) and writes embeddings into `asset_chunks`. | Asset save callback (`OnMarkdownSave`). |
| `processPDF` | Llama | End-to-end PDF asset ingestion: extracts text via `ledongthuc/pdf` (with `pdftotext` fallback), renders a thumbnail with `pdftoppm`, uploads both the PDF and thumbnail to S3, then chunks + embeds. Fire-and-forget background callback. | Asset upload of a PDF file (`OnPDFProcess`). |
| `generateContentPlan` | Anthropic | Produces a per-phase, per-platform set of post drafts for a campaign. Splits the work into K-sized batches that run in parallel against Anthropic; surfaces SSE progress events (`batch_started`, `post_generated`, `complete`). See `src/genkit/flows/content_plan/PERFORMANCE.md` for the parallel-batching design. | `POST /api/campaigns/:id/generate-draft` (SSE). |
| `postAssistant` | Anthropic | Interactive post editor. Streams `explanation_delta` and `content_delta` SSE events as the model writes its rationale and the updated post body. Supports tool use against the asset library: `listAssets`, `getAssetChunks`, `searchAssetChunks`, `getCurrentContent`. Per-turn token budget enforced via the `packChunks` helper. | `POST /api/posts/:id/assistant` (SSE). |

Shared helpers in the same package:

- `chunker.go` — paragraph-aware chunking with configurable target size and overlap; estimates tokens via the standard 3.5-chars/token heuristic.
- The `prompts/` subdirectories under each flow contain the Markdown templates the flows render at request time.

### Genkit dev UI

When the server is started via `make genkit`, the Genkit dev UI mounts on `http://localhost:4000`, letting you replay flows interactively, inspect inputs/outputs, and step through tool calls. Today the dev UI sees only the embedding instance (the Anthropic instance is intentionally separate so key rotations don't disturb the UI session).

---

## Development workflows

There are two supported workflows depending on what you're working on:

| You're working on | Use this workflow | What you need installed |
|--------------------|-------------------|-------------------------|
| Frontend only | In the **[`ogen-app/ui`](https://github.com/ogen-app/ui)** repo — run its Vite dev server, which proxies `/api` to a locally-running API. | See that repo (Node + pnpm). |
| Backend (`src/`, `cmd/`) — including end-to-end testing of the SPA against an in-progress API | **[Backend development](#backend-development)** — build the API from source; point the UI at `http://localhost:9001`. | Go 1.26 + poppler-utils + Docker (for the Llama sidecar). |

The UI points at `http://localhost:9001` by default (via its Vite `/api` proxy), so a backend running locally from source is picked up automatically.

---

## Backend development

### Prerequisites

| Tool | Minimum version | Notes |
|------|-----------------|-------|
| Go | 1.26 | The module sets `go 1.26.1`. |
| `make` | any | Build / test / openapi / seed targets. |
| SQLite | bundled | The `ncruces/go-sqlite3` driver is pure Go; no system SQLite needed. |
| `pdftoppm`, `pdftotext` (poppler-utils) | any | Required for PDF thumbnail and text extraction. Optional for tests. |
| Docker Desktop | 4.x | For the integration suite (MinIO + llama-embedserver) and for running the full app stack locally. |

### Development setup

#### 1. Install Go and poppler-utils

```bash
# macOS
brew install go@1.26 poppler

# Debian / Ubuntu
sudo apt install -y golang-1.26 poppler-utils
```

`poppler-utils` provides `pdftoppm` (thumbnail rendering) and `pdftotext` (extraction fallback). The server still boots without them; PDF ingestion just degrades to text-only.

#### 2. Clone and pull deps

```bash
git clone https://github.com/ogen-app/ogen.git
cd app
go mod download
```

#### 3. Generate the KEK (envelope-encryption master key)

The first run generates one for you under `OGEN_KEK_PATH` (default `./kek/kek.v1`). **Back this file up** — losing it bricks every encrypted secret in the SQLite DB. For a production deployment, mount it from a separate volume.

#### 4. Start the Llama embedding sidecar

```bash
docker compose -f docker-compose.integration.yml up -d llama-embedserver
```

Or run the standalone image directly:

```bash
docker run --rm -p 8080:8080 alephbetai/llama-embedserver:latest
```

Set `EMBED_SERVER_URL` accordingly if it isn't on `http://localhost:8080`.

#### 5. Frontend (separate repo)

The API no longer embeds the SPA — `make build` produces just the API binary.
The frontend lives in [`ogen-app/ui`](https://github.com/ogen-app/ui) and
runs/deploys independently. To exercise the UI against this API, run the UI's
Vite dev server; it proxies `/api` to `http://localhost:9001`.

#### 6. Run with hot reload

```bash
make run    # uses `air` for live rebuilds on .go file changes
```

API listens on `http://localhost:9001`. Swagger UI: `http://localhost:9001/swagger/index.html`. Backlite UI (after login): `http://localhost:9001/admin/backlite`.

Or build + run once:

```bash
make build && ./server
```

#### 7. Configure secrets at runtime

You don't have to set `ANTHROPIC_API_KEY` / `ZERNIO_API_KEY` as env vars — the server boots without them and surfaces an "integration disabled" state. To enable them after boot:

```bash
# Create a session cookie first via POST /api/sessions, then:
curl -X PUT http://localhost:9001/api/secrets/anthropic_api_key \
  -H "Content-Type: application/json" \
  -b session=<your-session-cookie> \
  -d '{"value":"sk-ant-..."}'

curl -X PUT http://localhost:9001/api/secrets/zernio_api_key \
  -H "Content-Type: application/json" \
  -b session=<your-session-cookie> \
  -d '{"value":"sk_..."}'
```

Both keys are encrypted at rest with a per-secret DEK wrapped by the KEK. The Anthropic Genkit instance auto-rebuilds on the first call after the key is set; the Zernio client resolves the key per outbound request, so rotations land without restart.

### Common Make targets

```bash
make build           # build the server binary into ./server
make run             # air-powered hot reload for the API
make genkit          # boot the API with Genkit dev UI on :4000
make test            # ginkgo handler/unit suites with coverage
make test-integration # spins up MinIO + llama-embedserver, runs `//go:build integration` specs
make openapi         # regenerates docs/swagger.json from swag annotations
make tidy            # go mod tidy
make docker          # build the API Docker image
```

### Configuration

All runtime knobs are env-vars, loaded by [`envconfig`](https://github.com/kelseyhightower/envconfig). See `src/config/config.go` for the full list with documented defaults. Highlights:

| Variable | Default | Purpose |
|----------|---------|---------|
| `ADDR` | `:9001` | HTTP listen address |
| `DATABASE_DSN` | `file:data/app.db?cache=shared&_pragma=journal_mode(WAL)` | SQLite file location |
| `EMBED_SERVER_URL` | `http://localhost:8080` | Llama embed server |
| `OGEN_KEK_PATH` | `./kek` | Directory holding `kek.v1` for envelope encryption — losing it bricks every encrypted secret |
| `STORAGE_*` | empty | S3-compatible storage (R2/MinIO/AWS); empty disables uploads |
| `ANTHROPIC_API_KEY` | empty | Claude key — also settable via `PUT /api/secrets/anthropic_api_key` for hot rotation |
| `ZERNIO_API_KEY` / `ZERNIO_BASE_URL` | empty / `https://zernio.com/api/v1` | Zernio integration; key resolved per outbound call so rotations land without restart |
| `BACKLITE_WORKERS` / `BACKLITE_RELEASE_AFTER` / `BACKLITE_SHUTDOWN_TIMEOUT` | 4 / 5m / 30s | Background job runtime |
| `RECONCILE_GRACE` | `1h` | Reconciliation timeout for stuck Scheduled posts |
| `POSTLOG_RETENTION_DAYS` | `90` | Post Log retention window |

### Testing

The handler suite uses real in-memory SQLite (no mocks) — every
test boots through the migration chain. The integration suite
(`//go:build integration`) brings up MinIO and llama-embedserver via
`docker-compose.integration.yml` and runs end-to-end flows
including the post-attachment round-trip.

```bash
make test                  # handler/unit (fast, in-process)
make test-integration      # MinIO + llama (slow, docker compose)
```

### Admin surfaces

When the server is running, the following routes are mounted behind
the standard session-cookie auth (`RequireAuth`):

| Route | Purpose |
|-------|---------|
| `/admin/backlite` | Backlite UI — running / upcoming / completed / failed tasks per queue |
| `/debug/vars` | `expvar` JSON — `ogen_jobs_*` counters and Zernio API latency average |
| `/api/posts/:id/log` | Per-post audit history |
| `/api/post-logs?event_type=…&since=…` | Cross-post audit search |

---

## Frontend development

The web UI lives in its own repository — **[`ogen-app/ui`](https://github.com/ogen-app/ui)** (CON-98).
Clone it and follow its README to run the Vite dev server, which proxies `/api`
to a locally-running API (`http://localhost:9001` by default) or any deployed API.

To run the API the UI talks to, see **[Backend development](#backend-development)**
above, or start the published image plus its dependencies:

```bash
docker compose pull && docker compose up   # API + Postgres + llama-embedserver
```

### Full stack via Docker Compose (API + UI hot-reload)

The `ui` service in `docker-compose.yml` mounts a sibling `ogen-app/ui` checkout
and runs the Vite dev server with hot-reload, proxying `/api` to the API. It's
gated behind the `ui` profile so the default `docker compose up` stays
backend-only:

```bash
docker compose --profile ui up         # API + deps + UI dev server
```

Then open **http://localhost:9002**. Edits under the local `ui` repo hot-reload
in the browser. If your UI checkout isn't at `../ui`, point `UI_PATH` at it:

```bash
UI_PATH=/path/to/ui docker compose --profile ui up
```

When the UI and API are served from different origins, set `CORS_ALLOWED_ORIGINS`
on the API to the UI origin(s); for local dev the Vite `/api` proxy keeps things
same-origin, so it can stay empty.

### API documentation

The full REST API reference (OpenAPI) is generated from swag annotations via
`make openapi` and published from `docs/swagger.json` on every merge to `main`.

---

## Repository skills (`.agents/skills/`)

The `.agents/skills/` directory ships with two **scaffolding skills** for [Claude Code](https://claude.ai/code) (and other agent harnesses that honour the same `SKILL.md` convention). Each skill encodes the project's existing patterns so a new feature lands looking like the rest of the codebase — not like an LLM guessed.

| Skill | When to invoke | What it scaffolds |
|-------|---------------|-------------------|
| **`add-entity`** | "Add a new resource / table / domain entity" — anything that needs a full vertical slice (migration → handler). | Migration up/down, `bun`-tagged model, narrow `Repository` interface + impl, Fiber handler with swag annotations, `server.New` wiring, ginkgo handler tests against in-memory SQLite, `http-client/<entity>/<entity>.http` example file. Asks up-front about entity name, fields/types/nullability, enums, relationships, and owner-scoping (`created_by` + `ON DELETE CASCADE`). |
| **`add-genkit-flow`** | "Add an AI assistant / generator / classifier" — anything LLM-driven that needs prompt templates, structured output, tool use, or progress streaming. | Genkit flow package mirroring `post_assistant` / `content_plan`: types, Markdown prompt template, `run.go` with the direct closure used for SSE streaming, optional tool definitions, context cache with `assembleContext` + `postFingerprint`-style invalidation, init function on `gkRuntime`, Fiber handler that streams `*_delta` events, ginkgo tests. |

Both skills follow the project's [feedback rules](../../.claude/projects/-Users-serhii-go-src-github-com-ogen-app-ogen/memory/) — real in-memory SQLite tests at the handler layer (no mocks), authz-before-lookup with 403-over-404, `feature/` branch naming — and refuse to invent new patterns where canonical references (`post_assistant`, `content_plan`, the existing repos and handlers) already exist to diff against.

To use them:

```bash
# Inside Claude Code, or any agent harness that loads .agents/skills:
/skill add-entity         # then describe the entity
/skill add-genkit-flow    # then describe the flow
```

The `SKILL.md` files are themselves the spec — frontmatter declares the skill name + description + allowed tools; the body is the step-by-step playbook the agent follows. Read them directly if you want to understand or audit the patterns the project considers canonical.

---
