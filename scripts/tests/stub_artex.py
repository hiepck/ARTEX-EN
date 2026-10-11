#!/usr/bin/env python3
"""Minimal ARTEX REST stub for testing setup-chaining-pack.sh.

Logs "METHOD PATH" (one per line) to the file in env STUB_LOG, and returns canned
JSON that simulates an install where the chainer agent and its on_finding trigger
ALREADY EXIST — so an idempotent seed script must issue no create POSTs.

Usage: STUB_LOG=/path/log python3 -I stub_artex.py <port>
"""
import json
import os
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

LOG = os.environ.get("STUB_LOG", "/dev/null")
BODYLOG = os.environ.get("STUB_BODYLOG", "/dev/null")


def _canned(path: str):
    if path == "/api/auth/login":
        return {"token": "test-token"}
    if path == "/api/agents":
        return {"agents": [{"key": "chainer", "name": "Chainer"}]}
    if path == "/api/tools":
        return {"tools": []}
    if path == "/api/agents/chainer/triggers":
        return {"triggers": [{"on_finding": True}]}
    if path == "/api/agents/worker/visibility":
        # a pre-existing worker config the seed must preserve when merging.
        return {"mcp": [7], "skill": ["api-recon"]}
    return {"ok": True}


class Handler(BaseHTTPRequestHandler):
    def _handle(self):
        with open(LOG, "a", encoding="utf-8") as fh:
            fh.write(f"{self.command} {self.path}\n")
        length = int(self.headers.get("Content-Length", 0) or 0)
        body = self.rfile.read(length).decode("utf-8", "replace") if length else ""
        with open(BODYLOG, "a", encoding="utf-8") as fh:
            fh.write(f"{self.command} {self.path} {body}\n")
        body = json.dumps(_canned(self.path)).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    do_GET = _handle
    do_POST = _handle
    do_PUT = _handle

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    port = int(sys.argv[1])
    HTTPServer(("127.0.0.1", port), Handler).serve_forever()
