# Contributing

This is an internal bridge service for the BCBP Workflow Engine platform.
This guide covers development setup, how to extend the service, test
requirements, and the PR process.

## Prerequisites

- Go 1.26.5+ (matches `go.mod`)
- Docker (required for the LocalStack SNS/SQS integration test via `testcontainers-go`, and for `docker compose up`)
- `golangci-lint` and `govulncheck` are managed as dev tools via `make tools` — no separate install needed
- `GOPRIVATE=github.com/BCBP-SOLUTIONS-FZC-LLC/*` (private module access)

## Development setup

```bash
git clone https://github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer
cd workflow-events-consumer
make setup    # copies .env.example -> .env, installs .githooks/pre-commit
cp config/topology.example.yaml config/topology.yaml   # fill in real queue URLs/addrs
make tools     # install golangci-lint, govulncheck, hadolint, trivy
make build
make lint
make test
```

To run the LocalStack integration test (requires Docker):

```bash
make test-integration
```

Or run the full local stack (LocalStack + two mock downstreams + the app):

```bash
docker compose up
```

## Project layout

```
cmd/event-consumer/       ← Composition root — config, telemetry, consumer wiring and supervision, graceful shutdown
cmd/mock-downstream/       ← Minimal standalone binary used only by docker-compose (not a test double)
internal/
  config/                 ← Typed env-derived Config (non-topology settings only) + validation
  topology/                ← Loads/validates config/topology.yaml — the queue/target/
                              routing registry (config.go, load.go, resolve.go)
  consumer/                ← Per-queue SQS wiring (queue.go), the event handler
                              (handler.go), and Router — resolves a topology.Config's
                              routing table against an event type (routing.go)
  forwarder/               ← Transport-only concurrent fan-out to named HTTP
                              targets (forwarder.go, target.go) — no event-type
                              knowledge of its own
  observability/           ← Collectors built with gincommon constructors, and telemetry initialisation
config/
  topology.example.yaml    ← Template — copy to topology.yaml and fill in real values
  topology.local.yaml      ← Real, checked-in topology for docker-compose (LocalStack + mocks)
test/
  fixtures/                ← LocalStack SNS/SQS topology fixture (integration-only)
  integration/             ← LocalStack-backed end-to-end test
  mocks/                   ← Reusable mock downstream service (mocks.Downstream)
docs/
  architecture/mermaid/    ← Diagram sources referenced from ARCHITECTURE.md
  runbook.md               ← Event routing catalogue + on-call procedures
deploy/
  helm/                    ← Kubernetes Helm chart
  aws/                     ← IAM policy for this service's IRSA role
  monitoring/              ← Prometheus alert rules
```

**Dependency rule:** `forwarder` and `observability` have no knowledge of
`consumer` or `topology`; `topology` has no knowledge of `consumer`;
`consumer` depends on both `forwarder` and `topology`'s output types.
Nothing in `internal/` imports `cmd/`. See [ARCHITECTURE.md](ARCHITECTURE.md)
for the full component model.

## Extending the service

### Adding a new event type, queue, or downstream target

See [docs/runbook.md](docs/runbook.md#adding-a-new-event-type) — this is a
`config/topology.yaml` edit, not a Go source change. Only a genuinely new
kind of target behavior (e.g. a different auth scheme) would need Go changes,
in `internal/forwarder/target.go`.

### Testing conventions

- Unit tests use `test/mocks.Downstream` to stand in for a downstream
  service — don't hand-roll a new `httptest.Server` closure per test file.
- The LocalStack integration test (`test/integration/localstack_flow_test.go`)
  is the only place that exercises a real SQS consumer end-to-end; keep unit
  tests fast and Docker-free.
- New code should not lower the merged coverage total below the Makefile's
  `COVER_THRESHOLD` — see `make cover-check`.

## PR process

- Keep PRs scoped to one change; unrelated cleanups belong in a separate PR.
- `make check` must pass locally before opening a PR (installed automatically
  as a pre-commit hook via `make setup`/`make install-hooks`).
- CI runs `validate-quality` (fmt/tidy/vet/lint/vuln) and `validate-test`
  (build/test/coverage) — both must be green to merge.
- Update [CHANGELOG.md](CHANGELOG.md) under `[Unreleased]` for any
  user-visible or operationally-relevant change (new env var, new event
  route, changed ack semantics, etc.).
- Update [ARCHITECTURE.md](ARCHITECTURE.md) and/or
  [docs/runbook.md](docs/runbook.md) if the change affects what they
  document.

## Conventions

- Non-test code carries no comments except `//go:` and `//nolint` directives.
- Logs, metrics and traces go through `platform-gincommon`, and every SQS consume, dead-letter and event envelope through `platform-events`. New collectors are `workflow_events_consumer_*`, built in `internal/observability`; `make metrics-lint` must pass.
- A fix starts with a test of the correct behaviour that fails, then the change.
- A topology change updates `config/topology.example.yaml`, the chart's `topology` values and `deploy/aws/policy.json` together; a test fails when they differ.
- Never add a `replace` directive to `go.mod`.
- A change to behaviour updates the README, `ARCHITECTURE.md`, the runbook and the CHANGELOG in the same pull request.
