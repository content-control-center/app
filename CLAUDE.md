# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

All Go tasks go through the `Makefile`. Web tasks can also be run directly via `npm` inside `web/`.

- `make build` — builds the React SPA into `web/dist`, then compiles the Go binary to `./server`. The Go binary embeds `web/dist` via `web/embed.go`, so `web/dist` MUST exist before `go build` will succeed.
- `make run` — installs `air` if missing and runs the server with live reload (config in `.air.toml`).
- `make test` — installs Ginkgo and runs the full Go test suite with race detector and coverage (writes `coverage.out`). Tests use Ginkgo v2 + Gomega.
- Run a single Go test package: `ginkgo ./src/handlers` or `go test ./src/handlers -run TestHandlers`.
- Focus a single Ginkgo spec: add `FDescribe`/`FIt`/`FContext` in the spec file, or pass `--focus="<regex>"` to ginkgo.
- `make openapi` — regenerates `docs/` from swag annotations on `cmd/server`, `src/handlers`, `src/models`.
- `make tidy` — `go mod tidy`.
- `make docker` — builds the production image using the multi-stage `Dockerfile`.
- `make web-dev` — runs the Vite dev server standalone (port 5173).
- `make clean` — removes `server`, `web/dist`, `web/node_modules`, `coverage.out`.

Frontend-only workflow (no Go toolchain needed) is documented in `README.md`: `docker compose pull && docker compose up`, then open http://localhost:5173. The Vite dev server proxies to the API container on port 3000.

## Architecture

This is a single-binary Go + React app. The compiled React SPA is embedded into the Go binary via `//go:embed dist` in `web/embed.go`, and Fiber serves it as a fallback for any non-API route (with `index.html` as the SPA fallback to support client-side routing). One process serves both the API and the UI in production.

### Backend layout

- `cmd/server/main.go` — entry point. Loads config → opens DB → runs migrations → builds the Fiber app → listens. Also holds the top-level swag annotations for OpenAPI generation.
- `src/config` — env-driven config via `kelseyhightower/envconfig`. Notable: `DEBUG=true` disables the `Secure` flag on session cookies so localhost over plain HTTP works in development.
- `src/database` — opens SQLite (modernc.org/sqlite, pure Go), forces a single connection (SQLite likes this), and runs embedded SQL migrations from `src/database/migrations` on every startup via Bun's migrator. New migrations are picked up automatically.
- `src/models` — Bun ORM models (`User`, `Session`, `Setting`, plus `id.go` and `password.go` helpers).
- `src/repository` — thin data-access layer over Bun. Handlers depend on these interfaces, not on `*bun.DB` directly. Add new persistence here.
- `src/handlers` — Fiber route handlers grouped by resource (`auth`, `health`, `sessions`, `users`, `settings`). Each handler type is constructed with its repositories in `src/server/server.go` and exposes a `Register(app)` method that mounts its routes. Authentication is enforced via `handlers.RequireAuth(sessionRepo, cookieName)` middleware, which reads the session cookie configured by `SESSION_COOKIE_NAME` (default `c3_session`).
- `src/server/server.go` — wires everything together: builds Fiber, registers middleware (`recover`, `logger`), wires repos → handlers, and finally mounts the embedded SPA at `/`. The custom `defaultErrorHandler` converts errors to JSON `{ "error": ... }` responses, preserving `*fiber.Error` status codes.

When adding a new resource: create the model in `src/models`, a migration in `src/database/migrations`, a repository in `src/repository`, a handler in `src/handlers` with a `Register` method, and wire it up in `src/server/server.go`.

### Frontend layout

- `web/` — Vite + React 18 + TypeScript SPA. Source under `web/src`. Build output `web/dist` is what gets embedded by the Go binary; do not commit it.
- Vite env vars must be prefixed `VITE_` (see README). `web/.env.local` is gitignored for local overrides.

### Tests

- Go tests use Ginkgo v2 + Gomega. The handlers suite is bootstrapped from `src/handlers/suite_test.go`. Spec files are `*_test.go` files alongside the code under test, in the `<pkg>_test` package.
- `make test` flags: `--randomize-all --randomize-suites -race -procs=2`, so tests must be parallel-safe and order-independent.

## Conventions

- Do not commit `web/dist/` or `web/node_modules/` — both are build artifacts.
- OpenAPI docs in `docs/` are generated; regenerate via `make openapi` after changing swag annotations rather than editing by hand.
- Database file lives at `data/app.db` locally (see default DSN in `src/config/config.go`) and at `/data/app.db` inside the container (see `docker-compose.yml`).