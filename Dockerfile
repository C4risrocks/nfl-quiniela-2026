# Build Stage
FROM golang:1.26-alpine AS builder

# Install build dependencies, CA certificates and timezone data
RUN apk add --no-cache ca-certificates tzdata git

WORKDIR /build

# Copy dependency files first to leverage Docker layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build single static binary with stripped debug symbols
# GOMAXPROCS=1 and -p 1 force single-threaded compilation, reducing peak memory usage from ~2.5GB to <400MB
# to prevent Out-Of-Memory (OOM) crashes on VPS instances with 1GB-2GB RAM when compiling modernc.org/sqlite.
ENV GOMAXPROCS=1
RUN CGO_ENABLED=0 GOOS=linux go build -p 1 -ldflags="-s -w" -o /build/server .

# Production Runtime Stage
FROM alpine:3.21

# Install CA certificates, timezone data, wget and su-exec for safe non-root execution on mounted volumes
RUN apk add --no-cache ca-certificates tzdata wget su-exec \
    && addgroup -g 10001 appgroup \
    && adduser -u 10001 -G appgroup -D -h /app appuser

WORKDIR /app

# Create persistent storage directory for SQLite database and set initial permissions
RUN mkdir -p /app/data && chown -R appuser:appgroup /app

# Copy binary from builder and entrypoint script
COPY --from=builder --chown=appuser:appgroup /build/server /app/server
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Declare persistent volume mount point
VOLUME ["/app/data"]

# Expose default HTTP application port
EXPOSE 8080

# Environment Defaults (Override via Dokploy environment variables)
ENV PORT=8080 \
    DB_TYPE=sqlite \
    DB_PATH=/app/data/quiniela.db \
    ENABLE_BACKGROUND_SYNC=true \
    ESPN_SYNC_INTERVAL_MINS=5 \
    CURRENT_SEASON_YEAR=2026

# Healthcheck for Dokploy container monitoring
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q -O - http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["/app/server"]
