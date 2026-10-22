#!/usr/bin/env bash
# Smoke test of the built image: the docker-compose stack (LocalStack SNS/SQS,
# two mock targets, the consumer) starts, the consumer becomes ready after
# receiving from every queue, and an event published to SNS reaches the
# target its route names and not the other one.
set -euo pipefail

cd "$(dirname "$0")/.."

: "${GO_PRIVATE_TOKEN:?GO_PRIVATE_TOKEN is required to build the images}"
token_file=$(mktemp "$PWD/.smoke-go-private-token.XXXXXX")
printf '%s' "$GO_PRIVATE_TOKEN" >"$token_file"
export GO_PRIVATE_TOKEN_FILE="$token_file"

cleanup() {
  status=$?
  if [ "$status" -ne 0 ]; then
    docker compose logs --no-color app mock-execution mock-definition | tail -n 80 || true
  fi
  docker compose down -v --remove-orphans >/dev/null 2>&1 || true
  rm -f "$token_file"
  exit "$status"
}
trap cleanup EXIT

docker compose up -d --build --wait

for _ in $(seq 1 60); do
  if curl -fsS http://localhost:8082/readyz >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 2
done
[ "${ready:-0}" = 1 ] || { echo "smoke: the consumer never became ready"; exit 1; }
echo "smoke: the consumer is ready"

id=$(uuidgen | tr '[:upper:]' '[:lower:]')
body=$(printf '{"id":"%s","type":"DepartmentMembershipRevoked","source":"iam","time":"%s","tenant_id":"%s","data":{"user_id":"%s","department_id":"%s","role":"reviewer"}}' \
  "$id" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$(uuidgen)" "$(uuidgen)" "$(uuidgen)")
docker compose exec -T localstack awslocal sns publish --region us-east-1 \
  --topic-arn arn:aws:sns:us-east-1:000000000000:iam-membership-events \
  --message "$body" >/dev/null

for _ in $(seq 1 30); do
  if docker compose logs --no-color mock-definition | grep -q "received event id=$id"; then
    delivered=1
    break
  fi
  sleep 2
done
[ "${delivered:-0}" = 1 ] || { echo "smoke: the definition target never received event $id"; exit 1; }
if docker compose logs --no-color mock-execution | grep -q "received event id=$id"; then
  echo "smoke: event $id reached the execution target, which its route does not name"
  exit 1
fi
echo "smoke: event $id was forwarded to the definition target only"
