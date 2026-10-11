# Chaining + Bug-Hunting Pack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add bug-finding depth and multi-bug exploit chaining to ARTEX purely through skills, a custom agent, tool bindings, and shipped Python scripts — zero changes to the harness, core Go, or DB schema.

**Architecture:** Three additive layers. (1) A stateless Python **chain-toolkit** (ranking, CVSS, dedup) called via `Bash`. (2) A **skills pack** under `skills/` ported/adapted from `~/Tools/penred`. (3) A **chainer** custom agent, created and wired over ARTEX's existing REST API by a **seed script**, subscribing to the `on_finding` trigger. Chains are stored as composite `findings` rows (option a) via the existing `report_finding`.

**Tech Stack:** Python 3 (stdlib only) for the toolkit; Markdown for skills; Bash + `curl` for the REST seed script; ARTEX's existing Go REST API (no Go is written).

**Spec:** `docs/superpowers/specs/2026-10-11-chaining-bughunting-pack-design.md`

## Global Constraints

- **No harness changes** — `github.com/Autumn-27/norma` is off-limits.
- **Zero new Go** — no edits under `agent/`, `server/`, `db/`, `cmd/`, or `db/schema.sql`; no build-script changes.
- **English only** for all user-facing text (translation-fork rule, `CLAUDE.md`).
- Skills live at `skills/<name>/SKILL.md`, with optional `references/` and `scripts/`.
- Python scripts are **stateless, JSON-in / JSON-out**, stdlib-only, and are always invoked as `python3 -I <script> …` (never imported from an untrusted CWD).
- `/docs/` is gitignored in this fork; plan/spec files are added with `git add -f` (matching the existing `docs/vulnerability-traffic-evidence.md`).
- Chains are stored as composite `findings` rows (`vulnclass="chain"`) via the existing `report_finding` tool. No schema change.
- Scope enforcement stays on the worker's existing `intercept`/`guard` gate; the chainer never drives target traffic.
- Work happens on branch `feat/chaining-bughunting-pack`.

## Review Focus

- **Unknown vulnclass into `chain_plan.py`** — a finding whose class is absent from the anchor table must yield an empty candidate list, not a crash. (Task 1)
- **Unknown vulnclass into `cvss_calc.py`** — must fall back to a safe default vector/bucket, not raise. (Task 3)
- **Empty corpus into `dedup.py`** — comparing against no prior findings must return "not duplicate", not divide-by-zero or index error. (Task 2)
- **Seed script run twice (idempotency)** — re-running must not create a second `chainer` agent or a duplicate `on_finding` trigger; it updates in place. (Task 13)
- **Seed script against a down / unauthenticated ARTEX** — must fail fast with a clear message and make no partial configuration. (Task 13)

---

## Phase 1 — Chain toolkit (Python, standalone)

Deliverable: `skills/chain-toolkit/scripts/` with three working, tested scripts. Testable in isolation with no ARTEX running.

### Task 1: `chain_plan.py` — rank next-hop candidates per finding

**Files:**
- Create: `skills/chain-toolkit/scripts/chain_plan.py`
- Create: `skills/chain-toolkit/scripts/tests/test_chain_plan.py`
- Source to adapt: `/Users/hiepck/Tools/penred/tools/chain_plan.py` (reuse its `ANCHORS_BY_CLASS` dict only)

**Interfaces:**
- Produces: CLI `python3 -I chain_plan.py` reads JSON from **stdin** of the form
  `{"findings":[{"id":<int>,"vulnclass":"<str>","severity":"<str>"}, …]}` and writes JSON to **stdout**:
  `{"candidates":[{"finding_id":<int>,"vulnclass":"<str>","next":[{"hop":"<desc>","toward":"<impact/label>"}, …]}, …]}`.
  Unknown `vulnclass` → that finding appears with `"next":[]`.

- [ ] **Step 1: Write the failing test**

