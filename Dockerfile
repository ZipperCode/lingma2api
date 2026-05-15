# =============================================================================
# Stage 1: Build
# =============================================================================
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache nodejs npm

WORKDIR /src

# Cache Go module dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy all source files
# Note: frontend/node_modules/, frontend-dist/ are excluded via .dockerignore
COPY . .

# Build frontend
WORKDIR /src/frontend
RUN npm ci && npm run build

# Build Go binary (CGO disabled for pure-Go SQLite compatibility on alpine)
WORKDIR /src
RUN CGO_ENABLED=0 go build -o /app/lingma2api .

# =============================================================================
# Stage 2: Runtime (minimal alpine)
# =============================================================================
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget

# Create non-root user
RUN adduser -D -u 1000 appuser

WORKDIR /app

COPY --from=builder /app/lingma2api /app/lingma2api
COPY docker-entrypoint.sh /app/docker-entrypoint.sh

RUN chmod +x /app/docker-entrypoint.sh

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/ || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/lingma2api", "-config", "/app/config/config.yaml"]
