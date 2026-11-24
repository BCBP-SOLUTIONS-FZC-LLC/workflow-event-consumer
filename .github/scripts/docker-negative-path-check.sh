#!/usr/bin/env bash
# Fails unless the given image exits non-zero when run with no environment set
# (APP_ENV and the AWS settings are required, so the configuration load fails
# fast). Usage:
#   docker-negative-path-check.sh <image:tag>
set -euo pipefail

image="$1"

set +e
docker run --rm "$image"
exit_code=$?
set -e

if [ "$exit_code" -eq 0 ]; then
  echo "expected a non-zero exit with no environment set (APP_ENV required), got 0"
  exit 1
fi
echo "container correctly exited $exit_code on missing required configuration"
