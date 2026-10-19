# workflow-events-consumer

The shared SNS → SQS → HTTP bridge of the BCBP Workflow Engine: it consumes a set of SQS queues and forwards each event envelope, unmodified, to the downstream service whose internal events endpoint needs it.

**Repository:** `github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer`  
**Module:** Go 1.26.9 · private module · a containerised microservice not yet deployed to any live environment

---

## Mental model

```text
SNS ──► SQS queues ──► Events Consumer ──HTTP──► Execution / Definition / Stream Gateway
                          topology.yaml: queues, targets, routing
```

| Concern | What this service owns | What it does NOT own |
| --- | --- | --- |
| **Consumption** | One SQS consumer per configured queue, started through `platform-events` | The queues, their redrive policy and dead-letter queues: infrastructure owns them |
| **Routing** | The topology file that maps each event type to its target services | What an event means: the producer owns the payload and the target owns the handling |
| **Forwarding** | One authenticated `POST` of the unmodified envelope per target, all-or-nothing per message | Idempotency: every target absorbs a redelivered envelope itself |
| **Dedup and storage** | Nothing: the service has no database and no inbox | Processed-event tables: each target keeps its own |

**Source of truth for one contract:** the topology file, [config/topology.example.yaml](config/topology.example.yaml).

---

## Why this service exists

Without a shared bridge, every service would run its own SQS consumers, retry policy and dead-letter handling. Centralising consumption gives one place that owns queue permissions, backoff, the per-message deadline and the consume metrics, and a topology file where a new queue, target or route is a configuration change rather than a code change.

---

## Contents

