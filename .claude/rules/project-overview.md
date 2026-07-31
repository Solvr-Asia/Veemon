# Project Overview & Monorepo Layout

This repo is a **polyglot Bun-workspace monorepo**. The Go backend lives in
`apps/api` (all Go paths in these rules are relative to it); `apps/web` is a
React + TanStack Router + Tauri client and `apps/ai` is a LangGraph.js service, both
consuming the Go API through the generated `@veemon/api-client`
(`packages/api-client`). The proto contract in `contract/` (repo root) is the
single source of truth generating Go **and** TypeScript. See the layout below and
`docs/superpowers/specs/` for the design.

The Go backend (`apps/api`) is a Go monolithic application using:

- **Go Fiber** for REST API
- **gRPC** for service-to-service communication
- **Domain-Driven Design (DDD)** with Clean Architecture
- **GORM** for database ORM (PostgreSQL); **golang-migrate** for schema migrations (source of truth)
- **PASETO v4** (`pkg/token`) for authentication tokens; **Redis-backed** login lockout + token revocation (`pkg/authguard`)
- **Redigo** for Redis caching
- **RabbitMQ** for message queuing (auto-reconnecting client)
- **Zap** for structured logging
- **OpenTelemetry** for distributed tracing
- **Viper** for configuration management
- **go-playground/validator** for validation
- **failsafe-go** for resilience (circuit breaker, retry, timeout)
- **Prometheus** for metrics and monitoring
- **Scalar** for API documentation (OpenAPI 3)
- **uber-go/fx** for dependency injection — `cmd/server`/`cmd/worker` are thin
  `fx.New(...).Run()` composition roots (see `config/fx_*.go`)

## Monorepo Layout

```
apps/api/       → Go backend (Fiber REST + gRPC) — the module the rules describe
apps/web/       → React + TanStack Router + Vite + Tauri (@veemon/web)
apps/ai/        → LangGraph.js agent + workflow service (@veemon/ai, Hono HTTP)
packages/api-client/ → generated protobuf-es types + typed REST client (@veemon/api-client)
packages/tsconfig/   → shared base tsconfig (@veemon/tsconfig)
packages/go-common/  → shared Go infra module (logging/, monitoring/, infra/) —
                       pulled into apps/api via a go.mod `replace`, not a Bun
                       workspace member
contract/       → proto source of truth (repo root) — generates Go + TS
buf.gen.yaml    → Go → apps/api/handler/grpc; TS → packages/api-client/src/gen
package.json    → Bun workspaces + root scripts (proto, dev, build, test)
```

Root scripts (Bun): `bun run proto` (regenerate Go + TS), `bun run dev`
(api + web + ai), `bun run build`, `bun run test`. `apps/api` is orchestrated via
`make` and is not a Bun workspace member. The TS apps call the existing Fiber
REST routes through `@veemon/api-client` — no changes to the Go server (Approach A).
