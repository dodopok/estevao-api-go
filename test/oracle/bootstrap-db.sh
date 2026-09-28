#!/usr/bin/env bash
# Builds the local oracle database from scratch, the same way for every
# machine: schema and seeds from the Rails repository (development
# environment, as bin/setup does), then the Bible translations. Only for a
# local, disposable PostgreSQL; it never touches a shared database.
#
#   test/oracle/bootstrap-db.sh        # ~20 minutes (seeds + Bible import)
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
export RAILS_ROOT="${RAILS_ROOT:-$DIR/../../../estevao-api}"
set -a; source "$DIR/oracle.env"; set +a
case "$DATABASE_URL" in
  *@localhost*|*@127.0.0.1*) ;;
  *) echo "refusing to bootstrap a non-local database: $DATABASE_URL" >&2; exit 1 ;;
esac
export RBENV_VERSION="${RBENV_VERSION:-3.2.3}" PATH="/opt/rbenv/shims:/opt/rbenv/bin:$PATH"
export BUNDLE_PATH="${ORACLE_BUNDLE_PATH:-/tmp/bundle32}" LANG=C.UTF-8
cd "$RAILS_ROOT"
RAILS_ENV=development SECRET_KEY_BASE=dev bundle exec bin/rails db:create db:schema:load
RAILS_ENV=development SECRET_KEY_BASE=dev bundle exec bin/rails db:seed
bundle exec bin/rails bible:setup_all
