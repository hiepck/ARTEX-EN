#!/usr/bin/env python3
"""Minimal ARTEX REST stub for testing setup-enhance-phase1.sh.

Logs "METHOD PATH" to STUB_LOG and "METHOD PATH BODY" to STUB_BODYLOG, and
returns canned JSON simulating an install where the `playwright` MCP row ALREADY
EXISTS (so an idempotent seed issues no duplicate /api/mcp POST) and the worker
already has prior skills/mcp (so a merge must preserve them).

Usage: STUB_LOG=/path STUB_BODYLOG=/path python3 -I stub_enhance_phase1.py <port>
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
    if path == "/api/mcp":
        # playwright already registered; writeup-search/burp absent.
        return {"servers": [{"id": 1, "name": "playwright", "transport": "stdio"}]}
    if path == "/api/llm":
        return {"profiles": [{"id": 9, "name": "strong"}]}
    if path == "/api/agents/worker/visibility":
        return {"mcp": [7], "skill": ["hunting-methodology", "api-recon"]}
    if path == "/api/agents/planner":
        return {"key": "planner", "llm_profile_id": None}
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
