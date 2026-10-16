#!/usr/bin/env python3
"""Minimal ARTEX REST stub for testing setup-enhance-phase2.sh.

Stateful: PUT visibility updates in-memory state, GET returns it, so running the
seed twice exercises real idempotency (second run reads back the first result).

Logs "METHOD PATH" to STUB_LOG and "METHOD PATH BODY" to STUB_BODYLOG.

Usage: STUB_LOG=/path STUB_BODYLOG=/path python3 -I stub_enhance_phase2.py <port>
"""
import json
import os
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

LOG = os.environ.get("STUB_LOG", "/dev/null")
BODYLOG = os.environ.get("STUB_BODYLOG", "/dev/null")

# Prior installed state: overlaps the new Phase-2 skills to exercise dedup.
STATE = {
    "/api/agents/worker/visibility": {"mcp": [7], "skill": ["hunting-methodology", "hunt-ssrf", "client-verify"]},
    "/api/agents/pentest/visibility": {"mcp": [7], "skill": ["hunting-methodology", "hunt-ssrf"]},
}


def _canned(method: str, path: str, body: str):
    if path == "/api/auth/login":
        return {"token": "test-token"}
    if path in STATE:
        if method == "GET":
            return STATE[path]
        if method == "PUT":
            STATE[path] = json.loads(body or "{}")
            return {"ok": True}
    return {"ok": True}


class Handler(BaseHTTPRequestHandler):
    def _handle(self):
        with open(LOG, "a", encoding="utf-8") as fh:
            fh.write(f"{self.command} {self.path}\n")
        length = int(self.headers.get("Content-Length", 0) or 0)
        body = self.rfile.read(length).decode("utf-8", "replace") if length else ""
        with open(BODYLOG, "a", encoding="utf-8") as fh:
            fh.write(f"{self.command} {self.path} {body}\n")
        out = json.dumps(_canned(self.command, self.path, body)).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(out)))
        self.end_headers()
        self.wfile.write(out)

    do_GET = _handle
    do_POST = _handle
    do_PUT = _handle
    do_PATCH = _handle

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    HTTPServer(("127.0.0.1", int(sys.argv[1])), Handler).serve_forever()
