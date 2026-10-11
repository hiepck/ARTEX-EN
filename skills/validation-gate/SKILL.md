---
name: validation-gate
description: Kill weak or theoretical findings before reporting — the 7-Question Gate, pre-submission gates, and the Owned-Test-Account Trap.
---

# Triage & Validation

## The 7-Question Gate (applied to EVERY finding)

First NO = KILL. Do not continue.

1. **Q1**: Exact HTTP request + response showing the issue RIGHT NOW?
2. **Q2**: Bug class accepted by this program?
3. **Q3**: Asset in-scope, owned by target (not third-party)?
4. **Q4**: Works without admin/privileged access?
5. **Q5**: Not documented behavior or known issue?
6. **Q6**: Impact proved with actual data (not just status code)?
7. **Q7**: Not on the never-submit list? — **mode-dependent.** In `mode: pentest`
   this inverts: the item is reported at Low/Info with remediation, not killed
   (rules/severity-floor.md). In `mode: bugbounty` it is killed. Resolve the mode
   from ENGAGEMENT.yaml first.

## Never-Submit List (bugbounty mode — inverts to a severity floor in pentest)

Missing headers (CSP/HSTS/X-Frame-Options), missing SPF/DKIM/DMARC,
GraphQL introspection alone, banner/version without exploit,
clickjacking without PoC, self-XSS, open redirect alone,
SSRF DNS-only, CORS wildcard without credentialed exfil,
logout CSRF, rate limit on non-critical, missing cookie flags alone,
OAuth client_id alone, OIDC discovery endpoints, SPA client-side config.

## 4 Pre-Submission Gates

**Gate 0 (30 sec)**: Confirmed with real HTTP? In scope? Reproducible? Evidence?

**Gate 1 (2 min)**: What does attacker walk away with? Real victim? Max 2 preconditions?

**Gate 2 (5 min)**: Searched hacktivity? Read disclosed reports? Not in changelog?

**Gate 3 (10 min)**: Title follows formula? Steps have exact HTTP? CVSS calculated?

## Conditional Validity (chain required)

| Finding | Chain Needed | Without Chain |
|---|---|---|
| Open redirect | + OAuth code theft → ATO | KILL |
| SSRF DNS-only | + internal data exfil | KILL |
| CORS wildcard | + credentialed data theft | KILL |
| GraphQL introspection | + auth bypass on mutations | KILL |
| S3 listing | + secrets in bundles | KILL |
| Prompt injection | + IDOR via chatbot | KILL |

## Severity Calibration

| Impact | Severity |
|---|---|
| Full ATO, RCE, mass PII exfil | Critical |
| Read/modify other user's private data | High |
| Stored XSS in victim context | Medium-High |
| Info disclosure (sensitive creds) | Medium |
| Info disclosure (non-sensitive) | Low/Info |

## When to run (ARTEX)

Apply this gate **before** a worker calls `report_finding` for any leg, and **before**
the chainer composes a composite (`vulnclass="chain"`). A finding that fails any gate
is recorded as an inferred fact with `record_fact` (suspicion + why it failed), not
filed as a finding. See `references/never-submit.md` for classes that are never worth
submitting alone and must chain to a concrete impact first.
