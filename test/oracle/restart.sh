#!/usr/bin/env bash
# Restarts the certificate server, the Rails oracle and the Go server.
set -uo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
LOG="${LOG_DIR:-/tmp}"
for pat in "puma 7" "estevao-go" "http.server 3999"; do
  for pid in $(ps -eo pid,args | grep "$pat" | grep -v grep | awk '{print $1}'); do kill "$pid" 2>/dev/null; done
done
sleep 2
"$DIR/gen-keys.sh"
(cd "$DIR/certs-www" && nohup python3 -m http.server 3999 --bind 127.0.0.1 > "$LOG/certs.log" 2>&1 &)
nohup "$DIR/run-oracle.sh" > "$LOG/oracle.log" 2>&1 &
nohup "$DIR/run-go.sh" > "$LOG/go.log" 2>&1 &
for i in $(seq 1 60); do
  if curl -s -o /dev/null localhost:${ORACLE_PORT:-3000}/up && curl -s -o /dev/null localhost:${GO_PORT:-3001}/up; then echo "ready"; exit 0; fi
  sleep 1
done
echo "servers did not start"; tail -5 "$LOG/oracle.log" "$LOG/go.log"; exit 1
