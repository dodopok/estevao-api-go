#!/usr/bin/env bash
# Runs the Go server under test on ${GO_PORT:-3001} with the oracle's
# environment, but its own Redis database so rate-limit budgets and usage
# counters of the two stacks do not interfere during comparisons.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
set -a; source "$DIR/oracle.env"; set +a
export REDIS_URL="${GO_REDIS_URL:-redis://localhost:6379/2}" PORT="${GO_PORT:-3001}"
export FIREBASE_CERTS_URL="${FIREBASE_CERTS_URL:-http://127.0.0.1:${CERTS_PORT:-3999}/certs}"
cd "$DIR/../.."
go build -o /tmp/estevao-go ./cmd/estevao-api
exec /tmp/estevao-go
