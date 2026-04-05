.PHONY: build run test web web-dev tidy docker clean

# ── Go ────────────────────────────────────────────────────────────────────────
build: web/dist
	go build -o server ./cmd/server

run: web/dist
	go run ./cmd/server

test:
	go test ./... -v

tidy:
	go mod tidy

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
