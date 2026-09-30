#!/usr/bin/env bash
# Records the oracle corpus: the Rails app's normalized answer to every
# request and scenario of the differential suites, and the database they
# ran against, so the Go server can be verified after the Rails app is gone
# (test/corpus/replay.sh). Run it where the oracle runs (docs/TESTING.md).
#
#   test/corpus/record.sh [OUT_DIR]      # default: test/corpus/recording
#
# OUT_DIR holds database.dump (pg_dump custom format), meta.json and one
# <suite>.jsonl.gz per suite. The dump has the full Bibles the oracle
# imported, many of them copyrighted: keep OUT_DIR out of git.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$DIR/../.."
OUT="$(realpath -m "${1:-$DIR/recording}")"
LOG="${LOG_DIR:-/tmp}"

# A replay runs faster than the recording, so its clock lags the
# recording's by up to the recording's length; a midnight inside that
# window would move the date of some requests. Days turn at 00:00 UTC and at
# 00:00 in America/Sao_Paulo (03:00 UTC).
hour=$(date -u +%H)
case "$hour" in
  23|00|02|03) echo "record.sh: too close to a day boundary (UTC hour $hour); record between 04:00 and 22:00 UTC" >&2; exit 2 ;;
esac

LOG_DIR="$LOG" "$ROOT/test/oracle/restart.sh"
set -a; source "$ROOT/test/oracle/oracle.env"; set +a
rm -rf "$OUT"; mkdir -p "$OUT"
RECORDED_AT=$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)
pg_dump -Fc --no-owner --no-privileges "$DATABASE_URL" > "$OUT/database.dump"
cd "$ROOT"
status=0
RAILS_RUNNER="${RAILS_RUNNER:?set RAILS_RUNNER (docs/TESTING.md)}" \
  go run ./cmd/difftest -suite all -show 40 -record "$OUT" -recorded-at "$RECORDED_AT" | tee "$OUT/record.log" || status=$?
python3 - "$OUT/meta.json" "$(git -C "$ROOT" rev-parse HEAD)" "$(git -C "${RAILS_ROOT:-$ROOT/../estevao-api}" rev-parse HEAD 2>/dev/null || echo unknown)" <<'PY'
import json, sys
p, go, rails = sys.argv[1:]
m = json.load(open(p)); m["go_commit"] = go; m["rails_commit"] = rails
json.dump(m, open(p, "w"), indent=2); open(p, "a").write("\n")
PY
if [ "$status" -ne 0 ]; then
  echo "record.sh: Rails and Go differed while recording; the corpus holds the Rails answers (see $OUT/record.log)" >&2
fi
du -sh "$OUT"
exit "$status"
