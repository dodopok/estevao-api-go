#!/usr/bin/env bash
# Runs the Go server under test on ${GO_PORT:-3001} with the oracle's
# environment, but its own Redis database so rate-limit budgets and usage
# counters of the two stacks do not interfere during comparisons.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
set -a; source "$DIR/oracle.env"; set +a
# GO_DATABASE_URL points the Go stack elsewhere (test/corpus/replay.sh).
export DATABASE_URL="${GO_DATABASE_URL:-$DATABASE_URL}"
export REDIS_URL="${GO_REDIS_URL:-redis://localhost:6379/2}" PORT="${GO_PORT:-3001}"
# The playground proxy calls APP_HOST (Rails' default is its own
# http://localhost:3000); PLAYGROUND_PROXY_BASE_URL points Go's at itself
# while APP_HOST, which also builds audio and avatar URLs, stays unset on
# both stacks.
export PLAYGROUND_PROXY_BASE_URL="http://localhost:${PORT}"
export FIREBASE_CERTS_URL="${FIREBASE_CERTS_URL:-http://127.0.0.1:${CERTS_PORT:-3999}/certs}"
cd "$DIR/../.."
go build -o /tmp/estevao-go ./cmd/estevao-api
go build -o /tmp/estevao-worker ./cmd/estevao-worker
exec /tmp/estevao-go
