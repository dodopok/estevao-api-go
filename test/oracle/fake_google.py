#!/usr/bin/env python3
"""Fake Google OAuth2 token endpoint, Identity Toolkit accounts:delete and
the RevenueCat subscribers API.

The oracle (patched in oracle.ru) and the Go server (FIREBASE_OAUTH_TOKEN_URL,
FIREBASE_IDENTITY_TOOLKIT_URL) both call it. A uid starting with
"difftest-reject" is refused with 400, like an unknown Firebase user.
GET /__deleted lists the uids deleted so far; DELETE /__deleted clears it.

RevenueCat (GET /revenuecat/v1/subscribers/<id>, bearer "rc-test-key")
answers per subscriber id from REVENUECAT below. GET /__revenuecat lists the
ids requested so far; DELETE /__revenuecat clears the list.
"""
import json
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

deleted = []
revenuecat_calls = []
lock = threading.Lock()


def entitlement(expires, product="ordo_plus_monthly", will_renew=True):
    return {"expires_date": expires, "product_identifier": product, "will_renew": will_renew,
            "purchase_date": "2026-01-01T00:00:00Z"}


def subscriber(entitlements):
    return {"request_date": "2026-09-24T12:00:00Z", "subscriber": {"entitlements": entitlements}}


# subscriber id -> (status, body); a str body is sent verbatim.
REVENUECAT = {
    "rc_active": (200, subscriber({"premium": entitlement("2099-01-01T00:00:00Z")})),
    "rc_later": (200, subscriber({"premium": entitlement("2099-06-01T12:30:00.5Z", will_renew="no")})),
    "rc_earlier": (200, subscriber({"premium": entitlement("2098-01-01T00:00:00Z", will_renew=None)})),
    "rc_lifetime": (200, subscriber({"Ordo +": {"expires_date": None, "product_identifier": "lifetime"}})),
    "rc_expired": (200, subscriber({"premium": entitlement("2020-01-01T00:00:00Z")})),
    "rc_multi": (200, subscriber({"old": entitlement("2021-01-01T00:00:00Z"),
                                  "pro": entitlement("2099-03-01T10:00:00+03:00", product=7, will_renew="false")})),
    "rc_zoneless": (200, subscriber({"premium": entitlement("2099-01-01 10:00:00")})),
    "rc_none": (200, subscriber({})),
    "rc_nosub": (200, {"request_date": "2026-09-24T12:00:00Z"}),
    "rc_nullents": (200, {"subscriber": {"entitlements": None}}),
    "rc_str_ent": (200, subscriber({"x": "abc"})),
    "rc_str_ent_key": (200, subscriber({"x": "has expires_date inside"})),
    "rc_bad_date": (200, subscriber({"premium": entitlement("garbage")})),
    "rc_int_date": (200, subscriber({"premium": entitlement(5)})),
    "rc_arr_sub": (200, {"subscriber": [1]}),
    "rc_str_sub": (200, {"subscriber": "abc"}),
    "rc_arr_ents": (200, {"subscriber": {"entitlements": [1, 2]}}),
    "rc_null": (200, "null"),
    "rc_badjson": (200, "not json"),
    "rc_404": (404, {"code": 7259, "message": "Subscriber not found"}),
    "rc_500": (500, {"message": "boom"}),
    "rc_429": (429, {"message": "slow down"}),
}


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

    def reply_raw(self, status, body):
        body = body.encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/__deleted":
            with lock:
                return self.reply(200, list(deleted))
        if self.path == "/__revenuecat":
            with lock:
                return self.reply(200, list(revenuecat_calls))
        prefix = "/revenuecat/v1/subscribers/"
        if self.path.startswith(prefix):
            if self.headers.get("Authorization") != "Bearer rc-test-key":
                return self.reply(401, {"message": "Invalid API key"})
            sid = self.path[len(prefix):]
            with lock:
                revenuecat_calls.append(sid)
            status, body = REVENUECAT.get(sid, (404, {"message": "Subscriber not found"}))
            if isinstance(body, str):
                return self.reply_raw(status, body)
            return self.reply(status, body)
        self.reply(404, {})

    def do_DELETE(self):
        if self.path == "/__deleted":
            with lock:
                deleted.clear()
            return self.reply(200, {})
        if self.path == "/__revenuecat":
            with lock:
                revenuecat_calls.clear()
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