```python
# skills/chain-toolkit/scripts/tests/test_chain_plan.py
import json, subprocess, sys, pathlib
SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "chain_plan.py"

def _run(payload):
    p = subprocess.run([sys.executable, "-I", str(SCRIPT)],
                       input=json.dumps(payload), capture_output=True, text=True)
    assert p.returncode == 0, p.stderr
    return json.loads(p.stdout)

def test_known_class_yields_candidates():
    out = _run({"findings":[{"id":1,"vulnclass":"open-redirect","severity":"low"}]})
    c = out["candidates"][0]
    assert c["finding_id"] == 1
    assert len(c["next"]) >= 1
    assert all("hop" in h and "toward" in h for h in c["next"])

def test_unknown_class_yields_empty_next():
    out = _run({"findings":[{"id":9,"vulnclass":"no-such-class","severity":"low"}]})
    assert out["candidates"][0]["next"] == []

def test_empty_findings():
    out = _run({"findings":[]})
    assert out["candidates"] == []
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `python3 -m pytest skills/chain-toolkit/scripts/tests/test_chain_plan.py -v`
Expected: FAIL (`chain_plan.py` does not exist).

- [ ] **Step 3: Write the minimal implementation**

Copy the `ANCHORS_BY_CLASS` dict verbatim from `/Users/hiepck/Tools/penred/tools/chain_plan.py` (the `{class: [(description, agent_label), …]}` mapping). Replace the file I/O with stdin/stdout JSON. The `agent_label` from penred becomes the human-readable `"toward"` hint (ARTEX has no sub-agents).

```python
#!/usr/bin/env python3
"""chain_plan.py — rank next-hop chain candidates per finding (stateless, JSON in/out)."""
from __future__ import annotations
import json, sys

# --- paste ANCHORS_BY_CLASS verbatim from penred tools/chain_plan.py here ---
ANCHORS_BY_CLASS: dict[str, list[tuple[str, str]]] = {
    # "open-redirect": [("OAuth redirect_uri reflection …", "oauth-hunter"), …],
    # … (full dict copied from source) …
}

def plan(findings: list[dict]) -> list[dict]:
    out = []
    for f in findings:
        vc = str(f.get("vulnclass", "")).lower()
        anchors = ANCHORS_BY_CLASS.get(vc, [])
        nxt = [{"hop": desc, "toward": label} for (desc, label) in anchors]
        out.append({"finding_id": f.get("id"), "vulnclass": vc, "next": nxt})
    return out

def main() -> int:
    try:
        data = json.load(sys.stdin)
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"invalid json: {e}"}), file=sys.stderr)
        return 2
    print(json.dumps({"candidates": plan(data.get("findings", []))}))
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `python3 -m pytest skills/chain-toolkit/scripts/tests/test_chain_plan.py -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add skills/chain-toolkit/scripts/chain_plan.py skills/chain-toolkit/scripts/tests/test_chain_plan.py
git commit -m "feat(toolkit): stateless chain_plan.py ranking next-hop candidates"
```

### Task 2: `simhash.py` + `dedup.py` — near-duplicate detection

