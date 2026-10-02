#!/usr/bin/env bash
set -euo pipefail
set +x

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_root"
test_project="flowforge-test-$$"
export POSTGRES_PORT="${TEST_POSTGRES_PORT:-5433}"
export NATS_PORT="${TEST_NATS_PORT:-4223}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

for port in "$POSTGRES_PORT" "$NATS_PORT"; do
  if ! [[ "$port" =~ ^[0-9]{1,5}$ ]] || (( 10#$port < 1 || 10#$port > 65535 )); then
    printf '%s\n' 'Test ports must be integers between 1 and 65535.' >&2
    exit 1
  fi
done

cleanup() {
  docker compose -p "$test_project" down -v --remove-orphans
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

docker compose -p "$test_project" up -d --wait postgres nats
database_password="$(docker compose -p "$test_project" exec -T postgres cat /run/db-secrets/postgres_password)"
nats_token="$(docker compose -p "$test_project" exec -T nats cat /run/nats-secrets/nats_token)"
export TEST_DATABASE_URL="postgres://flowforge:$database_password@127.0.0.1:$POSTGRES_PORT/flowforge?sslmode=disable"
export TEST_NATS_URL="nats://127.0.0.1:$NATS_PORT"
export TEST_NATS_TOKEN="$nats_token"
"${GO:-go}" test -race -tags=integration ./cmd/... ./internal/...
