# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Cache dependency downloads separately from source compilation
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a statically-linked binary
COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -trimpath \
      -ldflags="-s -w -X main.Version=${VERSION}" \
      -o /bin/qbee-engine \
      ./cmd/engine


# ── Runtime stage ─────────────────────────────────────────────────────────────
# Use Debian slim so we can install redis-cli, pg_dump, and mongodump.
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        tzdata \
        curl \
        gnupg \
        redis-tools \
        postgresql-client \
    && curl -fsSL https://www.mongodb.org/static/pgp/server-7.0.asc \
        | gpg --dearmor -o /etc/apt/trusted.gpg.d/mongodb.gpg \
    && echo "deb [ arch=amd64,arm64 ] https://repo.mongodb.org/apt/debian bookworm/mongodb-org/7.0 main" \
        > /etc/apt/sources.list.d/mongodb-org-7.0.list \
    && apt-get update && apt-get install -y --no-install-recommends mongodb-org-tools \
    && apt-get remove -y curl gnupg && apt-get autoremove -y \
    && rm -rf /var/lib/apt/lists/*

# Copy the statically-linked binary.
COPY --from=builder /bin/qbee-engine /usr/local/bin/qbee-engine

# Default environment
ENV HTTP_ADDR=":8080" \
    LOG_LEVEL="info" \
    CONFIG_FILE="/etc/qbee/config.yaml"

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD curl -sf http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/qbee-engine"]
