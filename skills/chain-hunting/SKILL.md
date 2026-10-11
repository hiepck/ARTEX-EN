---
name: chain-hunting
description: Correlate confirmed findings and actively pursue new hops to chain bugs into a higher-impact composite (ATO / RCE / mass data access), with a per-hop evidence gate.
---

# chain-hunting

Turn single findings into a chain. **Hybrid**: correlate the findings you already
have **and** actively pursue new hops that escalate a confirmed bug toward a
crown-jewel impact. Lean on the `chain-toolkit` helpers and the two references below.

## When this runs

On every new confirmed finding. Read the finding set and the exploration graph, then
decide whether any confirmed bug can feed the next hop toward a terminal impact.

## The loop

1. **Correlate.** List confirmed findings. Feed them to `chain_plan.py` (see the
   `chain-toolkit` skill) to rank candidate next-hops from the chain-table.
2. **Look up.** Open `references/chain-table.md` for the confirmed class: it names the
   capability each hop grants and the terminal impact it moves toward. Use
   `references/payloads.md` for the concrete bypass/payload to try on the hop.
3. **Plant the pursuit.** For an **in-scope** candidate, record a chain-pursuit hint
   with `add_task_hint`: state the capability already proven, the specific next hop to
   attempt, and the terminal impact it targets. The planner picks up the hint and
   dispatches a worker; the worker drives the hop through the normal scope-gated path.
   Do **not** drive target traffic yourself.
4. **Gate each hop.** A hop counts only when its postcondition is **independently
   observed** (see the evidence gate below).
5. **Advance or stop.** A proven hop becomes the new capability — loop again from it.
   Stop at a terminal impact, when no in-scope candidate remains, or when a hop can only
   be reached out of scope.

## Evidence gate (per hop)

- The hop's **postcondition must be observed on a second, independent action** — a
  leaked credential must authenticate a *fresh* request; a 200 response alone is never
  proof.
- The hop's **precondition must point at a proven postcondition** of an earlier hop —
  no orphan hops.
- **Chain confidence = the minimum confidence across its hops.** Any single un-triggered
  (theoretical) hop makes the whole chain theoretical → **not reportable** as a chain.
- Run the `validation-gate` skill (7-Question Gate, Owned-Test-Account Trap) before a
  leg is reported and before a composite is composed.
- Use `dedup.py` to avoid filing a hop or a composite that duplicates an existing one.

## Scope rule (hard)

Re-check scope on **every** hop. A hop that would pivot out of scope is a hard stop:
mark the chain blocked and do not pursue it. Never attempt an out-of-scope hop.

## Composing the composite

When a chain reaches a terminal impact with every hop proven:

1. Score it with `cvss_calc.py` on the **terminal impact** → severity bucket.
2. Register the composite with `report_finding`:
   - `vulnclass = "chain"`
   - `name` = the terminal impact (e.g. `SSRF → IMDS → account takeover`)
   - `severity` = the bucket from `cvss_calc.py`
   - `summary` = the one-line chain statement (legs → impact)
   - `evidence` = per-hop proof pointers **and the ordered leg finding IDs**
   - `traffic_refs` = the combined ordered traffic evidence across the legs
3. The leg findings stay as their own records in the exploration graph; the composite
   references them by id. (This is the storage model in `docs/chaining-pack.md`.)

## References

- `references/chain-table.md` — per-class capability → next-hop → terminal-impact anchors.
- `references/payloads.md` — bypass and payload tables for exploiting a hop.