- [Configuration](#configuration)
- [Local development](#local-development) and [Testing](#testing)
- [Architecture](#architecture)
- [Deployment](#deployment), [Observability](#observability), [Security](#security)
- [CI](#ci), [Versioning and releases](#versioning-and-releases)
- [Out of scope](#out-of-scope) and [See also](#see-also)

> **Design** — [docs/lld/workflow_event_consumer.md](docs/lld/workflow_event_consumer.md), [api/asyncapi.yaml](api/asyncapi.yaml). **Architecture** — [ARCHITECTURE.md](ARCHITECTURE.md). **On-call** — [docs/runbook.md](docs/runbook.md). **Contributing** — [CONTRIBUTING.md](CONTRIBUTING.md).

---

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
| `APP_ENV` | Required. One of `local`, `dev`, `test`, `staging`, `prod`; any other value fails startup. Outside `dev` and `local` every target address must be https, or http naming a port (the in-cluster services serve plain HTTP on 8080), and every target token env var must be set |
| `HEALTH_PORT` | `/healthz`, `/readyz` and `/metrics` listen port (default `8082`) |
| `TOPOLOGY_CONFIG_PATH` | Path to the topology YAML file (default `config/topology.yaml`) |
| `AWS_REGION`, `AWS_ENDPOINT_URL` | AWS SDK region (required) and LocalStack override |
| `CONSUMER_CONCURRENCY` | Concurrent handler goroutines per queue (default `10`) |
| `VISIBILITY_TIMEOUT` | SQS visibility timeout per queue (default `30s`) |
| `DRAIN_TIMEOUT` | Graceful-shutdown grace period per consumer (default `30s`) |
| `FORWARD_TIMEOUT` | Per-target HTTP client timeout (default `5s`) |
| `HANDLER_TIMEOUT` | Per-message deadline (default `20s`); must not exceed `VISIBILITY_TIMEOUT`, and `FORWARD_TIMEOUT` must not exceed it |
| `RETRY_BACKOFF_BASE`, `RETRY_BACKOFF_MAX` | Redelivery backoff after a failed message (defaults `30s`, `5m`); with the default receive threshold a message is handled at 0, 30 s, 90 s and 210 s and dead-lettered on its fifth receive at 450 s, so a target outage of about 3.5 minutes is survived |
| `MAX_RECEIVE_COUNT` | Receive count above which a message is dead-lettered instead of handled (default `4`, at least 1): four handling attempts, the fifth receive dead-letters. It must stay below every queue's redrive count (infrastructure-owned), or SQS moves the message first |
| `RECEIVE_STALL_WINDOW` | How long without a successful receive before `/readyz` answers 503 (default `2m`) |
| `QUEUE_DEPTH_INTERVAL` | Cadence of the queue-depth metric (default `30s`) |
| `OTEL_SERVICE_NAME`, `OTEL_EXPORTER_OTLP_ENDPOINT` | Trace and metric identity; export is off when the endpoint is unset or `none`. The NetworkPolicy allows egress on 443 and the target ports only, so a collector on another port (the usual 4317) needs an entry in `networkPolicy.additionalEgressRules` |
| *(one per target, named in topology.yaml's `tokenEnv`)* | e.g. `EXECUTION_SERVICE_TOKEN`, `DEFINITION_SERVICE_TOKEN` |

Each downstream service mints and rotates its own `INTERNAL_API_TOKEN`
independently — tokens are never assumed to be the same value across targets.

AWS SDK credentials come from its default provider chain (IRSA / instance
role in real environments); against LocalStack, set
`AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` to any static value — the
consumer has no static-credentials injection point of its own.

A malformed number or duration fails startup rather than falling back to a default. A queue without a redrive policy to a dead-letter queue fails startup.

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

## Testing

```sh
make test              # unit tests, using test/mocks' mock downstreams
make test-integration  # requires Docker; real LocalStack SNS and SQS
make test-ci           # both suites, merged coverage
make check             # format, lint, vet, import rules, the observability standard, chart lint and the coverage gate
make contract-check    # api/contracts matches its source (needs GH_TOKEN, or CONTRACT_SRC_DIR=<checkout> with CONTRACT_SRC_REPO=<owner/repo> for one local source)
```

`make test` also runs `test/contract`: every event type the topology routes to the Definition Service or the Execution Service is forwarded through the real forwarder and held to that service's OpenAPI copy in `api/contracts/`.

`test/mocks` provides a reusable mock downstream (`mocks.Downstream`) used by the unit and integration tests; `cmd/mock-downstream` is a standalone binary used only by `docker compose up`.

| Command | What it does |
| --- | --- |
| `make lint` / `make arch-lint` / `make vuln` | golangci-lint, import-direction rules, dependency vulnerabilities |
| `make metrics-lint` / `make metrics-inventory` | Observability standard, regenerate the inventory |
| `make helm-lint` / `make lint-workflows` | Chart validation, workflow lint |
| `make docker-check` / `make pin-base-images` | Dockerfile lint and scan, digest pins |

## Architecture

`cmd/event-consumer` is the composition root: configuration, telemetry, one consumer per queue, a supervisor that exits the process when a consumer stops, and the health server. `internal/topology` loads the queue, target and routing file; `internal/consumer` builds each SQS consumer and resolves an event type to targets; `internal/forwarder` is transport only. Dependency ownership: `platform-gincommon` owns logging, tracing, HTTP middleware and every metric constructor; `platform-events` owns the SQS consumer, the envelope, dead-lettering and the consume metrics. Details are in [ARCHITECTURE.md](ARCHITECTURE.md).

---

## Deployment

One image from the `Dockerfile` (target `event-consumer`), base images pinned by digest. The chart in `deploy/helm` has a Deployment, a PDB, a NetworkPolicy, an optional HPA and `ServiceMonitor`, and renders the topology into a ConfigMap. `deploy/aws` holds the IAM policy of the service's role: SQS receive, delete, change-visibility and attribute reads on the topology's queues, and nothing else. A test fails when the chart's topology or the policy differs from `config/topology.example.yaml`. `deploy/monitoring/app-alerts.yml` holds the alert rules.

## Observability

Every series carries `domain` (`workflow`), `service` (`workflow-events-consumer`) and `environment`. Service metrics are `workflow_events_consumer_*`; the consumer, dead-letter (including the dead-letters this service sends) and queue-depth series are `platform_*` from `platform-events`. [docs/observability/metric-registry.md](docs/observability/metric-registry.md) is generated from a real scrape, and `make metrics-lint` fails on a collector built outside `internal/observability`, a name the standard rejects, an alert naming a metric that does not exist, or a stale inventory. Forwarded requests carry the trace context.

## Security

- Each target has its own token, sent as `x-internal-token`; outside dev and local startup fails if any target's token is missing.
- The IAM role can only consume SQS; it never publishes and never touches S3, KMS or Glue.
- The health server serves `/healthz`, `/readyz` and `/metrics` only, with read, write and idle timeouts. `/healthz` is liveness and answers while the process serves. `/readyz` answers 503 until the first SQS receive has succeeded and again when none has succeeded for `RECEIVE_STALL_WINDOW` (two minutes by default), so a consumer that cannot receive is taken out of service; the chart's liveness probe does not follow it, since a restart cannot fix an SQS or STS outage.
- Container: distroless, non-root, read-only root filesystem, all capabilities dropped.

---

## CI

Pull requests and pushes run `ci.yml`: a `ci-gate` job skips draft pull requests and those labelled `skip-ci`, then `validate-quality` and `validate-test`, and an image job that lints the Dockerfile, builds the image and scans it with Trivy for fixable HIGH and CRITICAL findings. Pull requests that change only Markdown or `docs/` skip CI. `changelog-check.yml` requires a CHANGELOG entry on pull requests that touch code or configuration.

## Versioning and releases

Releases are Git tags `vMAJOR.MINOR.PATCH` cut on the merged commit after the CHANGELOG `[Unreleased]` section moves to a versioned heading. The `Release` workflow refuses a tag with no CHANGELOG entry, builds, scans and signs the image, and creates the GitHub Release. See [VERSIONING.md](VERSIONING.md) and [CHANGELOG.md](CHANGELOG.md).

---

## Out of scope

| Concern | Owner |
| --- | --- |
| Queues, topics, subscriptions and redrive policies | Infrastructure |
| Event payload schemas | The producing services |
| Processing and deduplicating an event | Each target service |

This repository carries no `LICENSE` file: it is private and proprietary to BCBP Solutions.

## See also

- [LLD](docs/lld/workflow_event_consumer.md) · [AsyncAPI](api/asyncapi.yaml) · [ARCHITECTURE.md](ARCHITECTURE.md) · [CONTRIBUTING.md](CONTRIBUTING.md) · [VERSIONING.md](VERSIONING.md) · [CHANGELOG.md](CHANGELOG.md) · [docs/runbook.md](docs/runbook.md)
