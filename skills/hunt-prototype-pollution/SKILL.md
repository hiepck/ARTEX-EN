---
name: hunt-prototype-pollution
description: Hunting skill for JavaScript prototype pollution (CWE-1321) — client-side and server-side (SSPP) via __proto__/constructor.prototype key injection through JSON body merges, query-string-to-object parsing (qs, $.extend, lodash merge/defaultsDeep), or deep-clone utilities. Use on any Node.js backend, JSON-merge endpoint, or client-side app parsing query params into state.
---

# Prototype Pollution Hunting Methodology

Prototype pollution lets you inject properties onto `Object.prototype` (or `Array.prototype`/`Function.prototype`) itself, so every object in the running process silently inherits an attacker-controlled default. Impact ranges from DOM XSS via a polluted sink to full RCE through an options object that gadget-chains into `child_process` or a template engine. This skill covers black-box hunting.

## When to invoke

- Any endpoint that deep-merges attacker JSON into a server-side object: config updates, PATCH semantics, nested form submission, bulk-update APIs, settings/preferences endpoints.
- Query-string-to-object parsing with bracket notation: `qs`, `jQuery.extend(true, ...)`, lodash `_.merge`/`_.mergeWith`/`_.defaultsDeep`, `deep-extend`, or any hand-rolled recursive assign.
- Deep-clone / deep-copy utilities (`_.cloneDeep`, `jQuery.extend(true, {}, obj)`, a hand-rolled `structuredClone` polyfill) applied to attacker-controlled input.
- Any Node.js backend — pollution there reaches every request in the process, not just the one that triggered it.
- Any client-side framework/SPA that reads `location.search`/`location.hash` into app state via one of the libraries above.
- A stack trace, source map, or lockfile leak naming a known-vulnerable merge library version.

## Impact ladder

1. **Client-side.** A polluted property becomes the default value a later call reads, landing in a DOM sink (`innerHTML`, `insertAdjacentHTML`, `jQuery.html()`, a templating library's "unescaped" output mode). Also useful to downgrade or bypass a page's CSP if the pollutable property feeds a `<script>`/`srcdoc` attribute the app builds itself.
2. **Server-side (SSPP) — denial of service.** Polluting a property like `toString`, `then`, or a type-check flag that core logic unconditionally reads can crash or hang every subsequent request in the process. Cheapest to prove and lowest severity — use it to confirm pollution lands before chasing RCE.
3. **Server-side (SSPP) → RCE via `child_process`.** Node's `child_process.exec`/`spawn` option objects are plain objects that inherit from `Object.prototype`. If you can pollute an option the caller never sets explicitly (e.g. the shell/exec-path the call falls back to), a later, unrelated `exec()`/`spawn()` call anywhere in the app picks up your polluted default and runs it.
4. **Server-side (SSPP) → RCE via template engine.** EJS, Pug, and Handlebars read behavior-affecting options (compile flags, output-function name, helper/compiler settings) from an options object that, absent pollution, falls back to safe built-in defaults. Polluting the right key makes the engine compile or evaluate attacker-controlled script content on the next render call anywhere in the app, not just the endpoint you injected through.

## Payload / detection table

| Payload (query/body key) | Form | What it targets |
|---|---|---|
| `__proto__[x]=y` | URL-encoded query string, bracket notation | Direct `Object.prototype.x = "y"` via a `qs`-style nested parser |
| `constructor[prototype][x]=y` | URL-encoded query string | Bypasses filters that block the literal string `__proto__` but not `constructor.prototype` |
| `{"__proto__":{"x":"y"}}` | JSON body | Direct pollution through `JSON.parse` + a recursive merge (`_.merge`, `$.extend(true, ...)`) |
| `{"constructor":{"prototype":{"x":"y"}}}` | JSON body | `__proto__`-filter bypass, JSON-body form |
| `a[__proto__][x]=y` nested inside an array/object value | URL-encoded query string | Some parsers only block `__proto__` as a top-level key, not nested one level down |
| `__proto__.x=y` | URL-encoded query string, dot notation | Parsers that split on `.` as well as `[` `]` |

Rotate the injected key (`x`) per payload family so a later confirmation probe can tell which injection actually landed.

## Confirming pollution landed when there is no direct reflection

Most pollution has no visible effect on the response that triggered it — you have to probe a *second* code path:

1. **Cross-endpoint property oracle.** Pollute a distinctive, unlikely-to-collide key (e.g. `__proto__[artexCanary]=1`), then hit an unrelated endpoint whose response body, headers, or behavior would change if any object in the process now carried that key (an endpoint that echoes `Object.keys(...)` of its input, a diagnostics endpoint that dumps config, or a feature that conditionally branches on a property it never explicitly sets).
2. **Behavior/timing oracle.** Pollute a property that is falsy-checked in a hot path you can time (a feature flag, a cache-bypass flag, a retry-count default) and compare response latency or branch behavior before and after the pollution attempt.
3. **Error-message oracle.** Pollute a type-sensitive property (`toString`, `valueOf`, `length`) and watch for a changed error class or stack trace on a later, unrelated request — a shift from a normal 404 to a `TypeError` confirms the prototype is dirty process-wide.
4. **Self-healing DoS correlation.** If a crash or hang follows a pollution attempt and clears after a process restart (or after sending a payload that deletes the polluted key where the parser supports unset), that correlation is itself evidence the pollution reached `Object.prototype`.

If `oob-probe` is available, use a canary property name unlikely to pre-exist and confirm its effect on a request that never touched the injection point.

## Depth floor

Do not report the surface exhausted until you have tried **at least 25 distinct payload/sink combinations** — vary the injection point (query string vs. JSON body), the key form (`__proto__` vs. `constructor.prototype`, bracket vs. dot notation), the target library inferred from the stack, and the sink you're chasing (DOM, `child_process`, template engine) across every merge/parse surface found. Record every negative result as a `record_fact` so the planner knows what was covered.

## Evidence gate

Before `report_finding`:
- **Client-side:** the exact request that injects the payload, plus the polluted property's observable effect — a DOM/console capture showing the sink executed attacker content (an `alert()`/marker firing from `innerHTML`, or a CSP-bypassing resource loading) — not just "the property was set."
- **Server-side:** the request plus a reproducible side effect on a *second*, independently-triggered request or a process-wide behavior change (crash, altered output, executed command) — never just "the merge accepted the key" with no observed downstream effect.
- **RCE chains:** the actual command/code execution output (contents of a canary file written, or command output reflected back) — not just that the options object carried the polluted key.
- Tag the finding `vulnclass: prototype-pollution` and bind `traffic_refs`.

## Stop-authority

This skill decides *what prototype-pollution vectors to try* and *how to verify*. It never decides when the entire vulnerability class is exhausted — that stays with `hunting-methodology`.
