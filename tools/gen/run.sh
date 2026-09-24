#!/usr/bin/env bash
# Regenerates the Go tables extracted from the Rails application.
set -euo pipefail
GO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
RAILS_ROOT="${RAILS_ROOT:-$GO_ROOT/../estevao-api}"
set -a; source "$GO_ROOT/test/oracle/oracle.env"; set +a
export LANG=C.UTF-8 BUNDLE_PATH="${BUNDLE_PATH:-/tmp/bundle}"
cd "$RAILS_ROOT"
for f in "$GO_ROOT"/tools/gen/*.rb; do
  bin/rails runner "$f" "$GO_ROOT" 2>&1 | grep '^wrote' || true
done
gofmt -w "$GO_ROOT/internal"
