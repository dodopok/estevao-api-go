#!/usr/bin/env python3
"""Fake Google OAuth2 token endpoint and Identity Toolkit accounts:delete.

The oracle (patched in oracle.ru) and the Go server (FIREBASE_OAUTH_TOKEN_URL,
FIREBASE_IDENTITY_TOOLKIT_URL) both call it. A uid starting with
"difftest-reject" is refused with 400, like an unknown Firebase user.
GET /__deleted lists the uids deleted so far; DELETE /__deleted clears it.
"""
import json
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

deleted = []
lock = threading.Lock()


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *args):
        pass

    def reply(self, status, obj):
        body = json.dumps(obj).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/__deleted":
            with lock:
                return self.reply(200, list(deleted))
        self.reply(404, {})

    def do_DELETE(self):
        if self.path == "/__deleted":
            with lock:
                deleted.clear()
            return self.reply(200, {})
        self.reply(404, {})

    def do_POST(self):
        n = int(self.headers.get("Content-Length") or 0)
        data = self.rfile.read(n) if n else b""
        if self.path == "/token":
            if b"assertion=" not in data:
                return self.reply(400, {"error": "invalid_grant"})
            return self.reply(200, {"access_token": "fake-admin-token", "expires_in": 3600, "token_type": "Bearer"})
        if self.path.endswith("/accounts:delete"):
            if self.headers.get("Authorization") != "Bearer fake-admin-token":
                return self.reply(401, {"error": {"message": "UNAUTHENTICATED"}})
            uid = json.loads(data or b"{}").get("localId", "")
            if uid.startswith("difftest-reject"):
                return self.reply(400, {"error": {"message": "USER_NOT_FOUND"}})
            with lock:
                deleted.append(uid)
            return self.reply(200, {"kind": "identitytoolkit#DeleteAccountResponse"})
        self.reply(404, {})


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(sys.argv[1]) if len(sys.argv) > 1 else 3998), Handler).serve_forever()
