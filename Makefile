.PHONY: build run test coverage _ginkgo _air web web-dev tidy docker clean openapi

GINKGO_FLAGS = --github-output -r -randomize-all -randomize-suites -race -trace -procs=2 -poll-progress-after=10s -poll-progress-interval=10s

# ── Go ────────────────────────────────────────────────────────────────────────
build: web/dist
	go build -o server ./cmd/server

run: web/dist _air
	air

_air:
	go install github.com/air-verse/air@latest

_ginkgo:
	go install github.com/onsi/ginkgo/v2/ginkgo

test: _ginkgo
	ginkgo $(GINKGO_FLAGS) --cover --coverpkg=./... --coverprofile=coverage.out --covermode=atomic --output-dir=. ./...
	go tool cover -func=coverage.out

tidy:
	go mod tidy

# ── OpenAPI ──────────────────────────────────────────────────────────────────
openapi:
	go install github.com/swaggo/swag/cmd/swag@latest
	swag init -g main.go -d cmd/server,src/handlers,src/models -o docs --outputTypes go,json

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

# ── Cleanup ──────────────────────────────────────────────────────────────────
clean:
	rm -f server
	rm -f data.db data.db-shm data.db-wal
	rm -rf web/dist web/node_modules
	rm -f coverage.out
