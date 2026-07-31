# Code Review Checklist

- [ ] All errors are handled properly
- [ ] Context is passed to all I/O operations
- [ ] No goroutine leaks (all goroutines can terminate)
- [ ] Race conditions checked (`go test -race`)
- [ ] Sensitive data is not logged (secrets/PII) — logs go through `logging.*`, not `fmt.Println`
- [ ] New metrics use bounded labels (no free IDs) and live in `packages/go-common/monitoring/metrics`
- [ ] Input validation is present
- [ ] Tests are included for new functionality
- [ ] No hardcoded credentials or secrets
- [ ] Database queries use parameterized inputs
- [ ] Resources are properly closed (defer)
- [ ] Every new route/RPC has an explicit auth policy (fail-closed)
- [ ] New env keys are added to `.env.example`
- [ ] REST responses go through `pkg/response` protojson helpers
