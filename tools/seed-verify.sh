#!/usr/bin/env bash
# Proves the Go seeder rebuilds seeds/ exactly: `estevao db prepare` creates a
# scratch database from db/schema.sql and loads seeds/, `estevao seed export`
# writes it back, and the two must be identical (rows, order, id gaps and
# sequences).
set -euo pipefail
GO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="${PG_URL:-postgres://postgres:postgres@localhost:5432}"
DB=estevao_seed_verify
OUT="$(mktemp -d)"
trap 'rm -rf "$OUT"' EXIT
psql -q "$BASE/postgres" -c "DROP DATABASE IF EXISTS $DB"
cd "$GO_ROOT"
go build -o "$OUT/estevao" ./cmd/estevao
DATABASE_URL="$BASE/$DB" "$OUT/estevao" db prepare -seeds seeds >/dev/null
DATABASE_URL="$BASE/$DB" "$OUT/estevao" seed export -dir "$OUT/seeds"
diff -r seeds "$OUT/seeds"
psql -q "$BASE/postgres" -c "DROP DATABASE $DB"
echo "seed round trip: identical"
