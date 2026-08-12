#!/usr/bin/env bash
# Build and run pqpm e2e tests in Docker.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
IMAGE="pqpm-e2e:local"

cd "$ROOT"

echo "Building e2e image..."
docker build -f test/e2e/Dockerfile -t "$IMAGE" .

echo "Running e2e tests..."
# privileged + host cgroup ns help cgroup v2 limit application inside containers
docker run --rm --name pqpm-e2e \
  --privileged \
  --cgroupns=host \
  "$IMAGE"
