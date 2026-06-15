#!/usr/bin/env bash
# integration_test.sh — Spin up Docker containers, run integration tests, tear down.
# Usage: ./integration_test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.test.yml"

cleanup() {
    local exit_code=$?
    echo "==> Tearing down containers..."
    docker compose -f "$COMPOSE_FILE" down -v 2>/dev/null || true
    exit $exit_code
}
trap cleanup EXIT

echo "==> Starting containers..."
docker compose -f "$COMPOSE_FILE" up -d --wait

echo "==> Running integration tests..."
go test -tags integration -v -count=1 ./integration/ -timeout 5m

echo "==> All integration tests passed."