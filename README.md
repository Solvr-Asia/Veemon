# Veemon

A production-ready **polyglot monorepo** starter kit. A Go Fiber + gRPC backend,
a React desktop app, and a LangGraph.js AI service, all sharing one proto contract.

## Monorepo

```
apps/api/               Go backend — Fiber REST + gRPC (Domain-Driven Design, Clean Architecture)
apps/web/               React 19 + TanStack Router + Vite + Tauri (desktop)
apps/ai/                LangGraph.js agent + workflow service (TypeScript, Hono)
packages/api-client/    Generated protobuf-es types + typed REST client (@veemon/api-client)
packages/tsconfig/      Shared base tsconfig (@veemon/tsconfig)
packages/go-common/     Shared Go infra module — logging/, monitoring/, infra/
                        (pulled into apps/api via a go.mod `replace`)
contract/               Proto contract — the single source of truth (Go + TypeScript)
CLAUDE.md / AGENTS.md   AI-assistant guidance (indexes into .claude/)
.claude/                AI config: settings.json (shared) + rules/, agents/, skills/
```

The `contract/` proto generates Go (into `apps/api`) **and** TypeScript (into
`packages/api-client`). `web` and `ai` call the existing Fiber REST routes
through the typed `@veemon/api-client` — the Go server is untouched.

### Quickstart

```bash
# Prerequisites: Go 1.25+, Bun 1.3+, buf (go install github.com/bufbuild/buf/cmd/buf@latest)
bun install                 # install all TS workspaces
bun run proto               # regenerate Go + TS clients from contract/*.proto
bun run dev                 # run api (:3000) + web (:5173) + ai (:4111) together

# Or per app:
bun run dev:api             # Go server (make -C apps/api dev)
bun run dev:web             # Vite dev server (browser)
bun run dev:ai              # LangGraph service (Hono, :4111)

bun run build               # build all workspaces
bun run test                # go test -race (api) + bun tests (ts)
```

**Prerequisites for specific apps:** the `web` desktop build (Tauri) needs a
**Rust toolchain**; the browser dev flow does not. Running the `ai` agent needs a
**model provider API key**. See `apps/web/README.md` and `apps/ai/README.md`.

Everything below documents the **Go backend** (`apps/api`); run its `make`
targets from that directory (or via the root `bun run *:api` scripts).

## Features

- **Dual Protocol**: REST (Fiber) + gRPC in single application
- **Clean Architecture**: Clear separation of concerns with DDD
- **Dependency Injection**: uber-go/fx composition root — `cmd/server`/`cmd/worker` are thin `fx.New(...).Run()` entry points (see `config/fx_*.go`)
- **Database**: PostgreSQL with GORM ORM + OpenTelemetry tracing
- **Caching**: Redis with Redigo connection pooling
- **Message Queue**: RabbitMQ for async processing
- **Logging**: Zap structured logging with trace context
- **Tracing**: OpenTelemetry tracing with OTLP exporter (`noop` | `stdout` | `otlp`)
- **Configuration**: Viper with `.env`, process env, and optional Infisical secret loading
- **Validation**: go-playground/validator with custom validators
- **Authentication**: PASETO v4 local tokens with role-based access control (RBAC)
- **Testing**: Testify with mocking support
- **Migrations**: golang-migrate with SQL files support
- **Resilience**: Circuit breaker, retry, timeout with failsafe-go
- **Rate Limiting**: Request throttling with configurable limits
- **Metrics**: Prometheus metrics with `/metrics` endpoint
- **API Documentation**: OpenAPI 3 spec served through the Scalar API reference UI
- **CI/CD**: GitHub Actions for build, docker, and release

## Tech Stack

The stack spans the whole monorepo — a Go backend, a React + Tauri desktop app,
a LangGraph.js AI service, and the shared proto contract + tooling that binds
them.

### Backend — `apps/api` (Go)

