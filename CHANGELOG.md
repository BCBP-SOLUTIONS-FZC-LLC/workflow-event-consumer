# Changelog

All notable changes to `workflow-events-consumer` are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

### Added

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
