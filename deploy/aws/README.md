# IRSA wiring for workflow-events-consumer

The service consumes SQS and sends to dead-letter queues; it never publishes to SNS and never touches S3 or Glue. `policy.json` has two statements: consume and read attributes on each queue of `config/topology.yaml`, and send plus read attributes on each queue's dead-letter queue, named `<queue>-dlq`. Adding a queue to the topology adds both ARNs here, and a test fails when the policy and the topology example differ.

## Encrypted queues

Queues with SSE-SQS need no KMS permission. Queues with a customer-managed KMS key need `policy-kms.json` attached as well, with the key ARNs filled in:

- `kms:Decrypt` on the source queues' key, to receive;
- `kms:GenerateDataKey` and `kms:Decrypt` on the dead-letter queues' key, to send a dead-letter.

Both statements are limited to use through SQS (`kms:ViaService`). Each key's own policy must allow the role as well. The source queues are fed by SNS, so their key's policy must also let `sns.amazonaws.com` call `kms:GenerateDataKey` and `kms:Decrypt`; the AWS-managed `alias/aws/sqs` key cannot be given that, so an SNS-fed queue uses SSE-SQS or a customer-managed key. A test fails when `policy-kms.json` loses either statement.

## Setup

1. Create the IAM policy from `policy.json`:

   ```bash
   aws iam create-policy \
     --policy-name workflow-events-consumer-sqs-consume \
     --policy-document file://policy.json
   ```

   With customer-managed queue keys, create `policy-kms.json` the same way (see Encrypted queues).

2. Create an IAM role trusted by your cluster's OIDC provider, attach the
   policy above, and annotate the chart's ServiceAccount with the role ARN:

   ```yaml
   serviceAccount:
     annotations:
       eks.amazonaws.com/role-arn: arn:aws:iam::<account-id>:role/workflow-events-consumer
   ```

3. Verify: attach an ephemeral debug container that has the AWS CLI, since the service image has no shell:
   `kubectl debug -it pod/<pod> --image=amazon/aws-cli --target=event-consumer -- sts get-caller-identity`
   resolves to the expected role. The role must not be able to call `sns:Publish` or any S3 or Glue action.

## Rollback

Removing the IRSA annotation (or the role's policy attachment) immediately
revokes access on the next credential refresh (~15 min, or immediately if
the pod restarts) — no data-plane change needed on this service's side.
