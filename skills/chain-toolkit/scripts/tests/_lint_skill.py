#!/usr/bin/env python3
"""_lint_skill.py — validate a SKILL.md / reference .md file.

Checks:
  1. Valid YAML frontmatter delimited by `---`, with non-empty `name` and `description`.
  2. None of the stale penred-specific tokens survive the port (penred paths and
     sub-agent names that mean nothing in ARTEX).

Exit 0 = clean; exit 1 = a problem (printed to stdout).

Usage: python3 -I _lint_skill.py <path-to-md>
"""
import re
import sys

# Tokens that must NOT appear in a ported skill/reference: penred filesystem paths
# and penred sub-agent names (ARTEX has no sub-agents — the worker pursues hops).
STALE = [
    ".claude/agent-memory",
    "findings.json",
    "tools/",
    "oauth-hunter",
    "xss-hunter",
    "idor-hunter",
    "csrf-hunter",
]


def main(path: str) -> int:
    with open(path, encoding="utf-8") as fh:
        text = fh.read()
    m = re.match(r"^---\n(.*?)\n---\n", text, re.DOTALL)
    if not m:
        print(f"{path}: missing YAML frontmatter")
        return 1
    fm = m.group(1)
    if not re.search(r"^name:\s*\S", fm, re.MULTILINE):
        print(f"{path}: frontmatter missing name")
        return 1
    if not re.search(r"^description:\s*\S", fm, re.MULTILINE):
        print(f"{path}: frontmatter missing description")
        return 1
    for tok in STALE:
        if tok in text:
            print(f"{path}: stale penred reference: {tok}")
            return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1]))
