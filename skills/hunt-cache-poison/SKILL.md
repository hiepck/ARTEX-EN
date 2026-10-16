---
name: hunt-cache-poison
description: Hunting skill for Web Cache Poisoning and Web Cache Deception — unkeyed header/param abuse, host/port/scheme injection, parameter cloaking, and cache-busting differential analysis. Use when the target sits behind a CDN or reverse proxy and user input reaches a cacheable response.
---

# Web Cache Poisoning / Deception Methodology

Cache poisoning makes a server store a malicious response and serve it to other users. Cache deception tricks a cache into storing a private response under a public URL. Both are high-impact because one request can affect many victims.

## When to invoke

- Target behind Cloudflare, Fastly, Akamai, CloudFront, or any reverse proxy.
- Responses carry cache headers: `CF-Cache-Status`, `X-Cache`, `Age`, `Cache-Control`, `X-CDN`.
- User input reaches headers reflected in responses: `X-Forwarded-Host`, `X-Forwarded-Port`, `X-Forwarded-Scheme`, `X-Forwarded-Proto`, `Origin`, `Referer`.
- Parameters are reflected without being part of the cache key.

## Identify unkeyed inputs

The cache key usually includes URL path + query + some headers (`Host`, `Accept-Encoding`). Inputs outside the key are "unkeyed" and can poison the cache.

Test each header independently; send two requests with only that header changed and compare response body/headers:

```text
X-Forwarded-Host: attacker.com
X-Forwarded-Port: 1337
X-Forwarded-Proto: https
X-Forwarded-Scheme: https
X-Original-URL: /admin
X-Rewrite-URL: /admin
X-Http-Method-Override: POST
Origin: https://attacker.com
Referer: https://attacker.com
User-Agent: ...
Accept: application/json
Accept-Language: en-US
Cookie: ...
```

If the response reflects the header or changes status/body while `CF-Cache-Status: HIT` appears, the input is likely unkeyed.

## Host / port / scheme poisoning

1. `X-Forwarded-Host: attacker.com` in a password-reset or email feature may rewrite links.
2. `X-Forwarded-Port: 443` or `80` may change generated `Location` or asset URLs.
3. `X-Forwarded-Proto: https` may flip http→https in absolute URLs.
4. Combine with cache-busting: append `?cb=123` and verify the poisoned response is served to a clean request.

## Parameter cloaking

Some caches key on the first occurrence of a parameter, while backends use the last:

```
GET /?q=benign&q=<script>alert(1)</script>
```

If the cache stores under `q=benign` but the backend executes `q=<script>...`, you have stored XSS via cache poisoning.

Other variants:
- Different parameter encodings: `q%00=`, `q[]=`, `q%2b=`
- Path parameters: `/api;param=value/users`
- Semicolon path separators

## Web Cache Deception

Trick the cache into storing a private page under a public-looking path:

```
GET /account/settings/foo.css
Accept: text/css
```

If the backend ignores the `.css` suffix and returns `/account/settings`, but the cache keys on path and stores it, a victim requesting `GET /account/settings/foo.css` later receives the attacker's private page.

Requirements:
- Path suffix is ignored by the app or static handler.
- Cache keys on full path.
- Response is cacheable (`Cache-Control` absent or permissive).

## Cache-busting differential

To confirm poisoning without affecting real users, use a unique cache-buster and check that the poisoned response is replayed:

1. `GET /page?cb=uniquetoken` with unkeyed header `X-Forwarded-Host: evil.com`.
2. Observe `X-Cache: MISS` and response body containing `evil.com`.
3. `GET /page?cb=uniquetoken` from a clean session without the header.
4. Observe `X-Cache: HIT` and same `evil.com` body → confirmed poisoning.

## Bypass families

- Header name variants: `X-Forwarded-Host`, `X-Forwarded-For-Host`, `X-Host`, `Forwarded`, `X-Real-Host`
- Case: `x-forwarded-host`, `X-FORWARDED-HOST`
- Multiple values: `X-Forwarded-Host: legit.com, evil.com`
- Truncation: `X-Forwarded-Host: evil.com\x00 legit.com`
- Unicode normalization: `X-Forwarded-Host: ｅｖｉｌ．ｃｏｍ`

## Depth floor

Try at least **25 distinct unkeyed inputs or parameter/path tricks** per cacheable endpoint. Record negative results as `record_fact`.

## Evidence gate

Before `report_finding`:
- Show the poisoned request and the clean replay receiving the same poisoned response.
- Include cache-status headers (`CF-Cache-Status`, `X-Cache`, `Age`) for both requests.
- If mass impact is claimed, prove the poisoned resource is served to a different session/IP.
- Tag `vulnclass: web cache poisoning` or `web cache deception` and bind `traffic_refs`.

## Stop-authority

This skill decides *what cache inputs to test* and *how to confirm poisoning*. It never decides when the cache-poisoning class is exhausted — that stays with `hunting-methodology`.
