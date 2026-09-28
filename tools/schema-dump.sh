#!/usr/bin/env bash
# Regenerates db/schema.sql: the Rails schema (db:schema:load into a scratch
# local database) dumped with pg_dump, plus the applied migration versions.
#   tools/schema-dump.sh            # RAILS_ROOT defaults to ../estevao-api
set -euo pipefail
GO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RAILS_ROOT="${RAILS_ROOT:-$GO_ROOT/../estevao-api}"
SCRATCH=estevao_schema_dump
URL="postgres://postgres:postgres@localhost:5432/$SCRATCH"
export RBENV_VERSION="${RBENV_VERSION:-3.2.3}" PATH="/opt/rbenv/shims:/opt/rbenv/bin:$PATH"
export BUNDLE_PATH="${BUNDLE_PATH:-/tmp/bundle32}" LANG=C.UTF-8
(cd "$RAILS_ROOT" && DATABASE_URL="$URL" RAILS_ENV=development SECRET_KEY_BASE=dev \
  bundle exec bin/rails db:drop db:create db:schema:load >/dev/null)
{
  echo "-- Schema of the Rails application's database (db/schema.rb), dumped by"
  echo "-- tools/schema-dump.sh. The Rails migrations own the schema; this snapshot"
  echo "-- only creates new local databases for the Go server, its tests and its seeder."
  echo
  pg_dump --schema-only --no-owner --no-privileges --no-comments "$URL" |
    grep -v '^-- Dumped\|^\\restrict\|^\\unrestrict'
  echo
  echo "SET search_path = public;"
  echo "INSERT INTO schema_migrations (version) VALUES"
  psql "$URL" -Atc "SELECT string_agg('(''' || version || ''')', E',\n' ORDER BY version) FROM schema_migrations"
  echo ";"
} > "$GO_ROOT/db/schema.sql"
dropdb --if-exists -h localhost -U postgres "$SCRATCH" 2>/dev/null || psql postgres://postgres:postgres@localhost:5432/postgres -qc "DROP DATABASE IF EXISTS $SCRATCH"
echo "wrote db/schema.sql"