**Files:**
- Create: `skills/chain-toolkit/scripts/simhash.py` (verbatim copy of penred's)
- Create: `skills/chain-toolkit/scripts/dedup.py`
- Create: `skills/chain-toolkit/scripts/tests/test_dedup.py`
- Source: `/Users/hiepck/Tools/penred/tools/simhash.py` (copy verbatim)

**Interfaces:**
- Consumes: `simhash.signature(vuln_type, endpoint, param, evidence)`, `simhash.simhash(sig)`, `simhash.hamming(a,b)` (from the copied module).
- Produces: CLI `python3 -I dedup.py` reads stdin
  `{"candidate":{"vulnclass","endpoint","param","evidence"}, "corpus":[{same fields}, …], "threshold":3}` and writes stdout
  `{"duplicate":<bool>, "nearest_hamming":<int|null>}`. Empty corpus → `{"duplicate":false,"nearest_hamming":null}`.

- [ ] **Step 1: Write the failing test**

```python
# skills/chain-toolkit/scripts/tests/test_dedup.py
import json, subprocess, sys, pathlib
SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "dedup.py"

def _run(payload):
    p = subprocess.run([sys.executable, "-I", str(SCRIPT)],
                       input=json.dumps(payload), capture_output=True, text=True)
    assert p.returncode == 0, p.stderr
    return json.loads(p.stdout)

def test_empty_corpus_not_duplicate():
    out = _run({"candidate":{"vulnclass":"xss","endpoint":"/search","param":"q","evidence":"alert(1)"},
                "corpus":[], "threshold":3})
    assert out == {"duplicate": False, "nearest_hamming": None}

def test_near_duplicate_detected():
    cand = {"vulnclass":"xss","endpoint":"/search","param":"q","evidence":"reflected alert(1)"}
    prior = {"vulnclass":"xss","endpoint":"/find","param":"term","evidence":"reflected alert(1)"}
    out = _run({"candidate":cand, "corpus":[prior], "threshold":6})
    assert out["duplicate"] is True
    assert out["nearest_hamming"] is not None

def test_distinct_not_duplicate():
    cand = {"vulnclass":"xss","endpoint":"/a","param":"x","evidence":"alert(1)"}
    prior = {"vulnclass":"sqli","endpoint":"/z","param":"id","evidence":"union select"}
    out = _run({"candidate":cand, "corpus":[prior], "threshold":3})
    assert out["duplicate"] is False
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `python3 -m pytest skills/chain-toolkit/scripts/tests/test_dedup.py -v`
Expected: FAIL (`dedup.py` missing).

- [ ] **Step 3: Copy simhash.py, then write dedup.py**

Copy `/Users/hiepck/Tools/penred/tools/simhash.py` verbatim to `skills/chain-toolkit/scripts/simhash.py`. Then:

```python
#!/usr/bin/env python3
"""dedup.py — near-duplicate finding detection over a corpus (stateless, JSON in/out)."""
from __future__ import annotations
import json, sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parent))
import simhash  # noqa: E402

def _sig(f: dict) -> int:
    return simhash.simhash(simhash.signature(
        str(f.get("vulnclass", "")), str(f.get("endpoint", "")),
        str(f.get("param", "")), str(f.get("evidence", ""))))

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
    print(json.dumps(check(d.get("candidate", {}), d.get("corpus", []),
                           int(d.get("threshold", 3)))))
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `python3 -m pytest skills/chain-toolkit/scripts/tests/test_dedup.py -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add skills/chain-toolkit/scripts/simhash.py skills/chain-toolkit/scripts/dedup.py skills/chain-toolkit/scripts/tests/test_dedup.py
git commit -m "feat(toolkit): simhash port + stateless dedup.py near-duplicate check"
```

### Task 3: `cvss_calc.py` — composite severity

