#!/usr/bin/env bash
# Regenerates seeds/ from the Rails seeds: `rails db:seed` into a scratch
# local database built from db/schema.rb, then `estevao-seed export`.
# Run after a change to the Rails seeds; commit the resulting diff.
#   tools/seed-export.sh             # RAILS_ROOT defaults to ../estevao-api (~12 min)
set -euo pipefail
GO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RAILS_ROOT="${RAILS_ROOT:-$GO_ROOT/../estevao-api}"
URL="postgres://postgres:postgres@localhost:5432/estevao_seed_export"
export RBENV_VERSION="${RBENV_VERSION:-3.2.3}" PATH="/opt/rbenv/shims:/opt/rbenv/bin:$PATH"
export BUNDLE_PATH="${BUNDLE_PATH:-/tmp/bundle32}" LANG=C.UTF-8
(cd "$RAILS_ROOT" && DATABASE_URL="$URL" RAILS_ENV=development SECRET_KEY_BASE=dev \
  bundle exec bin/rails db:drop db:create db:schema:load db:seed >/dev/null)
cd "$GO_ROOT"
DATABASE_URL="$URL" go run ./cmd/estevao-seed export -dir seeds
echo "wrote seeds/ (verify with tools/seed-verify.sh)"
