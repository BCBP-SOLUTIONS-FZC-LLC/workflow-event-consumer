#!/usr/bin/env bash
# Fails unless the given image, run with no environment set, exits with code 1
# and reports a configuration error (APP_ENV is required, so the configuration
# load fails fast). Any other exit code, such as 125-127 for a missing image or
# entrypoint, or a crash, fails the check, as does a run that outlives the
# timeout. Usage:
#   docker-negative-path-check.sh <image:tag>
set -euo pipefail

image="$1"

set +e
output=$(timeout 60 docker run --rm "$image" 2>&1)
exit_code=$?
set -e

echo "$output"

if [ "$exit_code" -ne 1 ]; then
  echo "expected exit code 1 with no environment set (APP_ENV required), got $exit_code"
  exit 1
fi
if ! grep -q "config error" <<<"$output"; then
  echo "exit code 1, but the output does not report a configuration error"
  exit 1
fi
echo "container correctly exited 1 on missing required configuration"
