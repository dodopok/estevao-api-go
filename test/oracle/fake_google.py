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

Perplexity (POST /perplexity/chat/completions, bearer "px-test-key") answers
one bullet per prayer request of the prompt, unless the first request's
title is one of the PX-* switches below. GET /__perplexity lists the request
bodies received (so the prompts of both stacks are compared); DELETE clears.
"""
import json
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

deleted = []
revenuecat_calls = []
perplexity_bodies = []
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
        if self.path == "/__perplexity":
            with lock:
                return self.reply(200, list(perplexity_bodies))
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
        if self.path == "/__perplexity":
            with lock:
                perplexity_bodies.clear()
            return self.reply(200, {})
        self.reply(404, {})

    def do_POST(self):
        n = int(self.headers.get("Content-Length") or 0)
        data = self.rfile.read(n) if n else b""
        if self.path == "/perplexity/chat/completions":
            if self.headers.get("Authorization") != "Bearer px-test-key":
                return self.reply(401, {"error": "unauthorized"})
            with lock:
                perplexity_bodies.append(data.decode())
            status, body = perplexity_reply(json.loads(data))
            if isinstance(body, str):
                return self.reply_raw(status, body)
            return self.reply(status, body)
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


def perplexity_reply(payload):
    """(status, body) for a chat completion request."""
    content = payload["messages"][1]["content"]
    data = content.split("<prayer_request_data>\n", 1)[1].split("\n</prayer_request_data>", 1)[0]
    items = json.loads(data)
    first = items[0]["title"] if items else ""
    def chat(text):
        return 200, {"id": "fake", "model": payload.get("model"), "choices": [{"index": 0, "message": {"role": "assistant", "content": text}}]}
    if first == "PX-500":
        return 500, {"error": "boom"}
    if first == "PX-429":
        return 429, {"error": "slow down"}
    if first == "PX-400":
        return 400, {"error": "bad request"}
    if first == "PX-badjson":
        return 200, "not json"
    if first == "PX-nochoices":
        return 200, {"choices": "x"}
    if first == "PX-empty":
        return chat("   ")
    if first == "PX-url":
        return chat("\u2022 visit http://example.com")
    if first == "PX-extra":
        return chat("\n".join("\u2022 item %d" % i for i in range(len(items) + 1)))
    if first == "PX-long":
        return chat("\u2022 one two three four five six seven eight nine ten eleven")
    if first == "PX-nobullet":
        return chat("- not a bullet")
    lines = []
    for it in items:
        text = it["title"]
        if it.get("content"):
            text += " \u2014 " + it["content"]
        lines.append("\u2022  " + " ".join(text.split()[:10]))
    return chat("\n\n".join(lines) + "\n")


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(sys.argv[1]) if len(sys.argv) > 1 else 3998), Handler).serve_forever()