**Files:**
- Create: `skills/chain-toolkit/scripts/cvss_calc.py` (port of penred's, add a JSON mode)
- Create: `skills/chain-toolkit/scripts/tests/test_cvss_calc.py`
- Source: `/Users/hiepck/Tools/penred/tools/cvss_calc.py` (copy, then add a `json` subcommand)

**Interfaces:**
- Produces: CLI `python3 -I cvss_calc.py json` reads stdin `{"vulnclass":"<str>","auth_required":<bool>}` and writes stdout `{"vector":"<cvss3.1 vector>","score":<float>,"severity":"critical|high|medium|low"}`. Unknown class → a documented default vector (`AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:L/A:N`) and its bucket, never an exception. The existing `score`/`from-vector`/`list` subcommands are preserved verbatim.

- [ ] **Step 1: Write the failing test**

```python
# skills/chain-toolkit/scripts/tests/test_cvss_calc.py
import json, subprocess, sys, pathlib
SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "cvss_calc.py"

def _run(payload):
    p = subprocess.run([sys.executable, "-I", str(SCRIPT), "json"],
                       input=json.dumps(payload), capture_output=True, text=True)
    assert p.returncode == 0, p.stderr
    return json.loads(p.stdout)

def test_known_class_scores():
    out = _run({"vulnclass":"sqli"})
    assert out["severity"] in {"critical","high","medium","low"}
    assert isinstance(out["score"], (int,float)) and out["score"] > 0
    assert out["vector"].startswith("AV:")

def test_unknown_class_safe_default():
    out = _run({"vulnclass":"made-up-class"})
    assert out["severity"] in {"critical","high","medium","low"}
    assert out["vector"].startswith("AV:")
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `python3 -m pytest skills/chain-toolkit/scripts/tests/test_cvss_calc.py -v`
Expected: FAIL (`cvss_calc.py` missing).

- [ ] **Step 3: Port the script and add the `json` subcommand**

Copy `/Users/hiepck/Tools/penred/tools/cvss_calc.py` verbatim (its `_AV/_AC/…` tables, `VULN_VECTORS`, score math, and `score`/`from-vector`/`list` subcommands). Remove the `sys.path.insert`/local-import block at the top (not needed when standalone). Add a `json` subcommand to its argparse that reuses the existing scoring functions:

```python
def _severity_bucket(score: float) -> str:
    if score >= 9.0: return "critical"
    if score >= 7.0: return "high"
    if score >= 4.0: return "medium"
    return "low"

def cmd_json(_args) -> int:
    d = json.load(sys.stdin)
    vc = str(d.get("vulnclass", "")).lower()
    vector = VULN_VECTORS.get(vc, "AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:L/A:N")
    s = score_vector(vector, auth_required=bool(d.get("auth_required", False)))  # reuse existing scorer
    print(json.dumps({"vector": vector, "score": s, "severity": _severity_bucket(s)}))
    return 0
```

Wire `cmd_json` to a `json` subparser. If the existing scorer function is named differently than `score_vector`, use that name — do not rename the ported function.

- [ ] **Step 4: Run the test to verify it passes**

Run: `python3 -m pytest skills/chain-toolkit/scripts/tests/test_cvss_calc.py -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add skills/chain-toolkit/scripts/cvss_calc.py skills/chain-toolkit/scripts/tests/test_cvss_calc.py
git commit -m "feat(toolkit): port cvss_calc.py with JSON mode for composite severity"
```

### Task 4: `chain-toolkit` SKILL.md (invocation contract)

**Files:**
- Create: `skills/chain-toolkit/SKILL.md`

**Interfaces:**
- Consumes: the three scripts from Tasks 1–3.
- Produces: the skill doc the `chainer` agent reads to know the exact `Bash` invocations and JSON shapes.

- [ ] **Step 1: Write SKILL.md**

YAML frontmatter (`name: chain-toolkit`, one-line `description`), then sections:
- **Scripts** — for each of `chain_plan.py`, `dedup.py`, `cvss_calc.py`: the exact `Bash` command (`echo '<json>' | python3 -I skills/chain-toolkit/scripts/<script> [subcmd]`), the input JSON shape, and the output JSON shape (copy the Interfaces blocks above).
- **Operating rule** — scripts are advisory; on any error, proceed with your own reasoning (fail-safe).

- [ ] **Step 2: Validate frontmatter parses**

Run: `python3 -I skills/chain-toolkit/scripts/tests/_lint_skill.py skills/chain-toolkit/SKILL.md`
(The linter is built in Task 5; if doing Task 4 first, defer this step to after Task 5.)
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add skills/chain-toolkit/SKILL.md
git commit -m "docs(skill): chain-toolkit invocation contract"
```

---

## Phase 2 — Skills content pack

Deliverable: the methodology, chain, per-class, and validation skills under `skills/`. Each skill is validated by a shared linter (Task 5) that checks frontmatter and flags stale penred-specific references.

### Task 5: Skills linter

**Files:**
- Create: `skills/chain-toolkit/scripts/tests/_lint_skill.py`
- Create: `skills/chain-toolkit/scripts/tests/test_lint_skill.py`

**Interfaces:**
- Produces: `python3 -I _lint_skill.py <SKILL.md path>` exits 0 if the file has valid `---` YAML frontmatter containing non-empty `name` and `description`, and contains **none** of the stale tokens that must not survive the port: the literal strings `.claude/agent-memory`, `findings.json`, `tools/` (penred paths), and the penred agent names `oauth-hunter`, `xss-hunter`, `idor-hunter`, `csrf-hunter`, `subdomain-takeover` (ARTEX has no such sub-agents). Exits 1 with the offending token otherwise.

- [ ] **Step 1: Write the failing test**

```python
# skills/chain-toolkit/scripts/tests/test_lint_skill.py
import subprocess, sys, pathlib, tempfile, os
LINT = pathlib.Path(__file__).resolve().parent / "_lint_skill.py"

def _lint(text):
    with tempfile.NamedTemporaryFile("w", suffix=".md", delete=False) as f:
        f.write(text); path = f.name
    try:
        return subprocess.run([sys.executable, "-I", str(LINT), path], capture_output=True, text=True)
    finally:
        os.unlink(path)

def test_valid_skill_passes():
    r = _lint("---\nname: hunt-x\ndescription: hunts x\n---\n# body\n")
    assert r.returncode == 0, r.stderr

def test_missing_description_fails():
    r = _lint("---\nname: hunt-x\n---\n# body\n")
    assert r.returncode == 1

def test_stale_penred_token_fails():
    r = _lint("---\nname: hunt-x\ndescription: d\n---\nuse oauth-hunter agent\n")
    assert r.returncode == 1
    assert "oauth-hunter" in (r.stdout + r.stderr)
```

- [ ] **Step 2: Run to verify it fails**

Run: `python3 -m pytest skills/chain-toolkit/scripts/tests/test_lint_skill.py -v`
Expected: FAIL (linter missing).

- [ ] **Step 3: Implement the linter**

```python
#!/usr/bin/env python3
"""_lint_skill.py — validate a SKILL.md: frontmatter + no stale penred references."""
import sys, re
STALE = [".claude/agent-memory", "findings.json", "tools/",
         "oauth-hunter", "xss-hunter", "idor-hunter", "csrf-hunter", "subdomain-takeover"]

def main(path: str) -> int:
    text = open(path, encoding="utf-8").read()
    m = re.match(r"^---\n(.*?)\n---\n", text, re.DOTALL)
    if not m:
        print("missing YAML frontmatter"); return 1
    fm = m.group(1)
    if not re.search(r"^name:\s*\S", fm, re.MULTILINE):
        print("frontmatter missing name"); return 1
    if not re.search(r"^description:\s*\S", fm, re.MULTILINE):
        print("frontmatter missing description"); return 1
    for tok in STALE:
        if tok in text:
            print(f"stale penred reference: {tok}"); return 1
    return 0

if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1]))
```

- [ ] **Step 4: Run to verify it passes**

Run: `python3 -m pytest skills/chain-toolkit/scripts/tests/test_lint_skill.py -v`
Expected: PASS (3 tests). Then run the deferred Task 4 Step 2 lint on `chain-toolkit/SKILL.md`.

- [ ] **Step 5: Commit**

```bash
git add skills/chain-toolkit/scripts/tests/_lint_skill.py skills/chain-toolkit/scripts/tests/test_lint_skill.py
git commit -m "test(skill): SKILL.md linter (frontmatter + stale-reference guard)"
```

### Task 6: `chain-hunting` skill + references

**Files:**
- Create: `skills/chain-hunting/SKILL.md`
- Create: `skills/chain-hunting/references/chain-table.md` (from penred `rules/chain-table.md`)
- Create: `skills/chain-hunting/references/payloads.md` (from penred `rules/payloads.md`)
- Sources: `/Users/hiepck/Tools/penred/rules/{chain-table.md,payloads.md,hunting.md}`

**Interfaces:**
- Consumes: the chain-toolkit scripts (Task 4) and the evidence gate (Task 10).
- Produces: the methodology the `chainer` agent and worker follow for correlate → chain-table lookup → pursue → gate → compose.

- [ ] **Step 1: Copy the references, strip stale tokens**

Copy `rules/chain-table.md` → `references/chain-table.md` and `rules/payloads.md` → `references/payloads.md`. Remove/replace stale tokens the linter flags (penred agent names → "pursue a hop of type X"; `findings.json`/`tools/` references → the ARTEX equivalents: `report_finding`, `add_task_hint`, the exploration graph).

- [ ] **Step 2: Write SKILL.md**

Frontmatter (`name: chain-hunting`, description). Sections: Hybrid rule (correlate existing findings **and** actively pursue new hops); the pursue loop (plant `add_task_hint` → planner dispatches worker → per-hop gate); chain-confidence = min(leg confidences); any theoretical leg ⇒ not reportable; terminal-impact → compose composite finding via `report_finding(vulnclass="chain")` (reference Task 11 storage mapping). Point to the two references for lookup.

- [ ] **Step 3: Lint**

Run: `for f in skills/chain-hunting/SKILL.md skills/chain-hunting/references/chain-table.md skills/chain-hunting/references/payloads.md; do python3 -I skills/chain-toolkit/scripts/tests/_lint_skill.py "$f" || echo "LINT FAILED: $f"; done`
Expected: no `LINT FAILED` lines. (References have no frontmatter requirement — see note: run the linter's stale-token check only on references by giving them minimal frontmatter OR extend the linter in Task 5 to accept a `--refs-only` flag that skips the frontmatter check. Decide in Task 5; default: references carry a short frontmatter stub so one linter path covers both.)

- [ ] **Step 4: Commit**

```bash
git add skills/chain-hunting/
git commit -m "docs(skill): chain-hunting methodology + chain-table/payloads references"
```

### Task 7: `hunting-methodology` skill

**Files:**
- Create: `skills/hunting-methodology/SKILL.md`
- Source: `/Users/hiepck/Tools/penred/skills/hunting-methodology/SKILL.md` (+ its references)

- [ ] **Step 1: Copy + adapt** the penred `hunting-methodology` SKILL.md and any `references/` it carries; strip stale tokens; keep the 5-phase flow and the depth-matrix floor (≥25 attempts/class). Map any penred tool/agent names to ARTEX tools (`report_finding`, `record_fact`, `add_task_hint`, worker intents).
- [ ] **Step 2: Lint** every produced file with `_lint_skill.py`. Expected: pass.
- [ ] **Step 3: Commit** `git add skills/hunting-methodology/ && git commit -m "docs(skill): hunting-methodology (5-phase + depth floor)"`

### Task 8: `validation-gate` skill + never-submit

**Files:**
- Create: `skills/validation-gate/SKILL.md`
- Create: `skills/validation-gate/references/never-submit.md`
- Sources: `/Users/hiepck/Tools/penred/skills/triage-validation/SKILL.md`, `/Users/hiepck/Tools/penred/rules/never-submit.md`

- [ ] **Step 1: Copy + adapt** the 7-Question Gate, the 4 pre-submission gates, the Owned-Test-Account Trap, and the devil's-advocate stance into `SKILL.md`; copy `rules/never-submit.md` → `references/never-submit.md`. Strip stale tokens. State that these gates run **before** a worker calls `report_finding` and **before** the chainer composes a composite.
- [ ] **Step 2: Lint** both files. Expected: pass.
- [ ] **Step 3: Commit** `git add skills/validation-gate/ && git commit -m "docs(skill): validation-gate (7-Q + never-submit + owned-account trap)"`

### Task 9: Per-class `hunt-*` skills

**Files (one sub-step each, same shape):**
- Create: `skills/hunt-idor/`, `skills/hunt-oauth/`, `skills/hunt-xss/`, `skills/hunt-rce/`, `skills/hunt-business-logic/`, `skills/hunt-info-disclosure/` (each `SKILL.md` + its `references/`)
- Sources: the matching `/Users/hiepck/Tools/penred/skills/hunt-*/` directories

- [ ] **Step 1: For each class**, copy the penred `hunt-<class>/` directory (SKILL.md + references), strip stale tokens, and map penred tool/agent names to ARTEX tools and the worker-intent model. Preserve the source-review greps and per-class depth. SSRF and file-upload material (no standalone penred skill) is folded into `hunt-rce/references/` and `chain-hunting/references/`.
- [ ] **Step 2: Lint** every produced file. Expected: pass for all.
- [ ] **Step 3: Commit** `git add skills/hunt-* && git commit -m "docs(skill): per-class hunt-* skills (idor/oauth/xss/rce/business-logic/info-disclosure)"`

---

## Phase 3 — Agent wiring, docs, validation

Deliverable: a one-command seed that creates and wires the `chainer` agent, plus the setup doc and a manual validation run.

### Task 10: `scripts/setup-chaining-pack.sh` — REST seed

**Files:**
- Create: `scripts/setup-chaining-pack.sh`
- Create: `scripts/tests/test_setup_chaining_pack.sh` (a shell test using a stub HTTP server — see steps)

**Interfaces:**
- Consumes ARTEX REST endpoints (verified in `server/server.go`): `POST /api/auth/login` (admin password → JWT), `GET/POST /api/agents`, `PUT /api/agents/{key}/prompt`, `PUT /api/agents/{key}/visibility`, `PUT /api/tools/{key}`, `GET/POST /api/agents/{key}/triggers`.
- Produces: an idempotent setup. Env: `ARTEX_URL` (default `http://127.0.0.1:8787`), `ARTEX_ADMIN_PASSWORD` (required). Behavior: login → create `chainer` if absent (else reuse) → set prompt → bind skills (`chain-hunting`, `chain-toolkit`, `validation-gate`) via visibility → bind the chainer's tools → create the `on_finding` trigger if none exists (else update). Any HTTP failure (incl. auth) → exit non-zero before any write, print the failing endpoint.

- [ ] **Step 1: Write the failing test**

A bash test that points `ARTEX_URL` at a tiny Python stub (`python3 -m http.server`-style) recording requests, asserting: (a) with no password set the script exits non-zero and makes **no** POST; (b) run twice against a stub that reports the agent already exists, the second run issues **no** second `POST /api/agents` and **no** duplicate `POST …/triggers`. Place the stub inline in the test.

Run: `bash scripts/tests/test_setup_chaining_pack.sh`
Expected: FAIL (script missing).

- [ ] **Step 2: Implement `setup-chaining-pack.sh`**

`set -euo pipefail`. Require `ARTEX_ADMIN_PASSWORD` (else error + exit 2). `curl` login → extract JWT (`jq -r .token`). Helper `api()` that sends `Authorization: Bearer $JWT` and fails the script on non-2xx. Check existence with `GET /api/agents` (grep for `"key":"chainer"`) before `POST`. Check `GET /api/agents/chainer/triggers` for an existing `on_finding` trigger before `POST`. Bind tools by `PUT /api/tools/{key}` adding `chainer` to each tool's `agents` array (read current, append if absent). Keep the prompt text and the `finding_message` in heredocs.

- [ ] **Step 3: Run the test to verify it passes**

Run: `bash scripts/tests/test_setup_chaining_pack.sh`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add scripts/setup-chaining-pack.sh scripts/tests/test_setup_chaining_pack.sh
git commit -m "feat(setup): idempotent REST seed for the chainer agent + on_finding trigger"
```

### Task 11: `docs/chaining-pack.md` — setup + storage mapping + manual-UI equivalent

**Files:**
- Create: `docs/chaining-pack.md` (force-added; `/docs/` is gitignored)

- [ ] **Step 1: Write the doc.** Sections: what the pack is; prerequisites (ARTEX running, admin password); the one-command seed (`ARTEX_ADMIN_PASSWORD=… scripts/setup-chaining-pack.sh`); the **manual UI equivalent** (create custom agent `chainer`, paste prompt, bind the three skills + the tool set, add an `on_finding` trigger with the finding message); the **storage mapping** for option (a) (composite `findings` row `vulnclass="chain"`, `report` narrative, leg `finding_id`s in `evidence`, combined `traffic_refs`); and the per-hop evidence gate + scope rule.
- [ ] **Step 2: Commit** `git add -f docs/chaining-pack.md && git commit -m "docs: chaining pack setup, storage mapping, and manual-UI equivalent"`

### Task 12: Full toolkit test sweep + manual sample-chain validation

**Files:**
- Modify: `docs/chaining-pack.md` (append a "Validation run" appendix)

- [ ] **Step 1: Run the whole Python suite.** `python3 -m pytest skills/chain-toolkit/scripts/tests/ -v`. Expected: all pass.
- [ ] **Step 2: Lint every SKILL.md and reference.** `find skills/chain-hunting skills/hunting-methodology skills/validation-gate skills/hunt-* skills/chain-toolkit -name '*.md' -exec python3 -I skills/chain-toolkit/scripts/tests/_lint_skill.py {} \;`. Expected: no failures.
- [ ] **Step 3: Manual sample-chain run** on a self-hosted lab (no ARTEX chain-eval harness exists). Scenario `SSRF → cloud metadata → credential → privileged action`: start ARTEX, run the seed script, launch a task against the lab, confirm the worker reports the SSRF leg, the `chainer` fires on `on_finding` and plants a chain-pursuit hint, the planner dispatches the next hop, and a composite `findings` row (`vulnclass="chain"`) is stored with the option-(a) structure. Record the outcome (pass/fail + notes) in the doc appendix. **This step requires a running ARTEX + lab and is performed by the human operator; it is the acceptance gate for the pack.**
- [ ] **Step 4: Commit** `git add -f docs/chaining-pack.md && git commit -m "docs: record sample-chain validation run"`

---

## Self-Review

- **Spec coverage:** §4.A skills → Tasks 4,6,7,8,9; §4.B chainer agent + trigger → Task 10 (+ manual UI in Task 11); §4.C toolkit → Tasks 1–3; §5 data flow → exercised in Task 12 manual run; §6 storage (a) → documented Task 11, used Task 6/12; §7 gates → Task 8; §8 CVSS → Task 3; §9 scope safety → chainer prompt (Task 10) + doc (Task 11); §10 delivery → Tasks 10,11; §11 testing → Tasks 1–3,5,12. Covered.
- **Placeholder scan:** The port tasks name exact source files and exact transformations; no "TBD"/"handle edge cases". Code steps carry real code; copy steps name the file + the token-strip rule.
- **Type consistency:** JSON shapes are stable across tasks (`chain_plan` candidates, `dedup` verdict, `cvss_calc json` output). The linter token list (Task 5) is the same list referenced by Tasks 6–9.
- **Review Focus:** unknown-class (Tasks 1,3), empty corpus (Task 2), seed idempotency + auth failure (Task 10) each have a test.
- **Known open decision folded into Task 5:** whether references carry a frontmatter stub or the linter gets a `--refs-only` flag — resolve when building the linter; default is a short frontmatter stub on references.
