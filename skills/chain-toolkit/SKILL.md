---
name: chain-toolkit
description: Stateless Python helpers (rank next-hop chain candidates, score composite CVSS, detect near-duplicate findings) the chainer agent calls via Bash.
---

# chain-toolkit

Three stdlib-only Python scripts that support exploit-chain analysis. Each is
**stateless**: it reads JSON from stdin and writes JSON to stdout, so it holds no
database and leaves no files. Call them with `python3 -I` from the Bash tool.

All three are **advisory**. If a call errors, times out, or returns nothing useful,
proceed with your own reasoning — never block the run on a helper. (Fail-safe.)

## chain_plan.py — rank next-hop candidates

Given the current findings, return ranked candidate next-hops per finding (from the
chain-table anchors). A finding whose class is unknown comes back with an empty
`next` list.

```bash
echo '{"findings":[{"id":1,"vulnclass":"info-disclosure","severity":"medium"}]}' \
  | python3 -I skills/chain-toolkit/scripts/chain_plan.py
```

- **Input:** `{"findings":[{"id":<int>,"vulnclass":"<str>","severity":"<str>"}, ...]}`
- **Output:** `{"candidates":[{"finding_id":<int>,"vulnclass":"<str>","next":[{"hop":"<desc>","toward":"<label>"}, ...]}, ...]}`

Use it right after a new finding to decide which hop to pursue toward a terminal impact.

## cvss_calc.py — composite severity

Score a composite chain by its terminal impact and map it to a severity bucket.

```bash
echo '{"vulnclass":"rce","auth_required":false}' \
  | python3 -I skills/chain-toolkit/scripts/cvss_calc.py json
```

- **Input:** `{"vulnclass":"<str>","auth_required":<bool>}`
- **Output:** `{"vector":"<AV:.../...>","score":<float>,"severity":"critical|high|medium|low"}`
- Unknown class → a generic default vector and its bucket (never an error).

Use the returned `severity` as the composite finding's severity when reporting the chain.

## dedup.py — near-duplicate check

Decide whether a candidate finding is a near-duplicate of anything already found,
by SimHash similarity (e.g. the same route with a different numeric id collapses to
one bug).

```bash
echo '{"candidate":{"vulnclass":"idor","endpoint":"/api/users/123","param":"id","evidence":"read other user"},"corpus":[],"threshold":3}' \
  | python3 -I skills/chain-toolkit/scripts/dedup.py
```

- **Input:** `{"candidate":{"vulnclass","endpoint","param","evidence"},"corpus":[{same fields}, ...],"threshold":<int, default 3>}`
- **Output:** `{"duplicate":<bool>,"nearest_hamming":<int|null>}` — empty corpus → `{"duplicate":false,"nearest_hamming":null}`.

Run it before reporting a leg or composing a composite, to avoid filing the same bug twice.

## Operating rule

Helpers inform; they do not decide. Treat their output as a ranked suggestion and a
severity/dedup sanity check — the evidence gate and the scope rule still govern what
you actually pursue and report.
