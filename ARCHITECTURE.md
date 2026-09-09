# Architecture

This document describes the internal structure and runtime data flows of
`workflow-events-consumer`.

`workflow-events-consumer` is a **private Go service**
(`github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer`, Go 1.26.5+). It
is a stateless bridge: it owns no database, no business logic, and no event
catalogue interpretation beyond routing. It exists so that
`execution_service` and `definition_service` never have to run their own SQS
consumers.

---

## Component model

> Source: [`docs/architecture/mermaid/component-diagram.mmd`](docs/architecture/mermaid/component-diagram.mmd)

```mermaid
graph TD
    subgraph cmd["Composition Root — cmd/event-consumer/"]
        main["main.go\nload config · register metrics\nbuild deps · start consumers\ngraceful shutdown (SIGINT/SIGTERM → drain)"]
        deps["deps.go\nbuildDeps — loads topology.yaml,\nwires Forwarder + N SQS consumers"]
        health["health.go\n/healthz · /metrics (promhttp)"]
    end

    subgraph topology["internal/topology/"]
        topoload["load.go — Load(path) config/topology.yaml\nvalidate: unique names, routes reference\nonly declared targets"]
        resolve["resolve.go — ResolveTargets(appEnv, os.Getenv)\nreads each target's tokenEnv"]
    end

    subgraph consumer["internal/consumer/"]
        queue["queue.go\nQueueSpec · Options · Build\n(wraps events.NewSQSConsumer)"]
        handler["handler.go\nMakeHandler — marshal envelope,\nresolve targets via Router, forward, ack/no-ack"]
        routing["routing.go\nRouter — built from topology.Config.Routes\nResolve(eventType) → target names"]
    end

    subgraph forwarder["internal/forwarder/"]
        fwd["forwarder.go\nForwarder — concurrent fan-out to\nnamed targets, all-or-nothing result"]
        target["target.go\nTarget — single HTTP POST\n/api/v1/internal/events + x-internal-token"]
    end

    subgraph observability["internal/observability/"]
        metrics["metrics.go\nRegister() — forward_total,\nforward_duration_seconds,\nhandler_duration_seconds"]
    end

    subgraph external["External"]
        yaml["config/topology.yaml\n(queues, targets, routes)"]
        sqs["SQS queues\n(one per topology.yaml entry)"]
        exec["execution_service\nPOST /api/v1/internal/events"]
        def["definition_service\nPOST /api/v1/internal/events"]
    end

    yaml --> topoload
    deps --> topoload
    topoload --> resolve
    resolve --> fwd
    sqs --> queue
    main --> deps
    deps --> queue
    deps --> routing
    main --> health
    queue --> handler
    handler --> routing
    handler --> fwd
    fwd --> target
    target --> exec
    target --> def
    handler --> metrics
    fwd --> metrics
```

**Rule:** `internal/forwarder` is transport-only — it fans a byte slice out
to named targets and reports success/failure per target. It has no opinion
about event types. `internal/topology` owns the queue/target/routing
*catalogue* (loaded from `config/topology.yaml`); `internal/consumer`'s
`Router` just resolves against whatever catalogue it's handed. `handler.go`
is the only place that ties routing and forwarding together.

---

## Routing: why per-event-type, not broadcast

Every event this bridge is known to carry belongs to exactly one downstream
service — never both (see [docs/runbook.md](docs/runbook.md)'s catalogue
table). `config/topology.yaml`'s `routes` map encodes this directly — the
common case (`delegation.started` → execution only,
`department.membership.revoked` → definition only, etc.) sends each event to
exactly the target that needs it, not to a target that will only discard it.
`internal/consumer.Router` (built from that map at startup by `deps.go`) is
a thin lookup — the catalogue itself lives entirely in the YAML file, not in
Go source, so adding a route is a config change, not a code change.

An event type absent from the routing table — a type not yet added to this
catalogue — falls back to broadcasting to every declared target. This is a
forward-compatibility safety net, not the steady-state behavior: both
downstream services already accept an unrecognized type as a `200` no-op, so
a genuinely new event type is never silently dropped while this table is
updated to route it precisely.

## Queues and targets: also config-driven

