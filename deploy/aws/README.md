# IRSA wiring for workflow-events-consumer

The service consumes SQS and sends to dead-letter queues; it never publishes to SNS and never touches S3, Glue or KMS. `policy.json` has two statements: consume and read attributes on each queue of `config/topology.yaml`, and send plus read attributes on each queue's dead-letter queue, named `<queue>-dlq`. Adding a queue to the topology adds both ARNs here, and a test fails when the policy and the topology example differ. A dead-letter queue encrypted with a customer-managed KMS key additionally needs `kms:GenerateDataKey` and `kms:Decrypt` on that key.

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
   should never be able to call `sns:Publish` or any S3/Glue action.

## Rollback

Removing the IRSA annotation (or the role's policy attachment) immediately
revokes access on the next credential refresh (~15 min, or immediately if
the pod restarts) — no data-plane change needed on this service's side.
