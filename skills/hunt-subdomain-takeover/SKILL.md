---
name: hunt-subdomain-takeover
description: Hunting skill for subdomain takeover — dangling CNAME records pointing at unclaimed cloud resources (S3, Azure, GitHub Pages, Heroku, Fastly, Shopify, WordPress.com) and dangling NS delegation. Covers CNAME-chain resolution, fingerprint matching against each service's unclaimed-error signature, and defaulting to reporting evidence over claiming. Use on any subdomain-enumeration result whose CNAME matches a known vulnerable-service pattern.
---

# Subdomain Takeover Hunting Methodology

A subdomain takeover happens when a DNS record — usually a CNAME, sometimes an NS delegation — still points at a third-party resource nobody owns anymore: a deleted S3 bucket, a deprovisioned Heroku app, a removed GitHub Pages site, or an identifier the expected tenant never actually claimed. The hostname still resolves into the third-party service's infrastructure, so anyone who registers that exact identifier on the service inherits requests, cookies, and trust for the organization's own subdomain.

## When to invoke

- Any subdomain-enumeration result (DNS brute force, certificate-transparency logs, passive DNS) whose CNAME target matches a known vulnerable-service fingerprint.
- Any NS record delegating a subdomain's zone to a third-party nameserver provider the organization may no longer control.
- Any allowlist entry, redirect target, or CORS origin another skill flags as sitting on takeover-eligible infrastructure (e.g. `hunt-cors`'s dangling-subdomain allowlist case) — this skill is where that lead gets confirmed.

## Technique: resolve, fingerprint, report before claiming

For every discovered subdomain, resolve the full CNAME chain (a name may hop through two or three CNAMEs before reaching the actual third-party service) and compare the terminal target against a fingerprint table. Then fetch the subdomain over HTTP(S) and match the response against that service's "unclaimed" signature.

### Fingerprint table

| Service | Typical CNAME target | Unclaimed-resource signature |
|---|---|---|
| AWS S3 (static website) | `*.s3.amazonaws.com`, `*.s3-website-<region>.amazonaws.com` | A distinctive XML error body naming the bucket as nonexistent (an S3 "no such bucket" class of error) |
| GitHub Pages | `*.github.io` | A distinctive "no Pages site configured" page served for that user/org/repo |
| Heroku | `*.herokuapp.com` | A generic Heroku "no such app"/app-not-found error page |
| Azure (Blob Storage, Web Apps, CDN) | `*.blob.core.windows.net`, `*.azurewebsites.net`, `*.azureedge.net` | A distinctive "resource not found" page naming the Azure service itself rather than the customer's app |
| Fastly | `*.fastly.net`, `*.fastlylb.net` | An edge-served "unknown domain" error distinct from any origin response — the edge itself refuses the request |
| Shopify | `*.myshopify.com` | A storefront-unavailable page indicating no shop is mapped to that name |
| WordPress.com | `*.wordpress.com` | A "no such blog"/domain-not-mapped notice |

Exact wording drifts as providers redesign error pages — treat each row as a signature *category* to confirm against a current, live reference, not a literal string to grep for blindly. Any other CNAME-based SaaS (Zendesk, Shopify-adjacent platforms, CDN/PaaS vendors not listed here) follows the same method: register-and-serve model, with a distinguishable "nothing is mapped here" response when the identifier is free.

### Dangling NS delegation

A more severe variant needs no CNAME at all: if a subdomain's NS records delegate its zone to a third-party DNS provider, and the organization's registration with that provider has lapsed while the parent's NS record still points at it, anyone can re-register the zone there and author *every* record type for the entire delegated subdomain — not just one hostname. Detect it by resolving NS for the subdomain and checking whether the provider has no zone configured for that name (NXDOMAIN/SERVFAIL/provider-specific "zone not found" responses on direct queries to those nameservers). Treat it as higher-severity than a single dangling CNAME and apply the same non-claiming default below.

### Default to reporting, not claiming

Actually claiming the identifier — registering the exact bucket/app/org/shop name on the vulnerable service to prove control — creates a live, attacker-controlled resource and is scope-sensitive: many bug bounty programs explicitly restrict or forbid it. Default to **reporting the CNAME record, the resolved error-page fingerprint match, and nothing more**. Only attempt a non-destructive claim-proof when the engagement's scope explicitly authorizes active claims, and even then keep the footprint minimal — a static proof-of-control page at the claimed identifier, nothing that interacts with real users or data.

## Depth floor

This class is naturally volume-driven: the floor is every discovered subdomain crossed against every fingerprint-table entry for a CNAME/NS match, which almost always exceeds **25 combinations** from ordinary subdomain enumeration alone. State that coverage explicitly rather than padding attempts artificially. If the discovered-subdomain count is genuinely small, broaden enumeration first (certificate transparency, passive DNS, permutation wordlists) instead of manufacturing busywork against a known-negative list.

## Evidence gate

Before `report_finding`:
- The exact CNAME chain resolved for the subdomain (or the NS records, for a delegation takeover).
- The resolved error page or fingerprint match — response body or screenshot showing the unclaimed-resource signature.
- If a claim was attempted: the explicit scope authorization it relied on, plus the proof-of-claim (the resource registered and the subdomain resolving to researcher-controlled content).
- Tag the finding `vulnclass: subdomain-takeover` and bind `traffic_refs`.

## Stop-authority

This skill decides *what takeover vectors to try* and *how to verify*. It never decides when the entire vulnerability class is exhausted — that stays with `hunting-methodology`.
