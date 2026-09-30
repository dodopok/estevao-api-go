#!/usr/bin/env bash
# Fails when db/schema.sql is not the schema the migrations produce: a scratch
# database is prepared (schema.sql, then every pending migration) and dumped
# again; a migration committed without `estevao db dump` shows up as a diff.
set -euo pipefail
GO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="${PG_URL:-postgres://postgres:postgres@localhost:5432}"
DB=estevao_schema_check
OUT="$(mktemp -d)"
trap 'rm -rf "$OUT"' EXIT
psql -q "$BASE/postgres" -c "DROP DATABASE IF EXISTS $DB"
cd "$GO_ROOT"
go build -o "$OUT/estevao" ./cmd/estevao
DATABASE_URL="$BASE/$DB" "$OUT/estevao" db prepare -no-seed
DATABASE_URL="$BASE/$DB" "$OUT/estevao" db dump -o "$OUT/schema.sql" >/dev/null
psql -q "$BASE/postgres" -c "DROP DATABASE $DB"
if ! diff -u db/schema.sql "$OUT/schema.sql"; then
  echo "db/schema.sql is stale: apply the migrations to a local database and run 'estevao db dump'" >&2
  exit 1
fi
echo "db/schema.sql matches the migrations"
