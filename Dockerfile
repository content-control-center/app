# ─── Stage 1: build React ────────────────────────────────────────────────────
FROM node:20-alpine AS web-builder

RUN corepack enable

WORKDIR /app/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm build

# ─── Stage 2: build Go binary ────────────────────────────────────────────────
FROM golang:1.26-alpine AS go-builder

WORKDIR /app

# Download dependencies first (cache-friendly layer).
COPY go.mod go.sum ./
RUN go mod download

# Copy source.
COPY . .

# Copy the compiled React assets so the embed compiles correctly.
COPY --from=web-builder /app/web/dist ./web/dist

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

# ─── Stage 3: certs + tz data + non-root user ────────────────────────────────
# Pull only the files we need from a throwaway Alpine layer.
FROM alpine:3.20 AS certs
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S appgroup && adduser -S -G appgroup appuser

# ─── Stage 4: scratch runtime ────────────────────────────────────────────────
FROM scratch

# TLS root certificates and timezone database.
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=certs /usr/share/zoneinfo                 /usr/share/zoneinfo

# Non-root user created in the certs stage.
COPY --from=certs /etc/passwd /etc/passwd
COPY --from=certs /etc/group  /etc/group

# Statically-linked binary (CGO_ENABLED=0 in the build stage).
COPY --from=go-builder /server /server

USER appuser

ENV ADDR=":3000"

EXPOSE 3000

ENTRYPOINT ["/server"]
