# ─── Stage 1: build React ────────────────────────────────────────────────────
FROM node:20-alpine AS web-builder

WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci

COPY web/ ./
RUN npm run build

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

# ─── Stage 3: certs + tz data ────────────────────────────────────────────────
# Pull only the two files we need from a throwaway Alpine layer.
FROM alpine:3.20 AS certs
RUN apk add --no-cache ca-certificates tzdata

# ─── Stage 4: scratch runtime ────────────────────────────────────────────────
FROM scratch

# TLS root certificates and timezone database.
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=certs /usr/share/zoneinfo                 /usr/share/zoneinfo

# Statically-linked binary (CGO_ENABLED=0 in the build stage).
COPY --from=go-builder /server /server

ENV ADDR=":3000"

EXPOSE 3000

ENTRYPOINT ["/server"]
