#!/usr/bin/env bash
# Verifies the Go server against a recorded corpus, with no Rails app:
# restores the recorded database into a scratch one, starts the fakes and
# the Go server on a test clock set to the recording's instant, and compares
# every answer (test/corpus/record.sh, docs/TESTING.md).
#
#   test/corpus/replay.sh [CORPUS_DIR] [difftest flags, e.g. -suite calendar_day]
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$DIR/../.."
CORPUS="$(realpath "${1:-$DIR/recording}")"; shift || true
LOG="${LOG_DIR:-/tmp}"
PG="${PG_URL:-postgres://postgres:postgres@localhost:5432}"
DB=estevao_replay
PORT="${REPLAY_PORT:-3002}"
REDIS="${REPLAY_REDIS_URL:-redis://localhost:6379/3}"
[ -f "$CORPUS/database.dump" ] && [ -f "$CORPUS/meta.json" ] || { echo "replay.sh: $CORPUS is not a recorded corpus" >&2; exit 2; }
RECORDED_AT=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["recorded_at"])' "$CORPUS/meta.json")

psql -q "$PG/postgres" -c "DROP DATABASE IF EXISTS $DB WITH (FORCE)" -c "CREATE DATABASE $DB"
pg_restore --no-owner --no-privileges -d "$PG/$DB" "$CORPUS/database.dump"
psql -q "$PG/$DB" -c "ANALYZE"
redis-cli -u "$REDIS" flushdb >/dev/null

# The fakes and the certificate server, unless an oracle already runs them.
up() { (echo > "/dev/tcp/127.0.0.1/$1") 2>/dev/null; }
up 9000 || nohup python3 "$ROOT/test/oracle/fake_s3.py" 9000 > "$LOG/fake_s3.log" 2>&1 &
up 3998 || nohup python3 "$ROOT/test/oracle/fake_google.py" 3998 > "$LOG/fake_google.log" 2>&1 &
up 3999 || (cd "$ROOT/test/oracle/certs-www" && nohup python3 -m http.server 3999 --bind 127.0.0.1 > "$LOG/certs.log" 2>&1 &)

export ESTEVAO_TEST_CLOCK="$RECORDED_AT" GO_DATABASE_URL="$PG/$DB" GO_PORT="$PORT" GO_REDIS_URL="$REDIS"
export GO_BIN=/tmp/estevao-replay GO_WORKER_BIN=/tmp/estevao-replay-worker
(cd "$ROOT" && go build -o "$GO_BIN" ./cmd/estevao-api && go build -o "$GO_WORKER_BIN" ./cmd/estevao-worker)
set -a; source "$ROOT/test/oracle/oracle.env"; set +a
export DATABASE_URL="$PG/$DB" REDIS_URL="$REDIS" PORT="$PORT"
export PLAYGROUND_PROXY_BASE_URL="http://localhost:$PORT" FIREBASE_CERTS_URL="http://127.0.0.1:3999/certs"
"$GO_BIN" > "$LOG/replay-go.log" 2>&1 &
server=$!
trap 'kill $server 2>/dev/null || true' EXIT
for i in $(seq 1 60); do curl -s -o /dev/null "localhost:$PORT/up" && break; sleep 1; done

cd "$ROOT"
go run ./cmd/difftest -corpus "$CORPUS" -go "http://localhost:$PORT" -go-redis "$REDIS" -show 40 "$@"
