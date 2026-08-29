# Runbook — workflow-events-consumer

## Event routing catalogue

[`config/topology.yaml`](../config/topology.example.yaml)'s `routes` map is
the single source of truth for which downstream service a given event type
is forwarded to — the same file also declares every queue and every
downstream target (see [ARCHITECTURE.md](../ARCHITECTURE.md#queues-and-targets-also-config-driven)).
Keep `routes` in sync with execution_service's and definition_service's own
event catalogues — a type absent here falls back to broadcasting to every
declared target, which is a safety net, not a substitute for updating this
table when a new event type is added upstream.

| Event type | Target |
| --- | --- |
| `delegation.started` | execution |
| `delegation.ended` | execution |
| `tenant.state.changed` | execution |
| `user.deleted` | execution |
| `user.availability.changed` | execution |
| `department.membership.revoked` | definition |
| *(anything else)* | broadcast to all targets |

## Type-scoped forwarding subpaths (execution_service only)

Forwarding to the `execution` target no longer always POSTs to
`/internal/events`. `internal/forwarder/category.go`'s `category` map assigns
each of execution_service's 5 routed event types to one of 3 categories, and
`Target.forward` builds `/events/<category>` for anything in that map:

| Event type | Category / subpath |
| --- | --- |
| `delegation.started`, `delegation.ended` | `delegation` → `/events/delegation` |
| `user.deleted`, `user.availability.changed` | `user-profile` → `/events/user-profile` |
| `tenant.state.changed` | `tenant` → `/events/tenant` |

Every other target (`definition`, `workflow-stream-gateway`) and any event
type absent from this map still gets the legacy `/internal/events` path —
execution_service keeps that route registered as an unconditional catch-all,
so an unmapped type is never dropped. **This map is Go code, not
`topology.yaml`** — deliberately not config-driven, because a mismatched edit
here (a category with no matching subpath registered on the execution_service
side) would 404 instead of falling back safely. Adding or changing a category
means updating both this map and execution_service's
`RegisterInternalEventsRoutes` in the same change.

## Alerting signals

- `workflow_events_consumer_forward_total{target,result="error"}` sustained
  at a nonzero rate — the named downstream service is failing or unreachable.
  Messages for that target's queue will keep redelivering until it recovers
  or the queue's `maxReceiveCount` is exhausted.
- `workflow_events_consumer_handler_duration_seconds{queue}` p99 climbing —
  either a downstream target is slow, or queue depth is outpacing
  `CONSUMER_CONCURRENCY`.
- Corroborate against the downstream service's own
  `internal_events_ingest_total{event_type,result}` metric — a spike in
  `bad_payload`/`decode_failed` there points at a producer-side contract
  break, not this bridge.

## A message is stuck redelivering

1. Check `workflow_events_consumer_forward_total` to identify which target is
   failing.
2. Check that target's own health/logs — a `5xx` there is expected to cause
   redelivery; that's by design (see [ARCHITECTURE.md](../ARCHITECTURE.md)'s
   ack-semantics section).
3. If the target is healthy but consistently returns `4xx` for one message,
   that message will exhaust `maxReceiveCount` (5) and land in that queue's
   dead-letter queue — this bridge has no way to short-circuit a `4xx`
   into an immediate DLQ move, only SQS's own redrive policy does that.

## Replaying a dead-lettered message

This bridge does not own the queues or their redrive policies (infra-owned).
Once the underlying cause is fixed, redrive from the DLQ back to the source
queue using the AWS CLI or console — this bridge picks it up exactly like
any other message on its next poll.

```bash
aws sqs start-message-move-task \
  --source-arn <dlq-arn> \
  --destination-arn <source-queue-arn>
```

## Adding a new event type

1. Confirm which downstream service(s) actually need it — check that
   service's own `POST /internal/events` handler and event catalogue.
2. Add an entry to `routes` in `config/topology.yaml` (and
   `config/topology.example.yaml`/`config/topology.local.yaml` for the
   checked-in reference/dev copies). No Go code change or rebuild needed —
   redeploy with the updated file.
3. If it changes cross-service behavior meaningfully, add a LocalStack
   integration test case (see `test/integration/localstack_flow_test.go`
   for the pattern) and update its `testRoutes` fixture map.
4. Update the table above.
5. If the new type routes to `execution` and belongs to one of the 3
   categories above (or a new one), add it to `internal/forwarder/category.go`'s
   `category` map **and** register/extend the matching subpath in
   execution_service's `RegisterInternalEventsRoutes` — this one step is a
   real Go code change on both sides, unlike step 2 above.

## Adding a new queue or downstream target

Same file, same principle: add an entry to `queues` or `targets` in
`config/topology.yaml`. A new target also needs its token's environment
variable set (named by that entry's `tokenEnv`) wherever secrets are
injected for the environment (see
[README.md](../README.md#configuration)). No Go code change.
