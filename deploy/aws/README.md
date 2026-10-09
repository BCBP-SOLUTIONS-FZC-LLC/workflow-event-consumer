# IRSA wiring for workflow-events-consumer

This service only ever reads from SQS — it never publishes to SNS and never
touches S3/KMS/Glue. `policy.json`'s resource list must be kept in sync with
`config/topology.yaml`'s `queues` list: add a queue there, add its ARN here. A test fails when `policy.json` and the topology example list different queues. The queue-depth metrics also call `sqs:GetQueueAttributes` on each queue's dead-letter queue, so the role needs that permission on those queues too.

## Setup

1. Create the IAM policy from `policy.json`:

   ```bash
   aws iam create-policy \
     --policy-name workflow-events-consumer-sqs-consume \
     --policy-document file://policy.json
   ```

2. Create an IAM role trusted by your cluster's OIDC provider, attach the
   policy above, and annotate the chart's ServiceAccount with the role ARN:

   ```yaml
   serviceAccount:
     annotations:
       eks.amazonaws.com/role-arn: arn:aws:iam::<account-id>:role/workflow-events-consumer
   ```

3. Verify: `kubectl exec` into a running pod and confirm
   `aws sts get-caller-identity` resolves to the expected role — the pod
   should never be able to call `sns:Publish` or any S3/KMS/Glue action.

## Rollback

Removing the IRSA annotation (or the role's policy attachment) immediately
revokes access on the next credential refresh (~15 min, or immediately if
the pod restarts) — no data-plane change needed on this service's side.
