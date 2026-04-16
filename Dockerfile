# syntax=docker/dockerfile:1
# ─── Stage 1: build React ────────────────────────────────────────────────────
FROM node:20-alpine AS web-builder

RUN corepack enable

WORKDIR /app/web
COPY web/package.json web/pnpm-lock.yaml ./

# Cache the pnpm content-addressable store across builds.
RUN --mount=type=cache,id=pnpm-store,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm build

# ─── Stage 2: build Go binary ────────────────────────────────────────────────
FROM golang:1.26-alpine AS go-builder

WORKDIR /app

# Download Go modules — cached as long as go.mod/go.sum are unchanged.
COPY go.mod go.sum ./
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    go mod download

# Copy source and compiled React assets.
COPY . .
COPY --from=web-builder /app/web/dist ./web/dist

# Build — the Go build cache avoids recompiling unchanged packages even when
# source files change, which is the main speedup on incremental builds.
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=go-build,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -p 4 -trimpath -ldflags="-s -w" -o /server ./cmd/server

# ─── Stage 3: Alpine runtime with poppler-utils ──────────────────────────────
# Alpine is used (not scratch) so the server can exec `pdftotext` and
# `pdftoppm` from poppler-utils — required by the PDF asset ingestion path
# (text extraction fallback + thumbnail rendering).
FROM alpine:3.20

RUN --mount=type=cache,id=apk-cache,sharing=locked,target=/var/cache/apk \
    apk add --no-cache ca-certificates tzdata poppler-utils && \
    addgroup -S appgroup && adduser -S -G appgroup appuser && \
    mkdir -p /data && chown appuser:appgroup /data

# Statically-linked Go binary (CGO_ENABLED=0 in the build stage).
COPY --from=go-builder /server /server

USER appuser

ENV ADDR=":3000"

EXPOSE 3000

ENTRYPOINT ["/server"]
