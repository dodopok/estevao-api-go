#!/usr/bin/env python3
"""In-memory S3 stand-in for the differential tests.

Both the oracle (aws-sdk-s3 through Active Storage) and the Go server talk to
it at AVATAR_BUCKET_ENDPOINT with path-style addressing. It stores objects in
memory, ignores authentication, and implements the calls Active Storage and
the Go port make: PutObject, GetObject, HeadObject, DeleteObject,
ListObjectsV2 and DeleteObjects. GET /__objects lists the stored keys (for
effect snapshots); DELETE /__objects clears the store.
"""
import hashlib
import json
import re
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, unquote, urlsplit
from xml.sax.saxutils import escape, unescape

store = {}
lock = threading.Lock()


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *args):
        pass

    def parts(self):
        u = urlsplit(self.path)
        path = unquote(u.path)
        bucket, _, key = path.lstrip("/").partition("/")
        return bucket, key, parse_qs(u.query, keep_blank_values=True)

    def body(self):
        n = int(self.headers.get("Content-Length") or 0)
        return self.rfile.read(n) if n else b""

    def reply(self, status, body=b"", headers=None):
        self.send_response(status)
        for k, v in (headers or {}).items():
            self.send_header(k, v)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)

    def not_found(self):
        self.reply(404, b'<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>',
                   {"Content-Type": "application/xml"})

    def do_GET(self):
        bucket, key, q = self.parts()
        if bucket == "__objects" and "summary" in q:
            # Key-independent view: (size, type, md5) of every object.
            with lock:
                summary = sorted([len(v["body"]), v["type"], v["md5"]] for v in store.values())
            return self.reply(200, json.dumps(summary).encode(), {"Content-Type": "application/json"})
        if bucket == "__objects":
            with lock:
                listing = {"/".join(k): {"size": len(v["body"]), "type": v["type"], "md5": v["md5"]} for k, v in sorted(store.items())}
            return self.reply(200, json.dumps(listing).encode(), {"Content-Type": "application/json"})
        if not key and "list-type" in q:
            prefix = q.get("prefix", [""])[0]
            with lock:
                keys = sorted(k for (b, k) in store if b == bucket and k.startswith(prefix))
            items = "".join(f"<Contents><Key>{escape(k)}</Key><Size>{len(store[(bucket, k)]['body'])}</Size></Contents>" for k in keys)
            xml = (f'<?xml version="1.0" encoding="UTF-8"?><ListBucketResult><Name>{bucket}</Name><Prefix>{escape(prefix)}</Prefix>'
                   f"<KeyCount>{len(keys)}</KeyCount><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>{items}</ListBucketResult>")
            return self.reply(200, xml.encode(), {"Content-Type": "application/xml"})
        with lock:
            obj = store.get((bucket, key))
        if obj is None:
            return self.not_found()
        body = obj["body"]
        headers = {"Content-Type": obj["type"], "ETag": f'"{obj["md5"]}"'}
        rng = self.headers.get("Range")
        if rng:
            m = re.match(r"bytes=(\d+)-(\d*)", rng)
            if m:
                start = int(m.group(1))
                end = int(m.group(2)) if m.group(2) else len(body) - 1
                headers["Content-Range"] = f"bytes {start}-{end}/{len(body)}"
                return self.reply(206, body[start:end + 1], headers)
        self.reply(200, body, headers)

    def do_HEAD(self):
        bucket, key, _ = self.parts()
        with lock:
            obj = store.get((bucket, key))
        if obj is None:
            return self.reply(404)
        self.reply(200, obj["body"], {"Content-Type": obj["type"], "ETag": f'"{obj["md5"]}"'})

    def do_PUT(self):
        bucket, key, _ = self.parts()
        data = self.body()
        md5 = hashlib.md5(data).hexdigest()
        with lock:
            store[(bucket, key)] = {"body": data, "type": self.headers.get("Content-Type", "binary/octet-stream"), "md5": md5}
        self.reply(200, b"", {"ETag": f'"{md5}"'})

    def do_DELETE(self):
        bucket, key, _ = self.parts()
        if bucket == "__objects":
            with lock:
                store.clear()
            return self.reply(204)
        with lock:
            store.pop((bucket, key), None)
        self.reply(204)

    def do_POST(self):
        bucket, key, q = self.parts()
        data = self.body()
        if "delete" in q:
            keys = [unescape(k) for k in re.findall(r"<Key>(.*?)</Key>", data.decode())]
            deleted = ""
            with lock:
                for k in keys:
                    store.pop((bucket, k), None)
                    deleted += f"<Deleted><Key>{escape(k)}</Key></Deleted>"
            xml = f'<?xml version="1.0" encoding="UTF-8"?><DeleteResult>{deleted}</DeleteResult>'
            return self.reply(200, xml.encode(), {"Content-Type": "application/xml"})
        self.reply(400)


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 9000
    ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()
