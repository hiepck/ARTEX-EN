---
name: client-verify
description: Prove or disprove client-side findings (DOM XSS, postMessage, client-side prototype pollution, SPA/authenticated flows) in a real browser via the Playwright MCP, when curl-based evidence is not enough.
mcps: [playwright]
---

# Client-Side Verification (real browser)

Curl-based evidence is a *candidate*. A client-side bug is proven only when the
payload fires in a real DOM. Use this skill when the sink or source is
client-side, when the flow needs a logged-in SPA, or when a reflected value only
a browser would execute.

## When to invoke
- A reflected/stored value reaches a client-side sink (`innerHTML`, `eval`,
  `document.write`, `location`, a framework template) and curl cannot confirm it
  executes.
- `postMessage` handlers, `window.name`, `location.hash`/`search` source→sink.
- Client-side prototype pollution with a DOM gadget.
- Any step that needs real cookies/session, redirects, or JS-rendered DOM.

## How to drive the browser (Playwright MCP)
1. `browser_navigate` to the target URL carrying the candidate payload.
2. `browser_snapshot` / `browser_evaluate` to read the DOM state and confirm the
   sink executed (e.g. a unique marker in `document`, a dialog, an attribute).
3. `browser_take_screenshot` / capture the console or network for evidence.
4. Prefer a unique, non-destructive marker (e.g. a random token in an alert /
   DOM node) over `alert(1)` so the proof is unambiguous and attributable.

## What counts as proof
The payload executing in-DOM, captured (screenshot / console / evaluated marker).
Record it with the finding's evidence. A 200 response or a reflected string in
the HTTP body is **not** client-side proof by itself.

## Stop-authority
This skill decides *how to verify*, never *when a class is exhausted* — that stays
with `hunting-methodology`. Do not stop the depth matrix early because a browser
check passed or failed.

## When the browser is unavailable
If the Playwright MCP is not reachable (the server did not unlock, a tool call
errors), **report that the browser channel is unavailable and fall back** to the
strongest in-band evidence; record an `inferred` fact describing what could not
be browser-confirmed. Never hang waiting on it and never fabricate a browser
result.
