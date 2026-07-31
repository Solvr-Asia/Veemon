# syntax=docker/dockerfile:1

# Build stage
#
# Built from the REPO ROOT (see docker-compose.yml `context: .`), not
# apps/api, because apps/api/go.mod has a local `replace veemon-common =>
# ../../packages/go-common` — that sibling module must be inside the build
# context. The image preserves the same apps/api <-> packages/go-common
# relative layout as the source tree so the replace path still resolves.
FROM golang:1.25.12-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

# Cache dependencies: copy just the module manifests first (both modules —
# the local `replace` needs go-common's go.mod/go.sum to resolve the module
# graph, even though its source isn't fetched over the network).
COPY packages/go-common/go.mod packages/go-common/go.sum ./packages/go-common/
COPY apps/api/go.mod apps/api/go.sum ./apps/api/
WORKDIR /app/apps/api
RUN go mod download

# Build all three binaries (server, migrate, worker)
WORKDIR /app
COPY packages/go-common ./packages/go-common
COPY apps/api ./apps/api
WORKDIR /app/apps/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/worker ./cmd/worker

# Final stage
FROM alpine:3.23

WORKDIR /app

# CA certs for HTTPS, tzdata for time zones; create an unprivileged user.
RUN apk --no-cache add ca-certificates tzdata \
 && addgroup -S app && adduser -S -G app app

# Binaries and SQL migrations (golang-migrate is the source of truth).
COPY --from=builder /out/server /out/migrate /out/worker /app/
COPY apps/api/migrations ./migrations

EXPOSE 3000 50051

USER app

# Liveness against the HTTP health endpoint (busybox wget ships with alpine).
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q -O - http://127.0.0.1:3000/health >/dev/null 2>&1 || exit 1

# Default to the API server; override the command to run ./migrate or ./worker.
CMD ["/app/server"]
