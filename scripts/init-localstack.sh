#!/usr/bin/env bash
# LocalStack init-hook (mounted at /etc/localstack/init/ready.d/) — provisions
# the same 2 topic -> queue(+DLQ) triples used in config/topology.local.yaml
# and the integration test fixture (test/fixtures/localstack_snssqs.go).
set -euo pipefail

REGION=us-east-1

create_topology() {
  local topic_name="$1" queue_name="$2" dlq_name="$3"

  awslocal sqs create-queue --queue-name "$dlq_name" --region "$REGION" >/dev/null
  local dlq_arn
  dlq_arn=$(awslocal sqs get-queue-attributes \
    --queue-url "http://localhost:4566/000000000000/$dlq_name" \
    --attribute-names QueueArn --region "$REGION" \
    --query 'Attributes.QueueArn' --output text)

  # --attributes takes a JSON object of Attribute Name -> String; RedrivePolicy's
  # own value is itself JSON, so its quotes must be escaped to embed it as a
  # string inside the outer JSON — not passed via the Key=Value shorthand
  # (which breaks on the embedded commas).
  local redrive_policy redrive_policy_escaped queue_attrs
  redrive_policy=$(printf '{"deadLetterTargetArn":"%s","maxReceiveCount":"5"}' "$dlq_arn")
  redrive_policy_escaped=$(printf '%s' "$redrive_policy" | sed 's/"/\\"/g')
  queue_attrs=$(printf '{"RedrivePolicy":"%s"}' "$redrive_policy_escaped")

  awslocal sqs create-queue --queue-name "$queue_name" --region "$REGION" \
    --attributes "$queue_attrs" >/dev/null
  local queue_arn
  queue_arn=$(awslocal sqs get-queue-attributes \
    --queue-url "http://localhost:4566/000000000000/$queue_name" \
    --attribute-names QueueArn --region "$REGION" \
    --query 'Attributes.QueueArn' --output text)

  local topic_arn
  topic_arn=$(awslocal sns create-topic --name "$topic_name" --region "$REGION" \
    --query 'TopicArn' --output text)

  awslocal sns subscribe \
    --topic-arn "$topic_arn" \
    --protocol sqs \
    --notification-endpoint "$queue_arn" \
    --attributes RawMessageDelivery=true \
    --region "$REGION" >/dev/null

  echo "provisioned $topic_name -> $queue_name (+ $dlq_name)"
}

create_topology iam-membership-events membership-workflow-q membership-workflow-q-dlq
create_topology iam-user-events user-wf-q user-wf-q-dlq

echo "LocalStack topology ready."
