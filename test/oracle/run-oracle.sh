#!/usr/bin/env bash
# Boots the Rails app (unmodified, from RAILS_ROOT) as the differential
# oracle on port ${ORACLE_PORT:-3000}, reproducing production's runtime:
#   * Ruby 3.2.3 (the Dockerfile's RUBY_VERSION);
#   * a stable qsort_r (production's glibc 2.36 merge sort) via LD_PRELOAD,
#     because Ruby sorts through the C library and newer glibc is unstable.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
"$DIR/gen-keys.sh"
[ -f "$DIR/stable_qsort.so" ] || gcc -shared -fPIC -O2 -o "$DIR/stable_qsort.so" "$DIR/stable_qsort.c"
export RAILS_ROOT="${RAILS_ROOT:-$DIR/../../../estevao-api}"
set -a; source "$DIR/oracle.env"; set +a
export RBENV_VERSION="${RBENV_VERSION:-3.2.3}" PATH="/opt/rbenv/shims:/opt/rbenv/bin:$PATH"
export BUNDLE_PATH="${ORACLE_BUNDLE_PATH:-/tmp/bundle32}" LANG=C.UTF-8
export LD_PRELOAD="$DIR/stable_qsort.so"
cd "$RAILS_ROOT"
exec bundle exec puma -p "${ORACLE_PORT:-3000}" -t 5:5 -w 0 "$DIR/oracle.ru"
