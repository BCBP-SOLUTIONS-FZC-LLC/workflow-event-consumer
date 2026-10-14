# Runbook — workflow-events-consumer

## Event routing catalogue

The `routes` map of [`config/topology.example.yaml`](../config/topology.example.yaml) is the single source of truth for which downstream service receives an event type. The same file declares every queue and every downstream target (see [ARCHITECTURE.md](../ARCHITECTURE.md#queues-and-targets-also-config-driven)). An event type absent from the map is sent to every declared target that accepts it (the `execution` target accepts only the types of the category table below); a type mapped to an empty target list is dropped. Each downstream service's own event catalogue and this table change together when an event type is added upstream.

| Event type | Targets |
| --- | --- |
| `DelegationStarted` | execution |
| `DelegationEnded` | execution |
| `TenantStateChanged` | execution |
| `UserDeleted` | execution |
| `UserAvailabilityChanged` | execution |
| `DepartmentMembershipRevoked` | definition |
| `MembershipRevoked` | definition, execution |
| `TenantMembershipsPurged` | execution |
| `WorkflowTaskCreated` | workflow-stream-gateway, execution |
| `WorkflowTaskCompleted` | workflow-stream-gateway |
| `WorkflowTaskDeferred` | workflow-stream-gateway |
| `WorkflowTaskReassigned` | workflow-stream-gateway |
| `WorkflowTaskSlaWarning` | workflow-stream-gateway |
| `WorkflowTaskSlaBreached` | workflow-stream-gateway |
| `WorkflowInstancePaused` | workflow-stream-gateway |
| `WorkflowInstanceResumed` | workflow-stream-gateway |
| `WorkflowInstanceCancelled` | workflow-stream-gateway |
| `WorkflowInstanceTerminated` | workflow-stream-gateway |
| `WorkflowInstanceFinished` | workflow-stream-gateway |
| `DelegationReviewRequested` | dropped |
| `DelegationEscalationRequested` | dropped |
| `TenderAssigneeOverridden` | dropped |
| `DepartmentMembershipGranted` | dropped |
| `DepartmentMembershipLevelChanged` | dropped |
| *(anything else)* | every target that accepts it |

## Type-scoped forwarding subpaths

The `execution` target receives the event types below on a per-category path. Every other target, and every event type absent from this table, receives `/api/v1/internal/events`.

| Event type | Category | Path |
| --- | --- | --- |
| `DelegationStarted` | `delegation` | `/api/v1/internal/events/delegation` |
| `DelegationEnded` | `delegation` | `/api/v1/internal/events/delegation` |
| `UserDeleted` | `user-profile` | `/api/v1/internal/events/user-profile` |
| `UserAvailabilityChanged` | `user-profile` | `/api/v1/internal/events/user-profile` |
| `MembershipRevoked` | `user-profile` | `/api/v1/internal/events/user-profile` |
| `TenantStateChanged` | `tenant` | `/api/v1/internal/events/tenant` |
| `TenantMembershipsPurged` | `tenant` | `/api/v1/internal/events/tenant` |
| `WorkflowTaskCreated` | `workflow-task` | `/api/v1/internal/events/workflow-task` |

The category map lives in code, not in the topology file: a category with no matching route registered on the Execution Service answers 404, so adding or changing one means changing the map and the Execution Service's internal event routes in the same change.

## Alerting signals

- `workflow_events_consumer_forward_total{target,result="error"}` sustained
  at a nonzero rate — the named downstream service is failing or unreachable.
  Messages for that target's queue will keep redelivering until it recovers
  or the queue's `maxReceiveCount` is exhausted.
- `workflow_events_consumer_handler_duration_seconds{queue}` p99 climbing —
  either a downstream target is slow, or queue depth is outpacing
  `CONSUMER_CONCURRENCY`.
- Corroborate against the downstream service's own
  `internal_events_ingest_total{event_type,result}` metric (`workflow_<service>_internal_events_ingest_total` in services that follow the platform standard) — a spike in
  `bad_payload`/`decode_failed` there points at a producer-side contract
  break, not this bridge.

## The pod is not ready and nothing is consumed

`/readyz` answers 503 after two minutes without a successful SQS receive (`/healthz`, the liveness probe, keeps answering, so a shared SQS outage does not restart the pods). Check the `WorkflowEventsConsumerReceiveFailing` alert and the pod's logs for `receive message failed`: a denied `sqs:ReceiveMessage`, a queue URL that does not exist, or an `AWS_REGION` that does not match the queues. Fix the role, the topology or the region; the pod becomes ready on its own once a receive succeeds. A pod is not ready until its first receive has succeeded. The receive metric has no queue label, so one queue with a wrong URL among healthy ones keeps the pod ready: the `WorkflowEventsConsumerReceiveFailing` alert and the `receive message failed` log line name the failing queue.

## A message is stuck redelivering

A transient failure is retried at 30 seconds, then 1, 2 and 4 minutes, and the message is dead-lettered on the fifth receive: a target outage of about seven minutes is covered, a longer one dead-letters what arrived during it and needs a redrive from the dead-letter queue.

1. Check `workflow_events_consumer_forward_total` to identify which target is
   failing.
2. Check that target's own health/logs — a `5xx` there is expected to cause
   redelivery; that's by design (see [ARCHITECTURE.md](../ARCHITECTURE.md)'s
   ack-semantics section).
3. A `400`, `413` or `422` is a permanent failure: the request is malformed and cannot succeed on any later attempt. When every failing target answered that way, the bridge sends the original message to the queue's dead-letter queue at once, with a reason naming the target and status, and deletes it from the source queue. Look for it in the dead-letter queue and in `platform_dlq_messages_total`. Every other status, including `401`, `403` and `404` (a rotated token or a route not yet deployed), and every network error is transient: the message stays on the queue and is redelivered with backoff until `maxReceiveCount` is exhausted. When one target fails permanently and another transiently, the message is redelivered, because the transient target still needs it; the permanent target receives it again, which its own idempotency absorbs.

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
   service's own `POST /api/v1/internal/events` handler and event catalogue.
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
