#!/usr/bin/env python3
"""
simhash.py — deterministic content SimHash for near-duplicate finding detection.

XBOW dedups the same bug across thousands of hosts with SimHash (content similarity) so one
systemic bug is one report, not forty. bbmem's exact fingerprint (type|path|param) already collapses
identical paths, but it can't see that "reflected XSS on /search?q=" and "reflected XSS on
/find?term=" are the SAME bug on the same stack — different path, identical weakness. A SimHash over
a normalized content signature catches those near-duplicates (small Hamming distance), so bb-reporter
can file them as one report citing every affected host instead of N look-alike reports.

Deterministic, stdlib only (blake2b). 64-bit fingerprints; near-duplicate ≈ Hamming distance ≤ 3.

    from simhash import simhash, hamming, signature
    a = simhash(signature("xss", "/search", "q", evidence))
    b = simhash(signature("xss", "/find", "term", evidence))
    hamming(a, b)   # small ⇒ likely the same systemic bug
"""

from __future__ import annotations

import hashlib
import re
from collections import Counter

BITS = 64
_MASK = (1 << BITS) - 1
_TOKEN = re.compile(r"[a-z0-9_]+")


def _norm_path(endpoint: str) -> str:
    """Host- and id-agnostic path (mirrors bbmem._norm_endpoint), so the same route on two hosts
    or with different numeric ids produces the same signature component."""
    e = endpoint.strip()
    e = re.sub(r"https?://[^/ ]+", "", e)
    parts = e.split(None, 1)
    if len(parts) == 2 and parts[0].lower() in {
            "get", "post", "put", "patch", "delete", "options", "head"}:
        e = parts[1]
    e = re.sub(r"\b\d+\b", "{id}", e)
    e = re.sub(r"\{[^}]+\}", "{id}", e)
    return re.sub(r"\s+", " ", e).strip().lower()


def signature(vuln_type: str, endpoint: str = "", param: str = "", evidence: str = "") -> str:
    """Build the content string that gets hashed for near-duplicate detection.

    Evidence (a response snippet / PoC) is the strongest signal that two findings are the SAME
    systemic bug — identical reflected markup, the same error page, the same leaked shape — so when
    it is present it is weighted to DOMINATE the fingerprint, and the differing path/param become
    minor. That is what lets "XSS on /search?q=" and "XSS on /find?term=" land close. Without
    evidence, the signature falls back to class + param + path (near-dup is inherently weaker then —
    pass --evidence for good clustering)."""
    ev = re.sub(r"\s+", " ", (evidence or "").strip().lower())[:600]
    core = " ".join(p for p in [(vuln_type or "").strip().lower(),
                                (param or "").strip().lower(),
                                _norm_path(endpoint)] if p)
    if ev:
        return f"{core} {(ev + ' ') * 4}".strip()   # evidence ~4x weight → dominates the vote
    return core


def _tokens(text: str) -> list[str]:
    text = text.lower()
    words = _TOKEN.findall(text)
    # char trigrams give robustness when strings are short and share substrings
    grams = [text[i:i + 3] for i in range(max(0, len(text) - 2))]
    return words + grams


def _hash64(token: str) -> int:
    return int.from_bytes(hashlib.blake2b(token.encode(), digest_size=8).digest(), "big")


def simhash(text: str, bits: int = BITS) -> int:
    """Charikar SimHash: weighted bitwise vote over token hashes → a bits-wide fingerprint."""
    if not text:
        return 0
    v = [0] * bits
    for tok, w in Counter(_tokens(text)).items():
        h = _hash64(tok)
        for i in range(bits):
            v[i] += w if (h >> i) & 1 else -w
    out = 0
    for i in range(bits):
        if v[i] > 0:
            out |= (1 << i)
    return out & _MASK


def hamming(a: int, b: int) -> int:
    return bin((a ^ b) & _MASK).count("1")


def is_near(a: int, b: int, threshold: int = 3) -> bool:
    return hamming(a, b) <= threshold


if __name__ == "__main__":
    import sys
    if len(sys.argv) >= 3:
        a = simhash(sys.argv[1])
        b = simhash(sys.argv[2])
        print(f"simhash A: {a:016x}\nsimhash B: {b:016x}\nhamming:   {hamming(a, b)}")
    else:
        print("usage: simhash.py <textA> <textB>   (prints both fingerprints + Hamming distance)")
