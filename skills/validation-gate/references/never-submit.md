---
name: never-submit
description: Findings never worth submitting alone and conditionally-valid findings that need a chain or a concrete impact.
---

# Never-Submit List & Conditionally Valid Findings

Reference file for validator agent. Do not duplicate this content elsewhere.

## Never-Submit List (instant kill without chain)

These findings are ALWAYS rejected unless accompanied by a working exploit chain:

- Missing headers (CSP/HSTS/X-Frame-Options)
- Missing SPF/DKIM/DMARC
- GraphQL introspection alone
- Banner/version disclosure without CVE exploit
- Clickjacking without sensitive action PoC
- Self-XSS
- Open redirect alone
- SSRF DNS-only
- CORS wildcard without credentialed exfil PoC
- Logout CSRF
- Rate limit on non-critical forms
- Session not invalidated on logout
- Concurrent sessions allowed
- Internal IP in error message
- Missing cookie flags alone
- OAuth client_secret in mobile app (expected)
- OAuth client_id alone (public by design)
- OIDC discovery endpoint (public by design)
- SPA client-side config (API URLs, Segment keys)
- Subdomain takeover claim on `*.azurewebsites.net` (Microsoft reserves deprovisioned App Service hostnames — not exploitable; do not test, do not report)
- IDOR/cross-tenant finding where the victim identifier was read only from the researcher's own second (victim) account, with no shown attacker-reachable acquisition path (list/search/JS/error/redirect/predictable-ID) — the owned-test-account trap: owning both accounts proves the authz mechanic, never the delivery. See `rules/mistakes.md`.
- Rate-limit / lockout / brute-force-protection absence with no demonstrated compromise (no valid credential, reset-OTP, or 2FA code actually guessed; no real account taken over; no enumeration oracle run against a real population) — a missing control is a hardening gap, not a finding, until it is carried to an actual outcome. Exception: when the unthrottled guessing itself directly extracts value (e.g. a low-entropy gift-card/coupon/promo code space with real redeemable balance), the missing rate limit IS the impact — this is reportable, show the hit or the enumeration-rate-vs-space-size math.
- Origin-IP discovery (behind a CDN/WAF) — recon, not a finding; the origin IP alone exposes nothing unless chained to a direct-access control gap.
- User / account enumeration — recon; chain to ATO (or a proven enumeration oracle against a real population) or discard.
- Standalone log injection — not a finding without a demonstrated sink (no log-viewing XSS, no log-based RCE, no header-injection impact).
- Tabnabbing / reverse-tabnabbing without a sensitive action on the page — no impact; the absence of `rel=noopener` alone is a hardening note.
- Autocomplete on form fields — UX, not a vulnerability.
- Redirect-to-login (302) for unauthenticated access — correct auth behavior, not a bug.
- Negative-control false negatives: when the "should-fail" case uses a reserved/denylisted domain (e.g. `example.com`), the negative control is not attacker-controllable and manufactures a false negative — use a genuinely attacker-owned control value.

## Conditionally Valid (chain required)

These findings become valid when chained with the specified escalation:

| You Have | Chain Needed | Combined Impact |
|---|---|---|
| Open redirect | + OAuth code theft → token exchange | ATO |
| SSRF DNS-only | + internal service data exfil | Data breach |
| CORS wildcard | + credentialed data theft PoC | Cross-origin data theft |
| GraphQL introspection | + auth bypass on mutations | Unauthorized actions |
| S3 listing | + secrets in bundles → OAuth chain | ATO |
| Prompt injection | + IDOR via chatbot (other user data) | Data breach |
| Subdomain takeover | + OAuth redirect_uri at that subdomain | ATO |
