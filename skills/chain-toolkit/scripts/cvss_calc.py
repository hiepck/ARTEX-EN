#!/usr/bin/env python3
"""
cvss_calc.py — CVSS 3.1 base score calculator from vulnerability class.

Deterministic scoring: each vulnerability class maps to a default CVSS 3.1
base vector.  ``score`` computes the numeric score, optionally adjusting
Privileges Required when authentication is needed.  Ported from SecPilot's
core/cvss.py into a standalone penred CLI tool (stdlib only).

Usage:
    python3 tools/cvss_calc.py score sqli
    python3 tools/cvss_calc.py score idor --auth-required
    python3 tools/cvss_calc.py score xss --oneline
    python3 tools/cvss_calc.py from-vector "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"
    python3 tools/cvss_calc.py list
"""

from __future__ import annotations

import argparse
import json
import math
import sys
from pathlib import Path

# ---------------------------------------------------------------------------
# CVSS v3.1 metric value tables
# ---------------------------------------------------------------------------

_AV = {"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.20}
_AC = {"L": 0.77, "H": 0.44}
_PR_UNCHANGED = {"N": 0.85, "L": 0.62, "H": 0.27}
_PR_CHANGED = {"N": 0.85, "L": 0.68, "H": 0.50}
_UI = {"N": 0.85, "R": 0.62}
_CIA = {"N": 0.0, "L": 0.22, "H": 0.56}

# ---------------------------------------------------------------------------
# Default CVSS vectors per vulnerability class
# ---------------------------------------------------------------------------

VULN_VECTORS: dict[str, str] = {
    # Injection — full compromise
    "sqli": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",
    "nosqli": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N",
    "rce": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",
    "command-injection": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",
    "ssti": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",
    "deserialization": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",
    "ldap-injection": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N",
    # Client-side
    "xss": "AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N",
    "cors": "AV:N/AC:L/PR:N/UI:R/S:C/C:H/I:N/A:N",
    "csrf": "AV:N/AC:L/PR:N/UI:R/S:U/C:N/I:H/A:N",
    "open-redirect": "AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N",
    "prototype-pollution": "AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:L/A:N",
    "host-header-injection": "AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N",
    "web-cache-poisoning": "AV:N/AC:L/PR:N/UI:R/S:C/C:N/I:L/A:N",
    # Server-side read/request
    "ssrf": "AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:N/A:N",
    "lfi": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N",
    "xxe": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N",
    "crlf": "AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:L/A:N",
    "graphql": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N",
    # Access control
    "idor": "AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:N/A:N",
    "jwt": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N",
    "auth-bypass": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N",
    "oauth-oidc": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N",
    "saml": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N",
    # Upload / smuggling
    "file-upload": "AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H",
    "request-smuggling": "AV:N/AC:L/PR:N/UI:N/S:C/C:L/I:L/A:N",
    # Timing / logic
    "race-condition": "AV:N/AC:H/PR:N/UI:N/S:U/C:N/I:H/A:N",
    "business-logic": "AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:H/A:N",
    "mass-assignment": "AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:H/A:N",
    # Exposure / infra
    "info-disclosure": "AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N",
    "subdomain-takeover": "AV:N/AC:L/PR:N/UI:N/S:C/C:N/I:L/A:N",
    "cloud-k8s-misconfig": "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N",
    # AI
    "llm-prompt-injection": "AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:N",
}

# ---------------------------------------------------------------------------
# Alias table — common names → canonical slug
# ---------------------------------------------------------------------------

_ALIASES: dict[str, str] = {
    "sql injection": "sqli",
    "nosql injection": "nosqli",
    "cross-site scripting": "xss",
    "reflected xss": "xss",
    "stored xss": "xss",
    "dom xss": "xss",
    "server-side request forgery": "ssrf",
    "cross-site request forgery": "csrf",
    "server-side template injection": "ssti",
    "remote code execution": "rce",
    "command injection": "command-injection",
    "local file inclusion": "lfi",
    "xml external entity": "xxe",
    "insecure direct object reference": "idor",
    "broken object level authorization": "idor",
    "bola": "idor",
    "json web token": "jwt",
    "authentication bypass": "auth-bypass",
    "oauth": "oauth-oidc",
    "oidc": "oauth-oidc",
    "openid connect": "oauth-oidc",
    "file upload": "file-upload",
    "http request smuggling": "request-smuggling",
    "race condition": "race-condition",
    "business logic": "business-logic",
    "mass assignment": "mass-assignment",
    "information disclosure": "info-disclosure",
    "subdomain takeover": "subdomain-takeover",
    "cloud misconfiguration": "cloud-k8s-misconfig",
    "kubernetes misconfiguration": "cloud-k8s-misconfig",
    "llm prompt injection": "llm-prompt-injection",
    "prompt injection": "llm-prompt-injection",
    "prototype pollution": "prototype-pollution",
    "host header injection": "host-header-injection",
    "web cache poisoning": "web-cache-poisoning",
    "crlf injection": "crlf",
    "graphql injection": "graphql",
    "ldap injection": "ldap-injection",
    "open redirect": "open-redirect",
    "cors misconfiguration": "cors",
    "deserialization": "deserialization",
    "insecure deserialization": "deserialization",
}

_GENERIC_VECTOR = "AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:L/A:N"


# ---------------------------------------------------------------------------
# Core calculation
# ---------------------------------------------------------------------------

def _roundup(value: float) -> float:
    return math.ceil(value * 10) / 10


def _parse_vector(vector: str) -> dict[str, str]:
    parts: dict[str, str] = {}
    clean = vector
    if clean.startswith("CVSS:"):
        clean = clean.split("/", 1)[1] if "/" in clean else clean
    for segment in clean.split("/"):
        key, _, val = segment.partition(":")
        if key and val:
            parts[key] = val
    return parts


def score_from_vector(vector: str) -> float:
    m = _parse_vector(vector)

    scope_changed = m.get("S") == "C"

    av = _AV[m["AV"]]
    ac = _AC[m["AC"]]
    pr_table = _PR_CHANGED if scope_changed else _PR_UNCHANGED
    pr = pr_table[m["PR"]]
    ui = _UI[m["UI"]]

    c = _CIA[m["C"]]
    i = _CIA[m["I"]]
    a = _CIA[m["A"]]

    iss = 1.0 - ((1.0 - c) * (1.0 - i) * (1.0 - a))

    if scope_changed:
        impact = 7.52 * (iss - 0.029) - 3.25 * (iss - 0.02) ** 15
    else:
        impact = 6.42 * iss

    if impact <= 0:
        return 0.0

    exploitability = 8.22 * av * ac * pr * ui

    if scope_changed:
        base = _roundup(min(1.08 * (impact + exploitability), 10.0))
    else:
        base = _roundup(min(impact + exploitability, 10.0))

    return base


def severity_from_score(score: float) -> str:
    if score >= 9.0:
        return "critical"
    if score >= 7.0:
        return "high"
    if score >= 4.0:
        return "medium"
    if score > 0.0:
        return "low"
    return "info"


def _adjust_vector_pr(vector: str, new_pr: str) -> str:
    parts: list[str] = []
    for segment in vector.split("/"):
        if segment.startswith("PR:"):
            parts.append(f"PR:{new_pr}")
        else:
            parts.append(segment)
    return "/".join(parts)


def normalize_slug(vuln_class: str) -> str:
    if not vuln_class:
        return ""
    low = vuln_class.lower().strip()
    if low in _ALIASES:
        return _ALIASES[low]
    slug = low.replace(" ", "-")
    if slug in VULN_VECTORS:
        return slug
    if low in VULN_VECTORS:
        return low
    return slug


def calculate_cvss(
    vuln_class: str,
    auth_required: bool = False,
) -> tuple[float, str]:
    slug = normalize_slug(vuln_class)
    vector = VULN_VECTORS.get(slug, _GENERIC_VECTOR)

    if auth_required:
        m = _parse_vector(vector)
        if m.get("PR") == "N":
            vector = _adjust_vector_pr(vector, "L")

    return score_from_vector(vector), vector


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def _cmd_score(args: argparse.Namespace) -> None:
    vuln = " ".join(args.vuln_class)
    score, vector = calculate_cvss(vuln, auth_required=args.auth_required)
    sev = severity_from_score(score)
    full_vector = f"CVSS:3.1/{vector}"
    slug = normalize_slug(vuln)

    if args.oneline:
        print(f"{score} {sev} {full_vector}")
    else:
        print(json.dumps({
            "vuln_class": slug or vuln,
            "score": score,
            "severity": sev,
            "vector": full_vector,
        }, indent=2))


def _cmd_from_vector(args: argparse.Namespace) -> None:
    raw = args.vector
    score = score_from_vector(raw)
    sev = severity_from_score(score)
    full = raw if raw.startswith("CVSS:") else f"CVSS:3.1/{raw}"

    if args.oneline:
        print(f"{score} {sev} {full}")
    else:
        print(json.dumps({
            "score": score,
            "severity": sev,
            "vector": full,
        }, indent=2))


def _cmd_list(_args: argparse.Namespace) -> None:
    header = f"{'Vuln Class':<25} {'Score':>5}  {'Severity':<10} Vector"
    print(header)
    print("-" * len(header))
    for slug in sorted(VULN_VECTORS):
        score = score_from_vector(VULN_VECTORS[slug])
        sev = severity_from_score(score)
        print(f"{slug:<25} {score:>5.1f}  {sev:<10} CVSS:3.1/{VULN_VECTORS[slug]}")


def _cmd_json(_args: argparse.Namespace) -> None:
    """Composite-severity mode: read {"vulnclass","auth_required"} from stdin,
    write {"vector","score","severity"} to stdout. The vector is the raw
    "AV:.../..." form (no "CVSS:3.1/" prefix). Unknown class → generic default."""
    d = json.load(sys.stdin)
    vc = str(d.get("vulnclass", ""))
    score, vector = calculate_cvss(vc, auth_required=bool(d.get("auth_required", False)))
    print(json.dumps({
        "vector": vector,
        "score": score,
        "severity": severity_from_score(score),
    }))


def main() -> None:
    parser = argparse.ArgumentParser(
        description="CVSS 3.1 base score calculator from vulnerability class.",
    )
    sub = parser.add_subparsers(dest="command")

    p_json = sub.add_parser("json", help="Composite mode: JSON stdin -> JSON stdout")
    p_json.set_defaults(func=_cmd_json)

    p_score = sub.add_parser("score", help="Score a vulnerability class")
    p_score.add_argument("vuln_class", nargs="+", help="Vulnerability class name or slug")
    p_score.add_argument("--auth-required", action="store_true",
                         help="Adjust PR:N to PR:L (auth needed)")
    p_score.add_argument("--oneline", action="store_true",
                         help="One-line output: score severity vector")
    p_score.set_defaults(func=_cmd_score)

    p_vec = sub.add_parser("from-vector", help="Score a raw CVSS vector")
    p_vec.add_argument("vector", help="CVSS vector string")
    p_vec.add_argument("--oneline", action="store_true")
    p_vec.set_defaults(func=_cmd_from_vector)

    p_list = sub.add_parser("list", help="List all vuln class mappings")
    p_list.set_defaults(func=_cmd_list)

    args = parser.parse_args()
    if not hasattr(args, "func"):
        parser.print_help()
        sys.exit(1)
    args.func(args)


if __name__ == "__main__":
    main()
