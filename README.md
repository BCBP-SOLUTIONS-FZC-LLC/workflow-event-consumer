# workflow-events-consumer

A stateless SNS → SQS → HTTP bridge. It subscribes to a set of SQS queues and
forwards each event envelope, unmodified, to whichever downstream service's
`POST /api/v1/internal/events` endpoint actually needs it. It never inspects or
decodes the payload beyond reading its event type for routing, and holds no
database of its own — duplicate delivery is absorbed by each downstream
service's own idempotency.

See [ARCHITECTURE.md](ARCHITECTURE.md) for the component model, routing
rationale, and ack-semantics sequence diagrams. See
[docs/runbook.md](docs/runbook.md) for the event routing catalogue and
on-call procedures. See [CONTRIBUTING.md](CONTRIBUTING.md) for the
development workflow and PR process.

## Configuration

Every queue, every downstream target, and the event-type routing table live
in **one file** — [`config/topology.example.yaml`](config/topology.example.yaml)
— loaded at startup by `internal/topology`. Adding a queue, a downstream
service, or a routing rule is a change to that file only, never to Go code.
Copy it to `config/topology.yaml`, fill in real queue URLs and target
addresses, and point `TOPOLOGY_CONFIG_PATH` at it. No secrets belong in that
file — each target only names the environment variable (`tokenEnv`) holding
its own token.

| Env var | Purpose |
| --- | --- |
| `APP_ENV` | `dev` or `prod` — `prod` requires every target's token env var to be set |
| `HEALTH_PORT` | `/healthz` and `/metrics` listen port (default `8082`) |
| `TOPOLOGY_CONFIG_PATH` | Path to the topology YAML file (default `config/topology.yaml`) |
| `AWS_REGION`, `AWS_ENDPOINT_URL` | AWS SDK region / LocalStack override |
| `CONSUMER_CONCURRENCY` | Concurrent handler goroutines per queue (default `10`) |
| `VISIBILITY_TIMEOUT` | SQS visibility timeout per queue (default `30s`) |
| `DRAIN_TIMEOUT` | Graceful-shutdown grace period per consumer (default `30s`) |
| `FORWARD_TIMEOUT` | Per-target HTTP client timeout (default `5s`) |
| *(one per target, named in topology.yaml's `tokenEnv`)* | e.g. `EXECUTION_SERVICE_TOKEN`, `DEFINITION_SERVICE_TOKEN` |

Each downstream service mints and rotates its own `INTERNAL_API_TOKEN`
independently — tokens are never assumed to be the same value across targets.

AWS SDK credentials come from its default provider chain (IRSA / instance
role in real environments); against LocalStack, set
`AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` to any static value — the
consumer has no static-credentials injection point of its own.

## Local development

```sh
cp .env.example .env
cp config/topology.example.yaml config/topology.yaml   # fill in real queue URLs / addrs
make setup    # installs .githooks/pre-commit
make build
make dev
```

Or run the full stack (LocalStack + two mock downstreams + the app) with:

```sh
gh auth token > .go_private_token   # or any token with read access to this org's private Go modules
docker compose up
```

## Running tests

```sh
make test              # unit tests (uses test/mocks' mock downstreams)
make test-integration  # requires Docker; runs against real LocalStack SNS/SQS
make test-ci            # both suites, merged coverage
make cover-check        # test-ci + coverage threshold gate
```

`test/mocks` provides a reusable mock downstream service (`mocks.Downstream`)
standing in for any real target's `/api/v1/internal/events` endpoint — used by both
the unit tests and the LocalStack integration tests. `cmd/mock-downstream` is
a separate, minimal standalone binary (not a test double) used only by
`docker compose up` for a working local dev loop.

## Other Makefile targets

```sh
make lint          # golangci-lint
make vuln          # govulncheck
make check         # fmt + lint + vet + coverage-gated tests — full local CI pass
make install-hooks # install .githooks/pre-commit
make docker-build  # build the container image (requires GO_PRIVATE_TOKEN)
make docker-check  # Dockerfile lint (hadolint) + dependency CVE scan (trivy)
make help          # list all targets
```

## CI

`.github/workflows/ci.yml` fans out to two reusable workflows:

- **validate-quality** — format, `go mod tidy` drift, `go vet`, `golangci-lint`, `govulncheck`.
- **validate-test** — build, unit + integration tests (race + coverage), coverage threshold gate.

## Deployment

See [deploy/helm](deploy/helm) for the Kubernetes Helm chart and
[deploy/aws](deploy/aws) for the IAM policy this service's IRSA role needs
(SQS receive/delete/change-visibility only — it never publishes).
