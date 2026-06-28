# syntax=docker/dockerfile:1
# The React SPA lives in its own repo (ogen-app/ui) and deploys separately
# (CON-98). This image builds the API only.
# ─── Stage 1: build Go binary ────────────────────────────────────────────────
FROM golang:1.26-alpine AS go-builder

WORKDIR /app

# Download Go modules first — this layer is cached as long as go.mod/go.sum are
# unchanged. (No BuildKit cache mounts below: Railway's builder requires cache-
# mount ids to be prefixed with a hardcoded `s/<service-id>-`, which would couple
# this Dockerfile to a single Railway service and break local / CI / prod builds.)
COPY go.mod go.sum ./
RUN go mod download

# Copy source.
COPY . .

# Build a static binary.
RUN CGO_ENABLED=0 GOOS=linux go build -p 4 -trimpath -ldflags="-s -w" -o /server ./cmd/server

# ─── Stage 2: Alpine runtime with poppler-utils ──────────────────────────────
# Alpine is used (not scratch) so the server can exec `pdftotext` and
# `pdftoppm` from poppler-utils — required by the PDF asset ingestion path
# (text extraction fallback + thumbnail rendering).
FROM alpine:3.20

# su-exec lets the entrypoint drop from root to appuser after fixing volume perms.
RUN apk add --no-cache ca-certificates tzdata poppler-utils su-exec && \
    addgroup -S appgroup && adduser -S -G appgroup appuser && \
    mkdir -p /var/lib/ogen/keys && \
    chown appuser:appgroup /var/lib/ogen/keys && \
    chmod 700 /var/lib/ogen/keys

# Statically-linked Go binary (CGO_ENABLED=0 in the build stage).
COPY --from=go-builder /server /server
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# The container starts as root so the entrypoint can chown the mounted KEK
# volume (Railway/Docker mount it as root, shadowing the build-time chown); the
# entrypoint then drops to the unprivileged appuser via su-exec. No `USER` here.
ENV ADDR=":3000"

EXPOSE 3000

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
