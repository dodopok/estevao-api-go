#!/usr/bin/env bash
# Compares the persisted effect of Audio::RecordUserUsageJob (user_audio_usages)
# between the Rails oracle and the Go server for the audio and daily_office
# suites. Each side starts from no usage rows; the oracle's queued jobs are
# performed by test/effects/perform_jobs.rb since it runs no worker.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$DIR/../.."
set -a; source "$ROOT/test/oracle/oracle.env"; set +a
OUT="${OUT_DIR:-$(mktemp -d)}"
RUNNER="${RAILS_RUNNER:?set RAILS_RUNNER to a script that runs bin/rails runner under the oracle env}"
reset() {
  psql "$DATABASE_URL" -q -v ON_ERROR_STOP=1 <<'SQL'
DELETE FROM user_audio_usages WHERE user_id IN (SELECT id FROM users WHERE provider_uid LIKE 'difftest-%');
DELETE FROM solid_queue_jobs WHERE class_name = 'Audio::RecordUserUsageJob' AND finished_at IS NULL;
SQL
}
snapshot() {
  psql "$DATABASE_URL" -tA -v ON_ERROR_STOP=1 -c "
    SELECT json_build_object('user', u.provider_uid, 'type', a.audio_type, 'asset', a.asset_key,
      'count', a.access_count, 'book', a.prayer_book_code, 'office', a.office_type, 'voice', a.voice,
      'clip', c.key, 'text', a.liturgical_text_id, 'track', a.background_track_id,
      'single_use', a.first_used_at = a.last_used_at)
    FROM user_audio_usages a JOIN users u ON u.id = a.user_id LEFT JOIN audio_clips c ON c.id = a.audio_clip_id
    WHERE u.provider_uid LIKE 'difftest-%' ORDER BY u.provider_uid, a.audio_type, a.asset_key" > "$1"
}
cd "$ROOT"
reset
go run ./cmd/difftest -suite audio,daily_office -replay "http://localhost:${ORACLE_PORT:-3000}"
"$RUNNER" "$DIR/perform_jobs.rb" Audio::RecordUserUsageJob | tail -1
snapshot "$OUT/rails.jsonl"
reset
go run ./cmd/difftest -suite audio,daily_office -replay "http://localhost:${GO_PORT:-3001}"
sleep 3
snapshot "$OUT/go.jsonl"
echo "rails $(wc -l < "$OUT/rails.jsonl") rows, go $(wc -l < "$OUT/go.jsonl") rows"
if diff -u "$OUT/rails.jsonl" "$OUT/go.jsonl" > "$OUT/diff.txt"; then echo "user_audio_usages: identical"; else head -40 "$OUT/diff.txt"; exit 1; fi
