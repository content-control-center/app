.PHONY: build run test coverage _ginkgo web web-dev tidy docker clean openapi

GINKGO_FLAGS = --github-output -r -randomize-all -randomize-suites -race -trace -procs=2 -poll-progress-after=10s -poll-progress-interval=10s

# ── Go ────────────────────────────────────────────────────────────────────────
build: web/dist
	go build -o server ./cmd/server

run: web/dist
	go run ./cmd/server

_ginkgo:
	go install github.com/onsi/ginkgo/v2/ginkgo

test: _ginkgo
	ginkgo $(GINKGO_FLAGS) --cover --coverpkg=./... --coverprofile=coverage.out --covermode=atomic --output-dir=. ./...
	go tool cover -func=coverage.out

coverage: _ginkgo test

tidy:
	go mod tidy

# ── OpenAPI ──────────────────────────────────────────────────────────────────
openapi:
	go install github.com/swaggo/swag/cmd/swag@latest
	swag init -g cmd/server/main.go -o docs --outputTypes go,json

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
