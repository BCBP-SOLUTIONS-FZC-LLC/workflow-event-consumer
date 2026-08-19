## Description
Provide a clear description of the changes.

---

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Refactor
- [ ] Documentation
- [ ] Test
- [ ] Breaking change (env var removed/renamed, ack semantics changed)
- [ ] Routing catalogue change (`internal/consumer/routing.go`)

---

## Testing
- [ ] Unit tests added/updated (`make test`)
- [ ] LocalStack integration test added/updated (`make test-integration`)
- [ ] `make check` passes locally
- [ ] Manual testing performed (if required)

---

## Checklist

### Code Quality
- [ ] Code is properly formatted (`make fmt-check`)
- [ ] Linting passed (`make lint`)
- [ ] `go vet` passed
- [ ] No debug logs / commented-out code
- [ ] No secrets or tokens hardcoded

### Routing catalogue
*Complete only when `internal/consumer/routing.go` changed.*

- [ ] `eventRoutes` entry matches what the target downstream service(s)
      actually consume (checked their own `/internal/events` handler/catalogue)
- [ ] `routing_test.go` updated
- [ ] `docs/runbook.md`'s routing table updated
- [ ] LocalStack integration test case added if the change is worth exercising end-to-end

### Configuration
- [ ] New env vars documented in README's configuration table
- [ ] `internal/config/config.go`'s `Validate()` updated if the new var is required in prod
- [ ] `.env.example` updated

### Documentation
- [ ] CHANGELOG.md updated under `[Unreleased]`
- [ ] ARCHITECTURE.md updated (if component model, routing rationale, or ack semantics changed)
- [ ] docs/runbook.md updated (if operational procedures changed)

---

## Related Issue
Closes #<issue-id>

---

## Deployment Notes
Mention anything important for operators upgrading (new required env vars, routing catalogue changes, config changes).
