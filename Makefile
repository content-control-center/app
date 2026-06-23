.PHONY: build run test test-integration coverage _ginkgo _air _pg-test-up tidy docker docker-genkit clean openapi seed genkit

GINKGO_FLAGS = --github-output -r -randomize-all -randomize-suites -race -trace -procs=2 -poll-progress-after=10s -poll-progress-interval=10s

# Postgres for the unit suite (CON-87 WS5). A throwaway pgvector container on
# host port 5433 — avoids clashing with a dev `docker compose` on 5432 — with
# max_connections bumped because each test provisions its own database.
PG_TEST_CONTAINER = ogen-test-pg
PG_TEST_DSN = postgres://ogen:ogen@localhost:5433/postgres?sslmode=disable

# ── Go ────────────────────────────────────────────────────────────────────────
build:
	go build -o server ./cmd/server

seed:
	go run ./cmd/seed/...

run: _air
	air

_air:
	go install github.com/air-verse/air@latest

_ginkgo:
	go install github.com/onsi/ginkgo/v2/ginkgo

_pg-test-up:
	@docker rm -f $(PG_TEST_CONTAINER) >/dev/null 2>&1 || true
	@docker run -d --name $(PG_TEST_CONTAINER) \
		-e POSTGRES_USER=ogen -e POSTGRES_PASSWORD=ogen -e POSTGRES_DB=postgres \
		-p 5433:5432 pgvector/pgvector:pg17 -c max_connections=500 >/dev/null
	@printf "Waiting for postgres"; \
	tries=30; \
	until docker exec $(PG_TEST_CONTAINER) pg_isready -U ogen >/dev/null 2>&1; do \
		tries=$$((tries - 1)); \
		[ $$tries -le 0 ] && { echo " timed out"; docker rm -f $(PG_TEST_CONTAINER) >/dev/null 2>&1; exit 1; }; \
		printf '.'; sleep 1; \
	done; echo " ready"

test: _ginkgo _pg-test-up
	@TEST_DATABASE_DSN="$(PG_TEST_DSN)" ginkgo $(GINKGO_FLAGS) --skip-package=integration --cover --coverpkg=./... --coverprofile=coverage.out --covermode=atomic --output-dir=. ./...; \
	EXIT=$$?; \
	docker rm -f $(PG_TEST_CONTAINER) >/dev/null 2>&1; \
	[ $$EXIT -eq 0 ] && go tool cover -func=coverage.out; \
	exit $$EXIT

test-integration:
	docker compose -f docker-compose.integration.yml up -d
	@printf "Waiting for llama-embedserver"; \
	timeout=90; \
	until curl -sf http://localhost:9003/health 2>/dev/null | grep -q ok; do \
		timeout=$$((timeout - 2)); \
		[ $$timeout -le 0 ] && { echo " timed out"; docker compose -f docker-compose.integration.yml down; exit 1; }; \
		printf '.'; sleep 2; \
	done; echo " ready"
	@printf "Waiting for minio"; \
	timeout=60; \
	until curl -sf http://localhost:9100/minio/health/live >/dev/null 2>&1; do \
		timeout=$$((timeout - 2)); \
		[ $$timeout -le 0 ] && { echo " timed out"; docker compose -f docker-compose.integration.yml down; exit 1; }; \
		printf '.'; sleep 2; \
	done; echo " ready"
	@printf "Waiting for postgres"; \
	timeout=60; \
	until docker compose -f docker-compose.integration.yml exec -T postgres pg_isready -U ogen -d ogen >/dev/null 2>&1; do \
		timeout=$$((timeout - 2)); \
		[ $$timeout -le 0 ] && { echo " timed out"; docker compose -f docker-compose.integration.yml down; exit 1; }; \
		printf '.'; sleep 2; \
	done; echo " ready"
	TEST_DATABASE_DSN="postgres://ogen:ogen@localhost:5432/postgres?sslmode=disable" \
		go test -tags integration -v -count=1 -timeout 180s ./src/integration/...; \
	EXIT=$$?; \
	docker compose -f docker-compose.integration.yml down; \
	exit $$EXIT

tidy:
	go mod tidy

# ── OpenAPI ──────────────────────────────────────────────────────────────────
openapi:
	go install github.com/swaggo/swag/cmd/swag@latest
	swag init -g main.go -d cmd/server,src/handlers,src/models,src/platforms,src/secrets,src/repository,src/genkit/flows/post_quality -o docs --outputTypes go,json

# ── Genkit ───────────────────────────────────────────────────────────────────
genkit:
	npx genkit start -- go run ./cmd/server

# ── Docker ───────────────────────────────────────────────────────────────────
docker:
	docker build -t ogen .

docker-genkit:
	docker compose -f docker-compose.genkit.yml up --build

# ── Cleanup ──────────────────────────────────────────────────────────────────
clean:
	rm -f server
	rm -f seed
	# rm -rf data/*
	rm -f coverage.out
