---
name: hunt-cors
description: Hunting skill for CORS misconfiguration — origin reflection combined with `Access-Control-Allow-Credentials: true` (the exploitable combo; reflection alone is usually low-impact), null-origin bypass, allowlist-regex bypasses (unanchored/overly permissive patterns), and simple-request tricks that skip preflight. Use on any API serving sensitive data to `fetch`/XHR, or any response carrying `Access-Control-Allow-Origin`.
---

# CORS Misconfiguration Hunting Methodology

CORS relaxes the browser's same-origin policy on a per-response basis. The server opts a requesting origin into reading the response by echoing it back in `Access-Control-Allow-Origin` (ACAO); `Access-Control-Allow-Credentials: true` (ACAC) additionally lets that read happen with the victim's cookies/session attached. Per the Fetch spec, a browser **rejects** the combination `ACAO: *` + `ACAC: true` outright — a wildcard origin can never carry credentials. The actual vulnerable pattern is never a wildcard: it is `ACAO: <reflected request Origin>` + `ACAC: true`, which is spec-legal and exploitable. Test for this combination specifically; neither header alone is the bug.

## When to invoke

- Any API serving sensitive data (auth tokens, PII, account data, internal state) consumed by `fetch`/XHR from a browser context.
- Any response — on any endpoint, sensitive or not — that includes `Access-Control-Allow-Origin` at all; it tells you CORS logic exists and may be flawed elsewhere on the same host.
- SPA/API-gateway architectures where the API is on a different origin/subdomain than the frontend, since that's what makes CORS logic exist in the first place.

## Technique: probe origin handling, always check the combination

For each candidate endpoint, send the real request (not just `OPTIONS`) with a crafted `Origin` header, and inspect **both** `ACAO` and `ACAC` together:

| Origin sent | What it tests |
|---|---|
| `https://attacker-controlled.example` | Naive reflect-anything logic |
| `null` | Null-origin allowlisting (see below) |
| `https://victim.com.attacker.example` | Unanchored substring match |
| `https://evilvictim.com` (no dot before the brand name) | Overly permissive subdomain regex |
| `https://victim.com` with trailing `.`, mixed case, or an added port | Parser/comparison differentials |

A finding requires: `ACAO` reflects (or wrongly allows) the sent origin **and** `ACAC: true` is present on that same response. If `ACAC` is absent, the origin can still read the response but carries no session — log it as a `record_fact`, not a finding, unless the response itself contains sensitive data a non-authenticated attacker origin shouldn't see at all.

### Null-origin bypass

Some allowlists special-case `Origin: null` to support legacy redirect flows or local file testing, then forget it's also what a browser sends from a sandboxed context the attacker fully controls: a `<iframe sandbox="allow-scripts allow-forms">` (sandboxing without `allow-same-origin` forces a null origin), or a page loaded via `data:text/html,...` or `file://`. If `null` reflects into `ACAO` alongside `ACAC: true`, host the attacking page as one of those null-origin contexts — no DNS or infrastructure needed — and confirm the credentialed read fires from it.

### Allowlist-regex bypasses

Regex-based allowlists fail in two distinct, both-common ways:

1. **Missing end-anchor** — `/https?:\/\/.*victim\.com/` (no trailing `$`) matches anywhere the literal substring `victim.com` occurs, so `https://victim.com.attacker.example` passes: the attacker registers a domain where `victim.com` is merely a prefix label, not the actual origin.
2. **Optional/missing separator dot** — `/^https?:\/\/.*\.?victim\.com$/` (the `.` before the brand name made optional, or omitted) is meant to allow `*.victim.com` but also matches `https://evilvictim.com`: `.*` consumes `evil`, the optional dot matches zero characters, and `victim.com` matches literally — no subdomain relationship exists, but the regex passes it anyway.

Both bugs come from the same root cause: treating `*.victim.com` as "ends with victim.com" instead of "ends with a dot, then victim.com, with nothing else before that dot." Any allowlist you can't read the source of, probe both ways: a domain containing the brand as a substring, and a domain missing the required separator.

### Simple-request preflight bypass

The browser only sends a CORS preflight (`OPTIONS`) for "non-simple" requests — methods outside GET/HEAD/POST, or headers/content-types outside the simple-request allowlist (`application/x-www-form-urlencoded`, `multipart/form-data`, `text/plain`). If the server's Origin validation logic lives only in the `OPTIONS` handler and the actual `GET`/`POST` handler never re-checks `Origin`, send the real payload as a simple request to skip that validation entirely — e.g. `POST` with `Content-Type: text/plain` carrying a JSON body the backend still happily parses. Compare the `ACAO`/`ACAC` headers (and whether the request is honored at all) between a preflighted `application/json` request and the `text/plain` simple-request equivalent; a gap between them is the bug.

## Depth floor

Do not report this surface exhausted until you have tried **at least 25 distinct origin-variant × endpoint combinations** — crossing each origin probe above (attacker domain, null, substring, missing-separator, case/port/trailing-dot variants) against each sensitive-data endpoint and each endpoint that returns any `Access-Control-Allow-Origin` header at all. Record every negative result (origin not reflected, `ACAC` absent, regex correctly anchored) as a `record_fact`.

## Evidence gate

Before `report_finding`:
- The exact request/response pair showing **both** the reflected/wrongly-allowed origin in `Access-Control-Allow-Origin` **and** `Access-Control-Allow-Credentials: true` present together on the same response — neither header alone qualifies.
- For regex bypasses: the probing origin used and why it defeats the pattern (missing anchor vs. missing separator).
- For null-origin or simple-request bypasses: which browser mechanism was used to produce the bypass context (sandboxed iframe attributes, `data:`/`file:` context, or which header/content-type kept the request "simple").
- A description of the PoC HTML page that performs the credentialed cross-origin `fetch`/XHR and exfiltrates the response — to be hosted on attacker-controlled infrastructure and never executed against a real victim's production session without authorization.
- Tag the finding `vulnclass: CORS` and bind `traffic_refs`.

## Stop-authority

This skill decides *what CORS vectors to try* and *how to verify*. It never decides when the entire vulnerability class is exhausted — that stays with `hunting-methodology`.
