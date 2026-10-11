#!/usr/bin/env python3
"""_lint_skill.py — validate a pack SKILL.md / reference .md file.

Checks, by file role (role = basename):

  Every .md:
    - no broken penred references: penred filesystem paths and orchestration hooks
      that mean nothing in ARTEX.

  SKILL.md (the actionable file loaded first) additionally:
    - valid YAML frontmatter with non-empty `name` and `description`.
    - no penred sub-agent names (ARTEX has no sub-agents; the worker pursues hops).
      Matches any `<word>-hunter` plus a few non-"-hunter" penred agent names.

  Reference files (lazy-loaded background DATA) may mention penred agent names as
  methodology labels — only the broken-path check applies to them.

Exit 0 = clean; exit 1 = a problem (printed to stdout). Usage: python3 -I _lint_skill.py <path.md>
"""
import os
import re
import sys

# Broken penred references — forbidden in EVERY file.
STALE_PATHS = [".claude/agent-memory", "findings.json", "tools/"]

# penred sub-agent names — forbidden in SKILL.md only.
AGENT_HUNTER = re.compile(r"\b[a-z]+-hunter\b")
AGENT_OTHER = ["auth-tester", "config-auditor", "poc-builder", "waf-profiler", "nuclei-writer"]


def main(path: str) -> int:
    with open(path, encoding="utf-8") as fh:
        text = fh.read()

    for tok in STALE_PATHS:
        if tok in text:
            print(f"{path}: stale penred reference: {tok}")
            return 1

    if os.path.basename(path) != "SKILL.md":
        return 0  # reference file: path check only

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

    hit = AGENT_HUNTER.search(text)
    if hit:
        print(f"{path}: penred sub-agent name in SKILL.md: {hit.group(0)}")
        return 1
    for name in AGENT_OTHER:
        if name in text:
            print(f"{path}: penred sub-agent name in SKILL.md: {name}")
            return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1]))
