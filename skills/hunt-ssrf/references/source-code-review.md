---
name: source-code-review
description: SSRF — source-code review patterns (grep + Semgrep) for when you have repo access.
---

# SSRF — Source Code Review Patterns

> **Part of `skills/hunt-ssrf/SKILL.md`.** Read this ONLY when you have source/repo access (OSS bug, internal pentest, in-scope GitHub org). Not useful for pure black-box hunting.

---

When you have repo access, grep is faster than blind payload fuzzing: it points you
straight at the fetcher, the allow-list, and the redirect policy.

## Find the fetchers

Grep for the HTTP client calls and the URL construction around them:

```bash
grep -rnE '(requests\.(get|post)|urlopen|urllib\.request|http\.client|aiohttp|httpx|axios|node-fetch|got\(|fetch\(|RestTemplate|HttpClient|okhttp|curl_|wget)' --include='*.py' --include='*.js' --include='*.ts' --include='*.java' --include='*.go' .
```

Then trace each URL back to its source: does it come from a request parameter,
a header (`Referer`, `X-Forwarded-*`), a stored value, or a DB field?

## Find the guards (and their holes)

```bash
grep -rniE '(allowlist|whitelist|denylist|blacklist|blocklist|isSafe|isInternal|privateip|validateurl|ssrf|urlparse|netloc)' .
```

Read each guard and look for:

- **Blocklist of literal strings** (`"127.0.0.1" in url`) — bypassable with
  `127.1`, decimal/octal IPs, `0.0.0.0`, IPv6, or DNS rebinding.
- **Check-then-use gap** — the URL is validated, then a client follows a
  redirect (302) or re-resolves DNS after the check.
- **Missing IMDS block** — `169.254.169.254`, `fd00:ec2::254`, and cloud
  metadata hostnames not denied.

## Semgrep rules (paste into `.semgrep.yml`)

```yaml
rules:
  - id: ssrf-requests-user-url
    patterns:
      - pattern-either:
          - pattern: requests.get($URL, ...)
          - pattern: requests.post($URL, ...)
          - pattern: urllib.request.urlopen($URL, ...)
          - pattern: httpx.get($URL, ...)
      - pattern-not: requests.get("...", ...)
      - pattern-not-inside: |
          if <... $GUARD ...>:
              ...
    message: Potential SSRF — user-controlled URL reaches an HTTP client with no visible guard.
    languages: [python]
    severity: WARNING

  - id: ssrf-urlopen-dynamic
    patterns:
      - pattern: urlopen($URL)
      - pattern-not: urlopen("...")
    message: urlopen on a dynamic URL — trace the source.
    languages: [python]
    severity: INFO
```

## Redirect policy

Redirects turn a benign-looking allow-listed host into an SSRF pivot. Grep for
the follow-redirect flag on the client:

```bash
grep -rniE '(allow_redirects\s*=\s*True|follow_redirects\s*=\s*True|maxRedirects|MaxRedirects)' .
```

Presence of an enabled redirect policy plus an allow-list that only checks the
*first* URL is a high-probability SSRF.

## Evidence from source

When reporting from source review, cite:

- `file:line` of the sink (the HTTP client call),
- `file:line` of the guard you claim is missing or bypassable,
- the data-flow path from an HTTP handler/parameter to the sink.