`config/topology.yaml`'s `queues` list and `targets` list are the only
places either exists — `cmd/event-consumer/deps.go` builds exactly one SQS
consumer per queue entry and one `forwarder.Target` per target entry, at
whatever count the file declares. Adding a fourth downstream service or a
fourth queue is a YAML edit + redeploy; nothing in `internal/` hardcodes a
count of 2 or 3. See [README.md](README.md#configuration) for how queue URLs
and target addresses/tokens are populated differently across local, Docker
Compose, and Kubernetes.

---

## Ack semantics: all-or-nothing across the *resolved* targets

A message is only deleted from its queue once **every target it was routed
to** returns `2xx`. If any of them fails, the message stays visible and SQS
redelivers it — after the queue's configured `maxReceiveCount`, it lands in
that queue's dead-letter queue. There is no retry or circuit-breaker logic
beyond what SQS itself provides.

> Source: [`docs/architecture/mermaid/sequence-ack.mmd`](docs/architecture/mermaid/sequence-ack.mmd)

```mermaid
sequenceDiagram
    participant SNS
    participant SQS as SQS queue
    participant C as consumer.Build (per-queue)
    participant H as handler.MakeHandler
    participant R as consumer.Router
    participant F as forwarder.Forward
    participant T as one resolved target

    SNS->>SQS: publish event
    SQS->>C: long-poll ReceiveMessage
    C->>H: handler(ctx, envelope)
    H->>R: Resolve(envelope.Type)
    R-->>H: [target names]
    H->>F: Forward(ctx, queue, targets, body)
    F->>T: POST /api/v1/internal/events (x-internal-token)
    T-->>F: 2xx
    F-->>H: nil (all resolved targets succeeded)
    H-->>C: nil
    C->>SQS: DeleteMessage (ack)
```

This is safe without any state of its own because both downstream services
maintain their own `processed_event`-style idempotency — a redelivery to a
target that already succeeded costs a cheap no-op, never a duplicate side
effect.

### Partial failure and redelivery

> Source: [`docs/architecture/mermaid/sequence-partial-failure.mmd`](docs/architecture/mermaid/sequence-partial-failure.mmd)

```mermaid
sequenceDiagram
    participant SQS as SQS queue
    participant C as consumer.Build
    participant H as handler.MakeHandler
    participant F as forwarder.Forward
    participant T1 as target A
    participant T2 as target B

    Note over H,F: event routes to 2 targets (unknown type fallback,<br/>or a future multi-target route)

    SQS->>C: ReceiveMessage (attempt 1)
    C->>H: handler(ctx, envelope)
    H->>F: Forward(ctx, queue, [A,B], body)
    par concurrent
        F->>T1: POST /api/v1/internal/events
        T1-->>F: 200
    and
        F->>T2: POST /api/v1/internal/events
        T2-->>F: 500
    end
    F-->>H: error (not all targets succeeded)
    H-->>C: error
    Note over C,SQS: message NOT deleted — stays invisible until\nvisibility timeout, then redelivers

    SQS->>C: ReceiveMessage (attempt 2, after visibility timeout)
    C->>H: handler(ctx, envelope)
    H->>F: Forward(ctx, queue, [A,B], body)
    par concurrent
        F->>T1: POST /api/v1/internal/events
        T1-->>F: 200 (idempotent no-op — already processed)
    and
        F->>T2: POST /api/v1/internal/events
        T2-->>F: 200 (now recovered)
    end
    F-->>H: nil
    H-->>C: nil
    C->>SQS: DeleteMessage (ack)

    Note over SQS: after maxReceiveCount (5) consecutive failures,<br/>SQS's native redrive policy moves the message to that queue's DLQ
```

---

## Dependency rule

`internal/forwarder` and `internal/observability` have no dependency on
`internal/consumer`; `internal/consumer` depends on both. `cmd/event-consumer`
is the only composition root and is the only package that constructs
concrete SQS consumers, HTTP targets, or reads configuration. Nothing in
`internal/` imports `cmd/`.

For local development and testing instructions, see [README.md](README.md).
For operational procedures (alerting, DLQ replay, adding a new event type),
see [docs/runbook.md](docs/runbook.md).