| Component | Technology |
|-----------|------------|
| Language | [Go 1.25](https://go.dev/) |
| Web Framework | [Go Fiber v3](https://gofiber.io/) |
| gRPC | [google.golang.org/grpc](https://grpc.io/) |
| ORM | [GORM](https://gorm.io/) |
| Database | PostgreSQL |
| Cache | Redis ([Redigo](https://github.com/gomodule/redigo)) |
| Message Queue | RabbitMQ ([amqp091-go](https://github.com/rabbitmq/amqp091-go)) |
| Logger | [Zap](https://github.com/uber-go/zap) |
| Tracing | [OpenTelemetry](https://opentelemetry.io/) |
| Config | [Viper](https://github.com/spf13/viper) + [Infisical](https://infisical.com/) |
| Validation | [go-playground/validator](https://github.com/go-playground/validator) |
| Testing | [Testify](https://github.com/stretchr/testify) |
| Auth | [PASETO](https://paseto.io/) |
| Migrations | [golang-migrate](https://github.com/golang-migrate/migrate) |
| Resilience | [failsafe-go](https://failsafe-go.dev/) |
| Metrics | [Prometheus](https://prometheus.io/) |
| API Docs | [Scalar](https://github.com/scalar/scalar) (served by `docs/scalar.go`) |
| DI / Composition Root | [uber-go/fx](https://github.com/uber-go/fx) |
| Shared infra | `packages/go-common` (separate Go module — logging, telemetry/metrics, redis/rabbitmq/resilience/database) |

### Web — `apps/web` (`@veemon/web`)

| Component | Technology |
|-----------|------------|
| UI | [React 19](https://react.dev/) |
| Routing | [TanStack Router](https://tanstack.com/router) |
| Data fetching | [TanStack Query](https://tanstack.com/query) |
| Build tool | [Vite 6](https://vite.dev/) |
| Desktop shell | [Tauri 2](https://tauri.app/) (Rust) |
| API access | `@veemon/api-client` (typed REST over the Go API) |

### AI — `apps/ai` (`@veemon/ai`)

| Component | Technology |
|-----------|------------|
| Agent/graph runtime | [LangGraph.js](https://langchain-ai.github.io/langgraphjs/) |
| Model | [Anthropic Claude](https://www.anthropic.com/) via [`@langchain/anthropic`](https://js.langchain.com/) |
| HTTP server | [Hono](https://hono.dev/) (invoke + SSE stream) |
| Persistence | LangGraph checkpointer — in-memory or [Postgres](https://github.com/langchain-ai/langgraphjs) |
| Schemas | [Zod](https://zod.dev/) |
| API access | `@veemon/api-client` (typed REST over the Go API) |

### Contract & tooling (shared)

| Component | Technology |
|-----------|------------|
| Proto contract | [Protocol Buffers](https://protobuf.dev/) (`contract/`, single source of truth) |
| Codegen | [buf](https://buf.build/) → `protoc-gen-go` / `-go-grpc` / `protoc-gen-fiber` (custom) / [protobuf-es](https://github.com/bufbuild/protobuf-es) |
| Monorepo | [Bun](https://bun.sh/) workspaces + `make` (Go) |
| CI/CD | GitHub Actions (build, docker, release) |

## Project Structure

```
Veemon/                              # polyglot Bun-workspace monorepo
├── apps/
│   ├── api/                         # Go backend — Fiber REST + gRPC (DDD, Clean Architecture)
│   │   ├── cmd/
│   │   │   ├── server/              # HTTP + gRPC API server entry point
│   │   │   ├── worker/              # RabbitMQ consumer entry point
│   │   │   ├── migrate/             # Migration + seeding CLI tool
│   │   │   └── protoc-gen-fiber/    # buf plugin: generates Fiber routes from proto
│   │   ├── config/                  # Viper config (+ Validate), fx composition root, infra init
│   │   │   └── fx_*.go              # fx.Module wiring (CommonModule, ServerModule, WorkerModule)
│   │   ├── handler/                 # Presentation layer (gRPC impl + generated Fiber routes)
│   │   │   ├── grpc/veemon/           # Generated veemon.route option types
│   │   │   ├── grpc/user/           # Generated protobuf / gRPC / Fiber routes
│   │   │   └── user_handler.go      # Shared gRPC/HTTP handler implementation
│   │   ├── app/usecase/             # Business logic layer
│   │   ├── repository/              # Data access layer (GORM)
│   │   ├── entity/                  # Domain entities
│   │   ├── pkg/                     # App-specific shared code: token, authguard, middleware,
│   │   │                            #   response, errors, validation (auth/presentation-coupled;
│   │   │                            #   see packages/go-common/ below for generic shared infra)
│   │   ├── migrations/              # golang-migrate SQL files (schema source of truth)
│   │   ├── database/                # Migration helper + seeders
│   │   ├── examples/                # Runnable usage examples (PASETO auth flow)
│   │   ├── docs/scalar.go           # OpenAPI spec + Scalar UI
│   │   ├── Dockerfile               # Multi-stage, non-root (ships server/migrate/worker)
│   │   ├── Makefile                 # Backend build / test / migrate commands
│   │   ├── .env.example             # Environment template (authoritative config list)
│   │   ├── .golangci.yml            # Linter config (golangci-lint v2)
│   │   ├── .goreleaser.yml          # Release config
│   │   └── go.mod · go.sum
│   ├── web/                         # React 19 + TanStack Router + Vite + Tauri (@veemon/web)
│   │   ├── src/                     # App: routes/, lib/, main.tsx, routeTree.gen.ts
│   │   ├── src-tauri/               # Rust desktop shell (Cargo.toml, tauri.conf.json)
│   │   ├── index.html · vite.config.ts · tsconfig.json
│   │   └── README.md
│   └── ai/                          # LangGraph.js agent + workflow service (@veemon/ai)
│       ├── src/                     # domain/, application/, infrastructure/, interface/http, composition.ts
│       └── README.md
├── packages/
│   ├── api-client/                  # @veemon/api-client — generated protobuf-es + typed REST client
│   │   └── src/gen/                 # Generated TS from contract/ (veemon, user)
│   ├── tsconfig/                    # @veemon/tsconfig — shared base tsconfig
│   └── go-common/                   # Shared Go infra module (not a Bun workspace member) —
│       ├── logging/                 #   structured Zap logger
│       ├── monitoring/              #   telemetry/ (OTel) + metrics/ (Prometheus)
│       ├── infra/                   #   redis/, rabbitmq/, resilience/, database/
│       └── go.mod · go.sum          #   pulled into apps/api via a `replace`
├── contract/                        # Proto contract — single source of truth (Go + TS)
│   ├── veemon/annotations.proto       # Custom veemon.route option (method/path/auth/rate limit)
│   └── user/user.proto              # UserApi service with veemon.route annotations
├── docs/superpowers/                # Design specs & plans (specs/, plans/)
├── .github/workflows/               # CI/CD: ci.yml (lint/test/vuln/build/docker), release.yml
├── .claude/                         # AI config
│   ├── settings.json                # Shared permissions (committed)
│   ├── settings.local.json          # Personal overrides (git-ignored, not pushed)
│   ├── rules/                       # Topical coding standards & conventions
│   ├── agents/                      # Subagent definitions (e.g. code-reviewer)
│   └── skills/                      # Reusable skills (e.g. proto-routes)
├── buf.yaml · buf.gen.yaml          # buf module + codegen (Go → apps/api, TS → packages/api-client)
├── package.json · bun.lock          # Bun workspaces + root scripts (proto, dev, build, test)
├── docker-compose.yml               # Infrastructure services + one-shot migrate
├── CLAUDE.md                        # AI guidance index → .claude/rules, agents, skills
├── AGENTS.md                        # Same guidance, for AGENTS.md-aware tools
├── CHANGELOG.md                     # Change log
├── LICENSE                          # MIT License
└── README.md
```

Paths inside `apps/api/` (e.g. `cmd/`, `config/`, `pkg/`, `handler/`) are relative
to that directory throughout this README; the backend is a self-contained Go
module orchestrated by its own `Makefile`.

### AI-assistant docs

Guidance for AI coding assistants is split into small, focused files under
[`.claude/`](.claude/). [`CLAUDE.md`](CLAUDE.md) and [`AGENTS.md`](AGENTS.md) are
thin indexes that link to [`.claude/rules/`](.claude/rules) (coding standards,
architecture, security, goroutines, testing, …), [`.claude/agents/`](.claude/agents),
and [`.claude/skills/`](.claude/skills). `.claude/settings.json` (shared
permissions) is committed; `.claude/settings.local.json` (personal overrides) is
git-ignored.

## Quick Start

### Prerequisites

- Go 1.25.12+ (CI and Docker use Go 1.25.12)
- Docker & Docker Compose
- Make

### 1. Clone and Setup

```bash
git clone https://github.com/yourusername/veemon.git
cd veemon

# Copy environment file
cp .env.example .env
```

### 2. Start Infrastructure

```bash
# Start Postgres, Redis, RabbitMQ, Jaeger — plus a one-shot `migrate` step that
# applies SQL migrations, then the app container itself.
make compose-up
```

`docker compose` runs the `migrate` service to completion before starting `app`,
so a compose bring-up is fully migrated and ready.

### 3. Run Application (without Docker)

```bash
# Apply migrations first (golang-migrate is the source of truth).
make migrate

# API server (HTTP + gRPC). Run directly…
make run

# …or build and run (Makefile builds bin/veemon)
make build
./bin/veemon

# Background worker (RabbitMQ consumer) — separate process
make run-worker
```

> **`JWT_SECRET` is required** — the server refuses to start without a strong
> value (no default is shipped). Generate one with `openssl rand -hex 32`. It
> must be a 64-character hex string or at least 32 raw bytes. See
> [Configuration](#configuration).

### 4. Test Endpoints

```bash
# Health check
curl http://localhost:3000/health

# Register user
curl -X POST http://localhost:3000/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123","name":"Test User"}'

# Login
curl -X POST http://localhost:3000/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}'

# Get current user (with token)
curl http://localhost:3000/api/v1/auth/me \
  -H "Authorization: Bearer <your-token>"
```

## Configuration

### Environment Variables

Copy `.env.example` to `.env` — **it is the authoritative, fully-documented list**
of every key with its default and units. Config is read from `.env`, then process
environment variables (which override), with optional Infisical loading. The
essentials:

```env
# Application
SERVICE_NAME=veemon
ENVIRONMENT=development           # development | production
HTTP_PORT=3000
GRPC_PORT=50051

# Database (golang-migrate is the source of truth; AutoMigrate is opt-in)
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=veemon_db
DB_SSL_MODE=disable               # use "require" or stricter in production
DB_AUTO_MIGRATE=false             # dev-only convenience; leave off in prod

# Redis (used for login lockout + token revocation)
REDIS_HOST=localhost
REDIS_PORT=6379

# RabbitMQ
RABBITMQ_HOST=localhost
RABBITMQ_PORT=5672
RABBITMQ_USER=guest
RABBITMQ_PASSWORD=guest

# Token signing — REQUIRED, no default. Must be a 64-char hex string
# (openssl rand -hex 32) or at least 32 raw bytes. The server refuses to boot
# with an empty, placeholder, or weak value.
JWT_SECRET=<run: openssl rand -hex 32>
JWT_EXPIRATION=24                 # hours

# Telemetry / Logging
OTEL_EXPORTER_TYPE=noop           # noop | stdout | otlp
OTEL_SAMPLE_RATIO=1.0             # 0.0-1.0 parent-based ratio sampler
LOG_LEVEL=info                    # debug | info | warn | error
LOG_FORMAT=json                   # json | console
```

**Hardening knobs** (all in `.env.example`, sensible defaults if unset):

| Group | Keys |
|-------|------|
| HTTP | `PREFORK` (must be `false` — unsupported with the embedded gRPC server), `HTTP_READ_TIMEOUT`, `HTTP_WRITE_TIMEOUT`, `HTTP_IDLE_TIMEOUT`, `REQUEST_TIMEOUT` (per-request deadline, seconds) |
| DB pool | `DB_MAX_IDLE_CONNS`, `DB_MAX_OPEN_CONNS`, `DB_CONN_MAX_LIFETIME` (minutes), `DB_PREPARE_STMT`, `DB_SKIP_DEFAULT_TRANSACTION` |
| Redis | `REDIS_PASSWORD`, `REDIS_DB`, `REDIS_MAX_IDLE`, `REDIS_MAX_ACTIVE`, `REDIS_IDLE_TIMEOUT`, `REDIS_DIAL_TIMEOUT`, `REDIS_READ_TIMEOUT`, `REDIS_WRITE_TIMEOUT` |
| Login protection | `LOGIN_MAX_ATTEMPTS`, `LOGIN_LOCKOUT_MINUTES` |
| Security | `CORS_ORIGINS` (must not be `*` in production), `METRICS_AUTH_TOKEN` (gate `/metrics`) |

> Two startup guards fail fast: `PREFORK=true` and `CORS_ORIGINS=*` in
> `production` both prevent the server from booting.

### Infisical

The app supports Infisical in two ways:

1. Use the Infisical CLI to inject secrets into the process:

```bash
infisical init
make infisical-run INFISICAL_ENV=dev INFISICAL_PATH=/
make infisical-run-worker INFISICAL_ENV=dev INFISICAL_PATH=/
```

2. Let the Go app fetch secrets at startup with the official Infisical Go SDK:

```env
INFISICAL_ENABLED=true
INFISICAL_CLIENT_ID=<machine-identity-client-id>
INFISICAL_CLIENT_SECRET=<machine-identity-client-secret>
INFISICAL_PROJECT_ID=<project-id>
INFISICAL_ENVIRONMENT=dev
INFISICAL_SECRET_PATH=/
INFISICAL_INCLUDE_IMPORTS=true
INFISICAL_RECURSIVE=false
INFISICAL_EXPAND_SECRET_REFERENCES=true
INFISICAL_OVERRIDE=false
```

When `INFISICAL_ENABLED=true`, secrets are fetched before the final config decode. Existing process environment variables keep precedence by default; set `INFISICAL_OVERRIDE=true` if Infisical should overwrite them.

## API Endpoints

### Authentication

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| POST | `/api/v1/auth/register` | No | Register new user |
| POST | `/api/v1/auth/login` | No | Login user |
| POST | `/api/v1/auth/refresh` | Yes | Refresh access token |
| GET | `/api/v1/auth/me` | Yes | Get current user profile |
| POST | `/api/v1/auth/logout` | Yes | Logout current session |

### User

| Method | Endpoint | Auth | Roles | Description |
|--------|----------|------|-------|-------------|
| GET | `/api/v1/users` | Yes | admin, superadmin | List all users |
| GET | `/api/v1/users/:id` | Yes | admin, superadmin | Get user by ID |
| PUT | `/api/v1/users/:id` | Yes | admin, superadmin | Update user |
| DELETE | `/api/v1/users/:id` | Yes | admin, superadmin | Soft-delete user |

### Health & Ops

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Liveness — shallow, always `200` if the process is up (no dependency checks) |
| GET | `/ready` | Readiness — pings Postgres, Redis, and RabbitMQ; `503` if any is unhealthy |
| GET | `/metrics` | Prometheus metrics (open by default; requires `Authorization: Bearer <token>` when `METRICS_AUTH_TOKEN` is set) |
| GET | `/docs/openapi.json` | OpenAPI JSON |
| GET | `/docs/` | Scalar API docs |

### Authentication behavior

- **Tokens** are PASETO v4 local, carrying a revocable `jti`. `JWT_EXPIRATION` sets the lifetime (hours).
- **Login** rejects non-`active` accounts (`403`) and is gated by a per-account lockout (`429`) after `LOGIN_MAX_ATTEMPTS` failures for `LOGIN_LOCKOUT_MINUTES` (Redis-backed).
- **Logout** revokes the presented token immediately (it can't be reused before expiry).
- **Refresh** reloads the user (so role/status changes take effect), rotates the token, and revokes the old one. It requires a still-valid token — it cannot refresh an already-expired one.
- **Authorization** is fail-closed: a route/RPC with no explicit policy is denied (a missing policy panics at startup rather than silently exposing an endpoint).

## gRPC Services

```protobuf
service UserApi {
    rpc Register(RegisterReq) returns (RegisterRes);
    rpc Login(LoginReq) returns (LoginRes);
    rpc RefreshToken(RefreshTokenReq) returns (RefreshTokenRes);
    rpc GetMe(google.protobuf.Empty) returns (UserProfile);
    rpc Logout(google.protobuf.Empty) returns (LogoutRes);
    rpc ListUsers(ListUsersReq) returns (ListUsersRes);
    rpc GetUser(GetUserReq) returns (UserProfile);
    rpc UpdateUser(UpdateUserReq) returns (UserProfile);
    rpc DeleteUser(DeleteUserReq) returns (DeleteUserRes);
}
```

Connect via gRPC at `localhost:50051`.

## Response Format

REST payloads are serialized with `protojson`, so `data` field names use
**camelCase** (matching the proto JSON mapping, e.g. `createdAt`, `companyCode`).

### Success Response

```json
{
    "success": true,
    "data": {
        "id": "uuid",
        "email": "test@example.com",
        "name": "Test User"
    }
}
```

### Success with Pagination

```json
{
    "success": true,
    "data": [...],
    "meta": {
        "page": 1,
        "size": 10,
        "total": 100,
        "totalPages": 10
    }
}
```

### Error Response

```json
{
    "success": false,
    "error": {
        "code": 40001,
        "message": "validation failed: email is required"
    }
}
```

## Validation Rules

Built-in validators:

| Tag | Description | Example |
|-----|-------------|---------|
| `required` | Field must not be empty | `validate:"required"` |
| `email` | Valid email format | `validate:"email"` |
| `min=N` | Minimum length/value | `validate:"min=8"` |
| `max=N` | Maximum length/value | `validate:"max=100"` |
| `oneof=a b` | Value must be one of | `validate:"oneof=asc desc"` |
| `gte=N` | Greater than or equal | `validate:"gte=1"` |
| `lte=N` | Less than or equal | `validate:"lte=100"` |

Custom validators:

| Tag | Description |
|-----|-------------|
| `phone` | Valid phone number |
| `password` | Min 8 chars, upper, lower, digit |
| `nik` | Indonesian NIK (16 digits) |

## Database Migrations

This project uses [golang-migrate](https://github.com/golang-migrate/migrate) for
database migrations with SQL files. **golang-migrate is the single source of
truth.** GORM `AutoMigrate` is opt-in via `DB_AUTO_MIGRATE=true` (default `false`)
and is a local-dev convenience only — production schema changes always go through
reviewed SQL migrations. Run `make migrate` (or `./bin/...-migrate up`) before
starting the server; `docker compose` does this automatically via its `migrate`
service.

### Migration Commands

```bash
# Run all pending migrations
make migrate

# Rollback all migrations
make migrate-down

# Rollback last migration only
make migrate-rollback

# Show current migration version
make migrate-status

# Create new migration (creates up and down files)
make migrate-create name=create_orders_table

# Run database seeders
make seed

# Drop all tables and re-run migrations
make fresh

# Drop all, migrate, and seed
make fresh-seed

# Rollback all and re-run migrations
make refresh

# Rollback all, migrate, and seed
make refresh-seed

# Reset database (rollback all)
make reset
```

### Migration File Format

Migration files are stored in `migrations/` directory:

```
migrations/
├── 000001_create_users_table.up.sql       # Creates users table
└── 000001_create_users_table.down.sql     # Drops users table
```

### Creating New Migrations

```bash
# Create a new migration
make migrate-create name=add_orders_table

# This creates (next number after the existing 000001):
# - migrations/000002_add_orders_table.up.sql
# - migrations/000002_add_orders_table.down.sql
```

### Seeding Data

The seeder creates sample data for development:

```bash
# Run all seeders
make seed
```

Default seed data:
- **superadmin@example.com** (password: `SuperAdmin123!`) - Roles: superadmin, admin
- **admin@example.com** (password: `Admin123!`) - Roles: admin
- **employee1@example.com** (password: `Employee123!`) - Roles: employee
- **employee2@example.com** (password: `Employee123!`) - Roles: employee
- **user@example.com** (password: `User123!`) - Roles: user

## Testing

```bash
# Run all tests
make test

# Run with race detection
go test -race ./...

# Run with coverage
go test -cover -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Resilience Patterns

This boilerplate uses [failsafe-go](https://failsafe-go.dev/) for resilience patterns:

### Circuit Breaker

Prevents cascading failures by stopping requests to failing services:

```go
import "veemon-common/infra/resilience"

// Create executor with circuit breaker
executor := resilience.New[*http.Response](
    "payment-service",
    resilience.DefaultConfig(),
    resilience.WithLogger[*http.Response](logger),
)

// Execute with resilience
response, err := executor.Execute(ctx, func(ctx context.Context) (*http.Response, error) {
    return httpClient.Do(request)
})
```

### Resilient HTTP Client

Pre-built HTTP client with circuit breaker, retry, and timeout:

```go
client := resilience.NewHTTPClient("external-api", resilience.DefaultHTTPClientConfig(), logger)

// Automatic retry, circuit breaker, and timeout
resp, err := client.Get(ctx, "https://api.example.com/data")
```

### Configuration

```go
cfg := resilience.Config{
    CBFailureThreshold: 5,           // Failures before opening circuit
    CBSuccessThreshold: 3,           // Successes before closing circuit
    CBDelay:            30 * time.Second, // Wait time in open state
    RetryMaxAttempts:   3,           // Max retry attempts
    RetryDelay:         100 * time.Millisecond,
    RetryMaxDelay:      2 * time.Second,
    Timeout:            10 * time.Second,
}
```

## Rate Limiting

**What ships enabled:** a global per-IP limiter (100 req/min) on all routes, a
stricter per-IP limiter (10 req/min) on the unauthenticated auth endpoints
(`/auth/login`, `/auth/register`), and a Redis-backed per-account **login
lockout** (`LOGIN_MAX_ATTEMPTS` / `LOGIN_LOCKOUT_MINUTES`). Health/metrics
endpoints are skipped. The middleware below is the reusable library for adding
more:

```go
import "veemon/pkg/middleware"

// Default: 100 requests per minute per IP
app.Use(middleware.RateLimitMiddleware(middleware.DefaultRateLimitConfig()))

// Custom configuration
app.Use(middleware.RateLimitMiddleware(middleware.RateLimitConfig{
    Max:      50,
    Duration: 1 * time.Minute,
    KeyGenerator: func(c *fiber.Ctx) string {
        return c.IP()
    },
}))

// User-based rate limiting (after auth)
app.Use(middleware.UserRateLimiter(config))

// API key-based rate limiting
app.Use(middleware.APIKeyRateLimiter(config, "X-API-Key"))
```

## Security (OWASP Top 10)

This boilerplate implements security best practices following the [OWASP Top 10](https://owasp.org/www-project-top-ten/):

Legend: ✅ implemented and wired · 📘 pattern documented in `CLAUDE.md` (implement per your domain).

| OWASP Risk | Status | Implementation |
|------------|--------|----------------|
| **A01: Broken Access Control** | ✅ | Fail-closed RBAC on every route/RPC (missing policy → deny), resource-ownership pattern 📘 |
| **A02: Cryptographic Failures** | ✅ | bcrypt hashing; PASETO v4 with a startup-enforced strong secret (weak/placeholder rejected); `DB_SSL_MODE` configurable |
| **A03: Injection** | ✅ | Parameterized GORM queries, go-playground/validator, ORDER BY column whitelist |
| **A04: Insecure Design** | ✅ | Global + per-auth-route rate limiting, Redis-backed account lockout, secure defaults |
| **A05: Security Misconfiguration** | ✅ | Env-based config, CORS wildcard blocked in production, helmet security headers, gRPC reflection off in prod |
| **A06: Vulnerable Components** | ✅ | CI runs tests (`-race`), `golangci-lint`, and `govulncheck` |
| **A07: Auth Failures** | ✅ | Password complexity policy, token expiry, failed-login lockout, token revocation (logout/refresh rotation) |
| **A08: Data Integrity** | ✅/📘 | Request validation ✅; payload signature/checksum verification 📘 |
| **A09: Logging Failures** | ✅ | Structured logging with request-id/trace correlation; 5xx causes logged server-side, never leaked to clients |
| **A10: SSRF** | 📘 | Guidance in `CLAUDE.md` (URL allowlist, internal-IP blocking); no user-driven outbound surface ships in the boilerplate |

### Security Features

```go
// Role-based access control is declared per RPC via veemon.route auth options in
// the .proto and generated into user_fiber.pb.go (see "Declaring Routes in Proto").

// Input validation
type RegisterRequest struct {
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=8,password"`
    Name     string `json:"name" validate:"required,min=2,max=100"`
}

// Secure password hashing
hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

// Parameterized queries (SQL injection prevention)
db.Where("email = ?", email).First(&user)
```

### CI Checks

The CI/CD pipeline (`.github/workflows/ci.yml`) runs these jobs:

```text
- Lint   : golangci-lint (v2 config)
- Test   : go vet ./...  +  go test -race -count=1 -covermode=atomic ./...
- Vuln   : govulncheck (golang/govulncheck-action)
- Build  : go build of cmd/server, cmd/migrate, cmd/worker
- Docker : build on PRs; build + push to ghcr.io on main/master
```

Run the same checks locally:

```bash
golangci-lint run ./...
go vet ./...
go test -race ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
go build ./cmd/server ./cmd/migrate ./cmd/worker
```

See [CLAUDE.md](CLAUDE.md) for detailed security guidelines and code examples.

## Prometheus Metrics

Automatic HTTP metrics collection with a `/metrics` endpoint. It is open by
default; set `METRICS_AUTH_TOKEN` to require `Authorization: Bearer <token>`
(otherwise restrict it at the network layer). HTTP metrics use the route pattern
(not the raw path) as the label to bound cardinality.

```go
import "veemon-common/monitoring/metrics"

// Initialize metrics
m := metrics.Init("myapp")

// Add metrics middleware
app.Use(m.Middleware())

// Expose metrics endpoint
app.Get("/metrics", m.Handler())

// Record custom metrics
m.RecordUserRegistered()
m.RecordDBQuery("select", "users", duration)
m.RecordCacheHit("redis")
m.SetCircuitBreakerState("payment-service", 0) // 0=closed, 1=half-open, 2=open
```

### Available Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `http_requests_total` | Counter | Total HTTP requests |
| `http_request_duration_seconds` | Histogram | Request latency |
| `http_requests_in_flight` | Gauge | Current active requests |
| `db_queries_total` | Counter | Database queries |
| `cache_hits_total` | Counter | Cache hits |
| `circuit_breaker_state` | Gauge | Circuit breaker state |

## API Documentation

Interactive API documentation using Scalar UI:

- **Scalar UI**: http://localhost:3000/docs
- **OpenAPI JSON**: http://localhost:3000/docs/openapi.json

The server registers these routes during bootstrap.

## Docker

### Pull from GitHub Container Registry

```bash
# Pull latest version
docker pull ghcr.io/yourusername/veemon:latest

# Pull specific version (semantic versioning)
docker pull ghcr.io/yourusername/veemon:1.0.0
docker pull ghcr.io/yourusername/veemon:1.0
docker pull ghcr.io/yourusername/veemon:1
```

### Build Image Locally

```bash
make docker
```

### Run with Docker Compose

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f app

# Stop all services
docker-compose down
```

## Semantic Versioning

This project follows [Semantic Versioning](https://semver.org/) (SemVer):

- **MAJOR** version (X.0.0): Breaking changes
- **MINOR** version (0.X.0): New features (backward compatible)
- **PATCH** version (0.0.X): Bug fixes (backward compatible)

### Creating a Release

```bash
# Create a new release tag
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0

# Or use make command
make release VERSION=1.0.0
```

### Conventional Commits

Use [Conventional Commits](https://www.conventionalcommits.org/) for automatic changelog generation:

| Type | Description | Version Bump |
|------|-------------|--------------|
| `feat:` | New feature | Minor |
| `fix:` | Bug fix | Patch |
| `perf:` | Performance improvement | Patch |
| `refactor:` | Code refactoring | None |
| `docs:` | Documentation | None |
| `test:` | Tests | None |
| `chore:` | Maintenance | None |
| `BREAKING CHANGE:` | Breaking change | Major |

Examples:
```bash
git commit -m "feat: add user authentication"
git commit -m "fix: resolve login timeout issue"
git commit -m "feat!: redesign API response format"  # Breaking change
```

## Observability

### Tracing (Jaeger)

Access Jaeger UI at: http://localhost:16686

### Logging

Logs are structured JSON with trace context:

```json
{
    "level": "info",
    "ts": "2024-01-15T10:30:00.000Z",
    "caller": "handler/user_handler.go:45",
    "msg": "user registered",
    "trace_id": "abc123",
    "span_id": "def456",
    "user_id": "user-123"
}
```

## Worker

`cmd/worker` is a separate binary that consumes RabbitMQ messages. It sets up a
topic exchange/queue/binding, runs several concurrent consumers (each on its own
channel), and self-heals across connection/channel drops with poison-message
handling and panic recovery. Extend `handleMessage` in `cmd/worker/main.go` with
your business logic.

```bash
make run-worker       # Run the worker (go run ./cmd/worker)
make build-worker     # Build bin/veemon-worker
```

## Make Commands

```bash
# Application
make run              # Run the API server (HTTP + gRPC)
make run-worker       # Run the RabbitMQ worker
make build            # Build the server binary
make build-worker     # Build the worker binary
make dev              # Run the server with hot reload (Air)
make test             # Run tests with the race detector
make test-coverage    # Run tests with coverage profile
make lint             # Run golangci-lint
make fmt              # Format code
make clean            # Clean build artifacts

# Secrets (Infisical CLI-injected)
make infisical-run          # Run the server with Infisical-injected secrets
make infisical-run-worker   # Run the worker with Infisical-injected secrets

# Database Migrations
make migrate          # Run all pending migrations
make migrate-down     # Rollback all migrations
make migrate-rollback # Rollback last migration
make migrate-status   # Show current version
make migrate-create name=<name>  # Create new migration
make seed             # Run database seeders
make fresh            # Drop all and re-migrate
make fresh-seed       # Drop all, migrate, and seed
make refresh          # Rollback all and re-migrate
make refresh-seed     # Rollback all, re-migrate, and seed
make reset            # Rollback all migrations

# Docker
make docker           # Build Docker image
make compose-up       # Start infrastructure
make compose-down     # Stop infrastructure

# Other
make proto            # Regenerate proto (protoc-gen-go/-go-grpc/-fiber via buf)
make deps             # Download dependencies
make install-tools    # Install dev tools
```

## Declaring Routes in Proto

The REST surface is declared **directly on each RPC** in the `.proto` and the Go
Fiber routes are generated — there is no hand-written route table or auth map to
keep in sync. Method, path, path/query/body binding, auth policy, and rate limits
all live in one place: the contract.

Add a `veemon.route` option to a method (`contract/user/user.proto`):

```proto
import "veemon/annotations.proto";

service UserApi {
    rpc GetUser(GetUserReq) returns (UserProfile) {
        option (veemon.route) = {
            method: "GET"
            path: "/api/v1/users/{id}"          // {id} binds to request field `id`
            auth: { required: true roles: ["admin", "superadmin"] }
        };
    }

    rpc Register(RegisterReq) returns (RegisterRes) {
        option (veemon.route) = {
            method: "POST"
            path: "/api/v1/auth/register"
            body: true                          // parse JSON body into the request
            response: RESPONSE_STYLE_CREATED    // 201 instead of 200
            rate_limit: { max: 10 window_seconds: 60 }
        };
    }
}
```

Then run `make proto`. The [`protoc-gen-fiber`](apps/api/cmd/protoc-gen-fiber) plugin
generates `apps/api/handler/grpc/user/user_fiber.pb.go` containing:

- `RegisterUserApiRoutes(router, srv, validator)` — wires every route onto Fiber,
  applying the declared auth middleware and rate limiters, binding path params
  (`{id}` → `:id`), query params (for `GET`), and JSON body, then writing the
  response (`RESPONSE_STYLE_OK` / `_CREATED` / `_LIST`).
- `UserApiAuthConfig` — the gRPC full-method → auth policy map consumed by the
  gRPC auth interceptor, so **gRPC and REST enforce the same rules from one
  declaration**.

Both are wired in `config/bootstrap.go`. To add an endpoint you now: define the
RPC + messages, annotate it with `veemon.route`, run `make proto`, and implement
the method on the handler — no route file to touch.

Options reference (`contract/veemon/annotations.proto`): `method`, `path`, `body`,
`auth { required, roles }`, `response` (`RESPONSE_STYLE_OK|_CREATED|_LIST`), and
`rate_limit { max, window_seconds }`.

## Architecture Decisions

### Why Fiber?

- High performance (up to 10x faster than net/http)
- Express.js-like syntax
- Zero memory allocation in hot paths
- Built-in middleware ecosystem

### Why gRPC alongside REST?

- REST for external clients (web, mobile)
- gRPC for internal service-to-service communication
- Shared business logic via Clean Architecture

### Why Redigo over go-redis?

- Lower memory footprint
- Simpler API
- Better connection pool control
- Well-tested in production environments

### Why Zap over other loggers?

- Blazing fast (zero allocation in hot paths)
- Structured logging out of the box
- Easy integration with OpenTelemetry

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Run tests with race detection (`go test -race ./...`)
4. Commit your changes (`git commit -m 'Add amazing feature'`)
5. Push to the branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- [Go Fiber](https://gofiber.io/)
- [GORM](https://gorm.io/)
- [OpenTelemetry](https://opentelemetry.io/)
- [Goroutine Problems Reference](https://github.com/superbolang/golang-goroutines_problem)
