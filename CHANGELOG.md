# Changelog

All notable changes to `workflow-events-consumer` are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

### Changed

- The receive threshold and the stall window are settings, `MAX_RECEIVE_COUNT` (default 4, at least 1) and `RECEIVE_STALL_WINDOW` (default 2m), defaults unchanged; they are in the chart values and `.env.example`. The threshold must stay below every queue's redrive count (infrastructure-owned). The docs said the retries spread over about seven minutes: a message is handled at 0, 30 s, 90 s and 210 s and dead-lettered on the fifth receive at 450 s, so a target outage of about three and a half minutes is survived (about seven and a half with a threshold of 5, after the queues' redrive count is raised). LLD rev 1.5.
- `APP_ENV` and `AWS_REGION` are required with no default, so an unset value no longer runs as `dev` in `us-east-1`; `HANDLER_TIMEOUT` and `RETRY_BACKOFF_BASE` must be positive.
- `/healthz` returns 503 until the first SQS receive has succeeded and again when none has succeeded for two minutes, so a consumer that cannot receive (denied permission, wrong queue or region) stops being ready and is restarted.
- A consumer that returns without being stopped, and a health server that cannot bind, stop the process like a failed consumer does.
- Alerts for a failing SQS receive and for a queue backlog; the dead-letter page groups by event type and reason, the labels that series carries.
- Outside `dev` and `local` every target address must be `https://`, and the forwarding client does not follow redirects, so the internal token is never sent in cleartext or to another host.
- The Dockerfile removes the private-module credential in the same `RUN` that uses it, and the release build neither reads nor exports a registry cache; the builder cache of earlier releases held the token, so the `GO_PRIVATE_TOKEN` secret needs rotating.
- `RETRY_BACKOFF_BASE` is `30s` in the code, the chart and the example environment (a test keeps them equal), so the four retries span about 7 minutes and a rolling deploy of a target no longer dead-letters the events that arrived during it. The receive threshold is exported as one constant that a test compares with the local queues' redrive count; an alert fires when any dead-letter queue is not empty, which also covers SQS moving a message itself.
- The chart sets `terminationGracePeriodSeconds` to 90s (drain 30s plus the handler deadline), enables the ServiceMonitor by default, and the alerts select the job by suffix because the ServiceMonitor's job is prefixed with the release name.
- A forward with an unknown target name fails before any target is called, so no delivered event is reported failed. A topology that routes a type to `execution` without an Execution category fails to load. The handler-duration metric labels a message `dead_lettered` or `dropped` (an empty route) instead of `ok`.
- A malformed message body is no longer written to the logs; the log keeps its size and SHA-256 and the dead-letter queue keeps the body.
- **Platform libraries** `platform-events/v2` v2.1.0 and `platform-gincommon` v1.6.0; Go 1.26.9. Collectors are built with `platform-gincommon` constructors.
- `APP_ENV` must be one of `local`, `dev`, `test`, `staging`, `prod`; a malformed number or duration fails startup instead of falling back to its default; every target's token is required outside dev and local, not only in prod; settings are cross-checked (handler timeout within the visibility timeout, forward timeout within the handler timeout).
- The chart's topology matches the example: the workflow-stream-gateway target, the dashboard queue and the 11 dashboard routes are present, and the IAM policy lists the dashboard queue.
- The health server records HTTP metrics and recovers panics through the gincommon middleware, and sets timeouts.
- Non-test code carries no comments.
- The runbook's route and category tables list every route and category by IAM's PascalCase event type, and the topology example names the queue filter policy as the source of the dropped types.
- The image builds for `TARGETARCH` (default `amd64`), carries OCI labels in the final stage and is built with `-buildvcs=false`.

### Fixed

- A production deploy could not reach its targets: addresses had to be https while the services serve plain HTTP on 8080, and egress allowed 443 only. A target address may be http naming a port outside dev, the chart points at the services on 8080 and the NetworkPolicy opens `networkPolicy.targetPorts`.
- `/healthz` followed the receive watch, so a shared SQS or STS outage restarted every pod. It is liveness now and `/readyz` carries the watch; the readiness and startup probes use `/readyz`.
- The forward client drains a bounded part of each answer, so connections are reused instead of opened for every forward.

### Added

- A check that the documents this repository keeps in the design repository still match their copies there: `make design-sync-check` (`DESIGN_DIR` reads a local checkout, otherwise `GH_TOKEN` reads `Haroon-BCBP/design`) compares the pairs listed in `scripts/design-sync.manifest`, and `make design-sync-check-test` tests the script. The CI step runs when `DESIGN_REPO_TOKEN` is set and says so when it is not.
- `TenantStateChanged` and `TenantMembershipsPurged` are routed to the Definition Service as well as to Execution: Definition records a departed tenant and purges its templates and credentials after a retention period (Definition LLD §7.4.6). The topology example, the local topology, the chart values, the runbook, the AsyncAPI and the LLD agree, and a test fails if either file routes them to one service only. The Definition contract copy and its `SOURCES` pin follow its internal spec.
- CI lints the Dockerfile, builds the image and scans it with Trivy (fixable HIGH and CRITICAL fail the build), as the other workflow services do.
- Contract tests (`test/contract`, run by `make test`) forward every event type the topology routes to the Definition Service or the Execution Service through the real forwarder and check each request, and every status the spec declares, against that service's internal OpenAPI copy in `api/contracts/`. `make contract-check` and a quality-workflow step compare each copy with its source.
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
- The integration tests load the topology example's routes and run the consumer with the production timeouts; config validation tests assert the error text; the chart drift test compares every target field; `buildDeps` is tested. The IRSA verification step uses a debug container because the image has no shell.
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
- The defect-class linters are on (bodyclose, noctx, nilerr, gosec, contextcheck, gocritic, nolintlint, sqlclosecheck) and lint covers the `integration` build tag.

