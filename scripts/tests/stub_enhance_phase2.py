#!/usr/bin/env python3
"""Minimal ARTEX REST stub for testing setup-enhance-phase2.sh.

Logs "METHOD PATH" to STUB_LOG and "METHOD PATH BODY" to STUB_BODYLOG, and
returns canned JSON simulating an install where the worker already has prior
skills/mcp bound and the pentest agent exists.

Usage: STUB_LOG=/path STUB_BODYLOG=/path python3 -I stub_enhance_phase2.py <port>
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
    if path == "/api/agents/worker/visibility":
        return {"mcp": [7], "skill": ["hunting-methodology", "hunt-idor", "client-verify"]}
    if path == "/api/agents/pentest/visibility":
        return {"mcp": [7], "skill": ["hunting-methodology"]}
    return {"ok": True}


class Handler(BaseHTTPRequestHandler):
    def _handle(self):
        with open(LOG, "a", encoding="utf-8") as fh:
            fh.write(f"{self.command} {self.path}\n")
        length = int(self.headers.get("Content-Length", 0) or 0)
        body = self.rfile.read(length).decode("utf-8", "replace") if length else ""
        with open(BODYLOG, "a", encoding="utf-8") as fh:
            fh.write(f"{self.command} {self.path} {body}\n")
        out = json.dumps(_canned(self.path)).encode()
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
