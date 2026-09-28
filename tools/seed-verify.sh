#!/usr/bin/env bash
# Proves the Go seeder rebuilds seeds/ exactly: an empty scratch database
# from db/schema.sql, `estevao-seed load`, `estevao-seed export`, and a diff
# against seeds/ (rows, order, id gaps and sequences).
set -euo pipefail
GO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="${PG_URL:-postgres://postgres:postgres@localhost:5432}"
DB=estevao_seed_verify
OUT="$(mktemp -d)"
trap 'rm -rf "$OUT"' EXIT
psql -q "$BASE/postgres" -c "DROP DATABASE IF EXISTS $DB" -c "CREATE DATABASE $DB"
psql -q -v ON_ERROR_STOP=1 "$BASE/$DB" -f "$GO_ROOT/db/schema.sql" >/dev/null
cd "$GO_ROOT"
DATABASE_URL="$BASE/$DB" go run ./cmd/estevao-seed load -dir seeds >/dev/null
DATABASE_URL="$BASE/$DB" go run ./cmd/estevao-seed export -dir "$OUT/seeds"
diff -r seeds "$OUT/seeds"
psql -q "$BASE/postgres" -c "DROP DATABASE $DB"
echo "seed round trip: identical"
