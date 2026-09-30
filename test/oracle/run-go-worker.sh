#!/usr/bin/env bash
# Runs the Go job worker (built by run-go.sh) with the Go server's
# environment, e.g. `run-go-worker.sh -drain Audio::RecordUserUsageJob`.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
set -a; source "$DIR/oracle.env"; set +a
# GO_DATABASE_URL points the Go stack elsewhere (test/corpus/replay.sh).
export DATABASE_URL="${GO_DATABASE_URL:-$DATABASE_URL}"
export REDIS_URL="${GO_REDIS_URL:-redis://localhost:6379/2}"
export FIREBASE_CERTS_URL="${FIREBASE_CERTS_URL:-http://127.0.0.1:${CERTS_PORT:-3999}/certs}"
exec "${GO_WORKER_BIN:-/tmp/estevao-worker}" "$@"
