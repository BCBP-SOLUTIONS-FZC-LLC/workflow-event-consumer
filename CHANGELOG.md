# Changelog

All notable changes to `workflow-events-consumer` are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

### Added

- Telemetry through `platform-gincommon` and `platform-events`: the `platform_*` series (including `platform_messages_*`, `platform_dlq_*` and `platform_queue_depth`) in every scrape, `make metrics-lint`, a generated `docs/observability/metric-registry.md`, and a scrape test that also checks every metric an alert rule names exists.
- A per-message handler timeout, redelivery backoff, queue-depth metrics and malformed-body logging on every consumer (`HANDLER_TIMEOUT`, `RETRY_BACKOFF_BASE`, `RETRY_BACKOFF_MAX`, `QUEUE_DEPTH_INTERVAL`).
- A supervisor that stops the process, so the pod restarts, when any consumer exits with an error.
- The forward duration metric is recorded, and forwarded requests carry the trace context.
- Alerts for failing messages, dead-lettered messages and handler panics.
- Tests that fail when the chart's topology or `deploy/aws/policy.json` differs from `config/topology.example.yaml`.
- `make arch-lint` with a `.go-arch-lint.yml` that fixes the import direction between `internal` packages, run by `make check` and the quality workflow.
- A low-level design (`docs/lld/workflow_event_consumer.md`) with the queue and event inventory across IAM and the Workflow Engine, and `api/asyncapi.yaml`, whose channels, event types and per-event targets tests tie to the topology example.
- A queue for `delegation-workflow-q` (the `iam.delegation.events` topic) in the topology, chart, IAM policy and local stack, with explicit drops for `DelegationReviewRequested` and `DelegationEscalationRequested`.
- A permanent forward failure (`400`, `413` or `422` from every failing target) sends the original message to the queue's dead-letter queue after one delivery, through the `platform-events` DLQ publisher; transient and mixed failures keep the retry path. The IAM policy gains send and attribute-read on each `<queue>-dlq`, and a test ties it to the topology.
- The consumer forwards the original message body, so a schema-encoded event reaches its target with its payload and `dataschema` intact; it decodes nothing itself. A type outside the Execution Service's category map is not forwarded to it (it has no route for it) and is counted with result `skipped`.
- Tests that fail when the runbook's route table or category table differs from the topology example or the category map.
- A release workflow (changelog gate, image build, Cosign signature, GitHub Release), a changelog check, `.editorconfig`, `helm-lint`, `lint-workflows`, a Dockerfile digest pin check, VERSIONING.
- Initial implementation: stateless SNS → SQS → HTTP bridge subscribing to
  `membership`, `user`, and `template` SQS queues and forwarding each event
  to `execution_service` and/or `definition_service`'s `POST /internal/events`.
- Event-type routing catalogue — each known event type routes to exactly the
  downstream service that needs it; an unrecognized type falls back to
  broadcasting to every registered target.
- `config/topology.yaml` — a single file declaring every SQS queue, every
  downstream target, and the event routing table (`internal/topology`
  package). Adding a queue, a target, or a routing rule is a config edit, not
  a Go code change. Loaded once at startup; `internal/consumer.Router` and
  `cmd/event-consumer/deps.go` are both driven entirely by it.
- `cmd/mock-downstream` — a minimal standalone binary (distinct from
  `test/mocks.Downstream`) used only by `docker compose up`, giving the
  local dev loop a fully working stack without either real downstream
  service checked out.
- `docker-compose.yml` + `scripts/init-localstack.sh` — full local dev stack
  (LocalStack + two mock downstreams + the app), auto-provisioning the same
  topology used by the integration test fixture.
- `deploy/helm` — Kubernetes Helm chart (Deployment, Service, ServiceAccount,
  ConfigMap/Secret for topology and tokens, HPA, PodDisruptionBudget,
  NetworkPolicy, ServiceMonitor). `deploy/aws` — the IAM policy for this
  service's IRSA role (SQS consume-only, never `sns:Publish`). `deploy/monitoring`
  — Prometheus alert rules.
- `.github/{CODEOWNERS,dependabot.yml,SECURITY.md,ISSUE_TEMPLATE/*}` and
  `.githooks/pre-commit` (+ `make setup`/`install-hooks`) — repo scaffolding
  parity with this org's other services.
- All-or-nothing ack semantics: a message is deleted only once every
  resolved target returns `2xx`; otherwise it stays visible for SQS's own
  redelivery and eventual DLQ move.
- Prometheus metrics (`workflow_events_consumer_forward_total`,
  `..._forward_duration_seconds`, `..._handler_duration_seconds`).
- Reusable mock downstream test double (`test/mocks.Downstream`) used by
  both unit tests and the LocalStack integration test.
- Full CI (`validate-quality`, `validate-test`), Dockerfile, and Makefile
  modeled on this org's existing service conventions.

### Changed

- **Platform libraries** `platform-events/v2` v2.1.0 and `platform-gincommon` v1.6.0; Go 1.26.9. Collectors are built with `platform-gincommon` constructors.
- `APP_ENV` must be one of `local`, `dev`, `test`, `staging`, `prod`; a malformed number or duration fails startup instead of falling back to its default; every target's token is required outside dev and local, not only in prod; settings are cross-checked (handler timeout within the visibility timeout, forward timeout within the handler timeout).
- The chart's topology matches the example: the workflow-stream-gateway target, the dashboard queue and the 11 dashboard routes are present, and the IAM policy lists the dashboard queue.
- The health server records HTTP metrics and recovers panics through the gincommon middleware, and sets timeouts.
- Non-test code carries no comments.
- The runbook's route and category tables list every route and category by IAM's PascalCase event type, and the topology example names the queue filter policy as the source of the dropped types.
