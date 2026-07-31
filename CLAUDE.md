# Claude Code Guidelines for Veemon

This file is the **index** for AI-assistant guidance. The detailed standards,
rules, agents, and skills live under [`.claude/`](.claude/) so each concern is a
small, focused file. Read the rule that matches the task; treat these as
authoritative for `apps/api` (the Go backend).

> This repo is a **polyglot Bun-workspace monorepo** — a Go backend (`apps/api`),
> a React + Tauri client (`apps/web`), and a LangGraph.js service (`apps/ai`),
> sharing one proto contract (`contract/`). Start with
> [rules/project-overview.md](.claude/rules/project-overview.md).

## Rules (`.claude/rules/`)

| Rule | Covers |
|------|--------|
| [project-overview](.claude/rules/project-overview.md) | Stack, monorepo layout, root Bun scripts |
| [architecture](.claude/rules/architecture.md) | Go backend layers, generated routes, data flow |
| [codebase-conventions](.claude/rules/codebase-conventions.md) | Fail-fast config, fail-closed auth, protojson responses |
| [coding-standards](.claude/rules/coding-standards.md) | File org, naming, errors, context, interfaces |
| [goroutines](.claude/rules/goroutines.md) | Leaks, races, deadlocks, worker pools, panics |
| [database](.claude/rules/database.md) | GORM usage, transactions, connection pool |
| [redis](.claude/rules/redis.md) | Redigo connection pooling |
| [testing](.claude/rules/testing.md) | Table-driven tests, mocks, race detection |
| [observability](.claude/rules/observability.md) | Logging shape, metric/span conventions, correlation, no secrets/PII |
| [security](.claude/rules/security.md) | OWASP Top 10 with Go examples |
| [api-responses](.claude/rules/api-responses.md) | Success / error / pagination envelopes |
| [commands](.claude/rules/commands.md) | Common `make` / `bun` commands |
| [code-review-checklist](.claude/rules/code-review-checklist.md) | Pre-PR checklist |

## Agents (`.claude/agents/`)

- [code-reviewer](.claude/agents/code-reviewer.md) — reviews Go changes against these rules before a PR.

## Skills (`.claude/skills/`)

- [proto-routes](.claude/skills/proto-routes/SKILL.md) — add/change a REST+gRPC endpoint via the proto contract, then `make proto`.

## Quick rules of thumb

- **REST routes are generated** — change `contract/<svc>/<svc>.proto` and run
  `make proto`; never hand-edit Fiber routes or the auth map.
- **Auth is fail-closed** — every route/RPC needs an explicit policy or the server
  panics at startup.
- **golang-migrate is authoritative** — schema changes go through SQL migrations.
- **Always** `go test -race ./...` for new behavior; never log secrets.
- **Composition root, not `main()`** — `cmd/server`/`cmd/worker` are thin
  `fx.New(...).Run()` entry points; wiring lives in `config/fx_*.go`
  (`CommonModule`/`ServerModule`/`WorkerModule`). `apps/ai`'s `composition.ts`
  uses Awilix the same way. Shared Go infra (logging, telemetry/metrics,
  redis/rabbitmq/resilience/database) lives in `packages/go-common` — a
  separate module, not `apps/api/pkg`.

## Related docs

- [README.md](README.md) — full project documentation.
- [AGENTS.md](AGENTS.md) — same guidance, for agent tools that read `AGENTS.md`.
