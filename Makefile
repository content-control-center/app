.PHONY: build run test test-integration coverage _ginkgo _air web web-dev tidy docker docker-genkit clean openapi seed genkit

GINKGO_FLAGS = --github-output -r -randomize-all -randomize-suites -race -trace -procs=2 -poll-progress-after=10s -poll-progress-interval=10s

# ── Go ────────────────────────────────────────────────────────────────────────
build: web/dist
	go build -o server ./cmd/server

seed:
	go run ./cmd/seed/...

run: web/dist _air
	air

_air:
	go install github.com/air-verse/air@latest

_ginkgo:
	go install github.com/onsi/ginkgo/v2/ginkgo

test: _ginkgo
	ginkgo $(GINKGO_FLAGS) --skip-package=integration --cover --coverpkg=./... --coverprofile=coverage.out --covermode=atomic --output-dir=. ./...
	go tool cover -func=coverage.out

test-integration:
	docker compose -f docker-compose.integration.yml up -d
	@printf "Waiting for llama-embedserver"; \
	timeout=90; \
	until curl -sf http://localhost:9003/health 2>/dev/null | grep -q ok; do \
		timeout=$$((timeout - 2)); \
		[ $$timeout -le 0 ] && { echo " timed out"; docker compose -f docker-compose.integration.yml down; exit 1; }; \
		printf '.'; sleep 2; \
	done; echo " ready"
	go test -tags integration -v -count=1 -timeout 120s ./src/integration/...; \
	EXIT=$$?; \
	docker compose -f docker-compose.integration.yml down; \
	exit $$EXIT

tidy:
	go mod tidy

# ── OpenAPI ──────────────────────────────────────────────────────────────────
openapi:
	go install github.com/swaggo/swag/cmd/swag@latest
	swag init -g main.go -d cmd/server,src/handlers,src/models -o docs --outputTypes go,json

# ── Genkit ───────────────────────────────────────────────────────────────────
genkit: web/dist
	npx genkit start -- go run ./cmd/server

# ── React ────────────────────────────────────────────────────────────────────
web/node_modules:
	cd web && npm install

web/dist: web/node_modules
	cd web && npm run build

web-dev: web/node_modules
	cd web && npm run dev

# ── Docker ───────────────────────────────────────────────────────────────────
docker:
	docker build -t content-control-center .

docker-genkit:
	docker compose -f docker-compose.genkit.yml up --build

# ── Cleanup ──────────────────────────────────────────────────────────────────
clean:
	rm -f server
	rm -f seed
	# rm -rf data/*
	rm -rf web/dist web/node_modules
	rm -f coverage.out
