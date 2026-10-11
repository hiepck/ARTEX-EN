---
name: oob-probe
description: Confirm blind/out-of-band vulnerabilities (blind SSRF, blind XXE, OOB RCE, blind XSS) with a Burp Collaborator callback via the burp MCP — an observed DNS/HTTP interaction is proof a timing guess is not.
mcps: [burp]
---

# Out-of-Band Confirmation (Burp Collaborator)

Blind classes have no in-band response to read. The proof is an out-of-band
callback the target makes to a channel you control.

## When to invoke
- Blind SSRF (a URL/host parameter with no reflected response).
- Blind XXE (an external-entity fetch).
- OOB / blind RCE (a command whose only observable effect is a network callback).
- Blind XSS (a payload that executes in another user's context later).

## How to use the channel (burp MCP)
1. `generate_collaborator_payload` to mint a unique callback host/URL.
2. Inject it into the candidate parameter / entity / command / stored field.
3. `get_collaborator_interactions` to read back any DNS/HTTP interaction keyed to
   that payload. An interaction tied to your unique payload is the confirmation.

## Evidence rule (consistent with chain-hunting)
An interaction observed on the collaborator is the **second, independent action**
that confirms the hop — exactly the per-hop gate the chaining pack enforces. One
payload sent with no interaction back is not proof; keep it an `inferred` fact.

## Stop-authority
This skill confirms a blind hop; it never decides a class is exhausted
(`hunting-methodology` owns that).

## When Burp/Collaborator is unavailable
The burp MCP works only while Burp Suite and its MCP endpoint are running and
reachable from this host. If it is not, **fall back** to timing-based or
differential in-band evidence and record an `inferred` fact noting that OOB
confirmation was unavailable. Never fabricate an interaction and never hang
waiting for a callback.
