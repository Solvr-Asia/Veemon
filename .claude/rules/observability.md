# Observability — what to log, log shape, when to add a metric/span

The Go equivalent of the "while-coding" observability checklist: what to log
(and what not to), what shape a log takes, and what actually deserves a metric
or a manual span. Logging is [`veemon-common/logging`](../../packages/go-common/logging)
(Zap); tracing is automatic per-request via
[`pkg/middleware/tracing.go`](../../apps/api/pkg/middleware/tracing.go) + gorm's
OTel plugin + `otelgrpc`; metrics are Prometheus via
[`veemon-common/monitoring/metrics`](../../packages/go-common/monitoring/metrics).
Paths below are relative to `apps/api/` unless prefixed `packages/`.

## TL;DR

Tracing is automatic per-request (HTTP, gRPC, DB) — you rarely open a span
yourself. When writing code:

1. Log through `logging.*` / `logger.WithContext(ctx)` — never `fmt.Println`/`log.Print`.
2. Log **sparingly** — lifecycle + outcomes, not narration or per-item chatter.
3. Add a **metric** to `packages/go-common/monitoring/metrics` only for a real
   domain outcome worth counting — not per-function.

## Logging — use the wrapper

- Use `veemon-common/logging` — package-level `logging.Info/Warn/Error/Debug/With(...)`,
  or `logger.WithContext(ctx)` on an injected `*logging.Logger` (adds `trace_id`/`span_id`
  when a span is active, via `pkg/logger`'s OTel integration). Never `fmt.Println`,
  `log.Print`, or writing straight to `os.Stdout`.
- **Why (not style):** `LoggerMiddleware` (`pkg/middleware/logger.go`) already logs every
  request with `request_id`/`trace_id`/`span_id` attached — a raw `fmt.Println` bypasses
  that correlation entirely and can't be filtered by request or trace in the OTel backend.
- **Enforced by:** code review (see `code-review-checklist.md`) — there's no
  `fmt.Println`-ban linter yet, so flag it on sight rather than relying on CI.

## When to log what

- **INFO** — lifecycle/outcome, sparingly: request/job start+finish, state transitions,
  external-call outcomes (`"Redis connection established"`, `"OpenTelemetry initialized"`).
  **Not** per-loop / per-item chatter.
- **WARN** — degraded but handled: a retry fired, a fallback taken, a soft-failure absorbed
  (e.g. `provideOptionalRedis`'s "Failed to connect to Redis, caching disabled").
- **ERROR** — a failed operation a human may need to act on; wrap the error with `%w` and
  include actionable detail (`logging.Error("failed to bootstrap", zap.Error(err))`).
- **DEBUG** — never shipped to production; dev-console only.

## Do NOT log (delete on sight)

- Mechanics narration / restating code; per-loop/per-item chatter — aggregate to a count
  or a metric instead.
- Routine success confirmations the request-logging middleware already covers.
- **PII** — emails, full user records, doctor/patient-style personal data.
- **Secrets** — `JWT_SECRET`, DB DSNs, Redis/RabbitMQ credentials, PASETO keys, tokens.
- **Raw request/response bodies** — log status + a short reason, never the body.

## Log shape

Structured Zap fields, not string interpolation or `fmt.Sprintf`:

```go
// ✓ Good
logging.Error("failed to create user", zap.String("email", email), zap.Error(err))

// ✗ Bad — no structure, can't be filtered/aggregated
logging.Error(fmt.Sprintf("failed to create user %s: %v", email, err))
```

## Metrics — only for a real domain outcome

- Predefined in `packages/go-common/monitoring/metrics` (the `Metrics` struct) — HTTP,
  business, DB, cache, queue, and circuit-breaker families are already registered there.
  Add a new metric to that struct rather than calling `promauto` inline elsewhere, so
  everything stays on the one `*prometheus.Registry` served at `/metrics`.
- Prometheus naming: `<namespace>_<area>_<thing>_total` (snake_case, underscore-joined —
  e.g. `veemon_users_registered_total`). Prometheus metric names don't allow `.`, so this
  is the Go/Prometheus equivalent of the dot-notation used in other stacks — not a copy of it.
- **Labels must be bounded.** `metrics.go`'s own HTTP middleware already models this
  correctly: it labels by `c.Route().Path` (the route *pattern*, e.g. `/api/v1/users/:id`),
  never the raw path with a real ID baked in — follow that pattern. If you can't enumerate
  the possible values (a small closed set like `status∈{2xx,4xx,5xx}` or `operation`), it's
  not a label, it's a log/span attribute.
- **Never** `user_id` / `request_id` / any free-form ID as a label — those stay log/span
  attributes only.
- Don't rename or relabel an existing metric (e.g. `http_requests_total`'s
  `method`/`path`/`status`) — live dashboards break; add a new instrument instead.

## Tracing — you barely touch it

Already automatic: HTTP via `TracingMiddleware` (`pkg/middleware/tracing.go` — one span
per request, named by route pattern, correlated to `request_id`), gRPC via
`otelgrpc.NewServerHandler()` (`config/bootstrap.go`), and every GORM query via
`gorm.io/plugin/opentelemetry`. Open a **manual span** only for a notable internal stage
none of those cover (a multi-step business operation, an external HTTP call): inject
`*telemetry.Telemetry` via fx and call `tel.StartSpan(ctx, "verb noun")` — low-cardinality
span name, IDs go in **attributes**, never the name. `span.RecordError`/`SetStatus` on the
HTTP and gRPC paths is already automatic; for a manual span, call
`telemetry.RecordError(ctx, err)` yourself on failure.

## Correlation (one line)

`request_id` is the shared key: set by `RequestIDMiddleware` (`pkg/middleware/requestid.go`),
attached to the HTTP span as the `request.id` attribute (`tracing.go`), and logged as
`request_id` by `LoggerMiddleware` alongside `trace_id`/`span_id` — filter any backend by
`request_id` to join that request's logs, trace, and (if labeled) metric.

## Micro-examples

- `fmt.Println("user", id, "registered")` → `logging.Info("user registered", zap.String("user_id", id))`
- A one-off `prometheus.NewCounter(...)` inside a handler → add the counter to
  `packages/go-common/monitoring/metrics/metrics.go`'s `Metrics` struct instead, so it's on
  the shared registry and the `/metrics` endpoint.
