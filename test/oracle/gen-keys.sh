#!/usr/bin/env bash
# Generates the throwaway RSA key/certificate that the oracle and the Go
# server both trust as the Firebase signing key (kid "test-kid"), plus the
# certificate JSON the Go server fetches from FIREBASE_CERTS_URL in tests.
set -euo pipefail
cd "$(dirname "$0")"
if [ ! -f test_firebase_key.pem ]; then
  openssl req -x509 -newkey rsa:2048 -keyout test_firebase_key.pem -out test_firebase_cert.pem \
    -days 36500 -nodes -subj "/CN=estevao-test-firebase" 2>/dev/null
fi
mkdir -p certs-www
python3 -c 'import json;print(json.dumps({"test-kid": open("test_firebase_cert.pem").read()}))' > certs-www/certs
