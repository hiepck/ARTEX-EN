#!/usr/bin/env python3
"""dedup.py — near-duplicate finding detection over a corpus (stateless, JSON in/out).

Input  (stdin):  {"candidate": {"vulnclass","endpoint","param","evidence"},
                  "corpus": [{same fields}, ...], "threshold": <int, default 3>}
Output (stdout): {"duplicate": <bool>, "nearest_hamming": <int|null>}

An empty corpus yields {"duplicate": false, "nearest_hamming": null}.
Near-duplicate = SimHash Hamming distance <= threshold (see simhash.py).
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import simhash  # noqa: E402


def _sig(f: dict) -> int:
    return simhash.simhash(
        simhash.signature(
            str(f.get("vulnclass", "")),
            str(f.get("endpoint", "")),
            str(f.get("param", "")),
            str(f.get("evidence", "")),
        )
    )


def check(candidate: dict, corpus: list[dict], threshold: int) -> dict:
    if not corpus:
        return {"duplicate": False, "nearest_hamming": None}
    cand = _sig(candidate)
    nearest = min(simhash.hamming(cand, _sig(p)) for p in corpus)
    return {"duplicate": nearest <= threshold, "nearest_hamming": nearest}


def main() -> int:
    try:
        d = json.load(sys.stdin)
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"invalid json: {e}"}), file=sys.stderr)
        return 2
    print(
        json.dumps(
            check(d.get("candidate", {}), d.get("corpus", []), int(d.get("threshold", 3)))
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
