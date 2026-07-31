## Summary
<!-- What changed and why — the motivation/context, not just a restatement of the diff -->

## Type of Change
<!-- Mark all that apply -->
- [ ] `feat` — new feature
- [ ] `fix` — bug fix
- [ ] `refactor` — no functional change
- [ ] `docs` — documentation only
- [ ] `test` — tests only
- [ ] `chore` / `build` / `ci` — tooling, dependencies, CI/CD
- [ ] `perf` — performance improvement
- [ ] Breaking change (describe impact + migration path under "Breaking Changes" below)

## Related Issue
<!-- Fixes #123, Closes #456 -->

## Scope
<!-- Which part(s) of the monorepo does this touch? -->
- [ ] `apps/api` (Go backend)
- [ ] `apps/web` (React/Tauri)
- [ ] `apps/ai` (LangGraph.js)
- [ ] `packages/go-common` (shared Go infra: logging, monitoring, infra clients)
- [ ] `packages/api-client` / `packages/tsconfig`
- [ ] `contract/` (proto)
- [ ] Other (specify):

## Contract changes
<!-- Only if contract/*.proto changed -->
- [ ] `make proto` was run from repo root and the regenerated Go + TS output is committed
- [ ] New/changed routes declare an explicit `veemon.route` `auth { required, roles }` policy (fail-closed — see `.claude/rules/codebase-conventions.md`)

## Database changes
<!-- Only if migrations/ changed -->
- [ ] Migration is idempotent / safe to re-run (`IF NOT EXISTS`, `ON CONFLICT`, explicit guards)
- [ ] `golang-migrate` up/down pair is present and correctly named

## Observability
<!-- Only if you added/changed logging, metrics, or spans — see .claude/rules/observability.md -->
- [ ] Logs go through `logging.*` / `logger.WithContext(ctx)` — no `fmt.Println`/`log.Print`, no secrets or PII
- [ ] New metrics were added to `packages/go-common/monitoring/metrics` (not a one-off `promauto` elsewhere), with bounded labels (no free IDs)
- [ ] Manual spans (if any) use a low-cardinality name; IDs go in attributes, never the span name

## Testing
<!-- Describe what you ran and how a reviewer can reproduce it -->
- [ ] `apps/api`: `make -C apps/api lint test` (golangci-lint v2 + `go test -race`)
- [ ] `packages/go-common`: `go build ./... && go test ./...`
- [ ] `apps/web` / `apps/ai`: `bun run typecheck && bun test`
- [ ] Manual testing performed (describe below)
- [ ] Unit tests added/updated for the change
- [ ] Integration tests added/updated (DB/storage/3rd-party boundaries)

<!-- Paste relevant command output / repro steps here -->

## Code Review Checklist
<!-- Full list: .claude/rules/code-review-checklist.md -->
- [ ] All errors are handled properly (wrapped with `%w`, nothing silently swallowed)
- [ ] Context is passed to all I/O operations
- [ ] No goroutine leaks; race conditions checked (`go test -race`)
- [ ] No hardcoded credentials/secrets; new env keys added to `.env.example`
- [ ] Every new route/RPC has an explicit auth policy (fail-closed)
- [ ] REST responses go through `pkg/response` protojson helpers
- [ ] Diff is scoped to the task — no drive-by reformatting/renaming in unrelated code

## Breaking Changes
<!-- Impact + migration path, or "N/A" -->
N/A

## Screenshots
<!-- apps/web UI changes: before/after screenshots or a short clip -->

## Additional Notes
<!-- Anything else a reviewer should know -->
