---
name: hunt-deserialization
description: Hunting skill for insecure deserialization (CWE-502) across Java (ObjectInputStream, ysoserial gadget chains), PHP (unserialize() + __wakeup/__destruct/__toString abuse), Python (pickle.loads, unsafe yaml.load), .NET (BinaryFormatter), and Ruby (Marshal.load, YAML.load/unsafe_load). Use on any cookie/header/hidden-field blob that isn't JSON, a non-standard session token, or a stack trace naming a serialization library.
---

# Insecure Deserialization Hunting Methodology

Insecure deserialization happens when a server reconstructs an object graph directly from attacker-reachable bytes — a cookie, a hidden form field, a header, an uploaded "saved state" file — without first validating that the bytes represent a safe, expected shape. Because deserializing a native object format can run constructor/finalizer/magic-method code as a side effect, a crafted blob can hijack that code path into arbitrary command execution (via a "gadget chain" of classes already loaded on the target, chained together by an attacker who controls none of their source), tamper with object state to bypass auth, or exhaust resources. This skill covers black-box hunting across the five most common ecosystems.

## When to invoke

- Any parameter, cookie, header, or hidden field holding a base64 or raw-binary blob that isn't valid JSON/standard JWT when decoded.
- A "remember me" cookie, session token, or SSO artifact whose decoded form doesn't look like opaque random bytes or a signed JSON structure — it has internal shape (lengths, type tags, class names).
- Any stack trace, error page, or verbose exception naming a (de)serialization class or call: `ObjectInputStream`, `readObject`, `InvalidClassException`, `unserialize`, `pickle`, `yaml.load`, `BinaryFormatter`, `Marshal.load`, `Psych`.
- Any feature that imports/restores a saved object: config import, report/template upload, "load saved session," cache/queue payloads (Redis, RabbitMQ, Java JMS) fed back into a consumer.
- API parameters literally named `state`, `token`, `session`, `data`, `payload`, or `object` carrying an opaque blob rather than a documented primitive type.

## Ecosystem fingerprints and sinks

| Ecosystem | How to recognize a legitimate sample | Typical sink / trigger |
|---|---|---|
| Java | Raw bytes start `0xAC 0xED` (base64 `rO0AB...`); if gzip-wrapped look for `H4sI` first. | `ObjectInputStream.readObject()` on attacker bytes; gadget runs via a chain of `readObject`/`toString`/`hashCode` overrides in a library on the classpath. |
| PHP | `a:N:{...}` (array), `O:N:"ClassName":M:{...}` (object), `s:N:"...";` (string) — often itself base64/urlencoded inside a cookie. | `unserialize()` triggers `__wakeup()` immediately; `__destruct()` fires at GC/end-of-request; `__toString()` fires if the object is later coerced to a string. |
| Python | `\x80` + protocol-version byte (pickle protocol ≥2), or ASCII opcode text (`c__main__\n`, `(dp0\n`) for older protocols. | `pickle.loads()`/`pickle.load()` — no "safe" mode exists; or `yaml.load(data)` without `Loader=yaml.SafeLoader`, which can instantiate objects via `!!python/object/apply:...`. |
| .NET | MS-NRBF header, visible as `AAEAAAD/////` once base64-encoded. | `BinaryFormatter.Deserialize()` (also watch `LosFormatter`, `NetDataContractSerializer`, and `JSON.NET` with `TypeNameHandling` not `None`). |
| Ruby | 2-byte version marker, typically `\x04\x08` (base64 `BAh`) for `Marshal`; YAML (Psych) input instead shows a `!ruby/object:` or `!ruby/hash:` tag in otherwise ordinary YAML. | `Marshal.load()`; `YAML.load()`/`YAML.unsafe_load()` (vs. the safe `YAML.safe_load`, which restricts tags). |

## Technique: fingerprint, then identify, before you ever fire a gadget chain

1. **Get a legitimate sample first.** Log in normally, capture the real cookie/token/blob, and match it against the table above before touching the parameter — firing gadget-chain tooling at an unconfirmed format wastes attempts and risks unpredictable behavior.
2. **Narrow language, framework, and version.** Stack traces/exception class names, response headers (`X-Powered-By`, server banners), and any visible dependency manifest (`pom.xml`/`build.gradle`, `composer.lock`, `requirements.txt`, `packages.config`/`.csproj`, `Gemfile.lock`) from a source leak or verbose error all narrow which libraries are actually on the classpath/vendor path — this is what determines whether a gadget chain exists at all, not just which language is in play.
3. **Treat gadget-chain tooling as a hypothesis generator, not a guaranteed hit.** `ysoserial`/`ysoserial.net` enumerate known Java/.NET gadget chains keyed to specific vulnerable libraries (e.g. Commons-Collections, Spring, Hibernate for Java; a comparable list for .NET formatters). `PHPGGC` (PHP Generic Gadget Chains) does the same for common PHP frameworks/CMSes. Generate candidates only for libraries you have evidence are present, and expect most candidates to fail — a confirmed deserialization sink with no matching gadget on the target is a real (and common) negative result, worth recording, not a dead end to paper over.
4. **For PHP, reason about the magic-method trigger, not direct code you supply.** A PHPGGC-style chain strings together classes already loadable on the target so that `__wakeup()`, `__destruct()`, or `__toString()` firing naturally during normal PHP object lifecycle steps into a dangerous sink (file write, `call_user_func`, SQL, `include`) inside an already-vendored class.
5. **For Python and Ruby, flag the unsafe call itself as the finding when no safe variant is used**, even before a gadget is confirmed: `pickle.loads`/`pickle.load` have no safe mode at all; `yaml.load()` without `SafeLoader` and `YAML.load`/`YAML.unsafe_load` (vs. `YAML.safe_load`) are each, by design, capable of instantiating arbitrary objects from tags in the input — confirm with a non-destructive proof primitive (below) rather than reasoning from the sink alone, since some apps route it through genuinely trusted-only input.
6. **Confirm format acceptance before escalating to a full chain.** Submit the legitimate sample unmodified (does it still work?), then a single corrupted byte (does the error differ from a normal auth failure — a parse/class-not-found error is a strong signal the bytes are truly being deserialized), before investing in a full gadget payload.

## Depth floor

Do not report this surface exhausted until you have tried **at least 25 distinct format/library/gadget-chain combinations** per candidate sink — vary the ecosystem assumption only if fingerprinting is genuinely ambiguous, then vary library/version guesses, specific gadget chains within a tool's catalog, and callback-channel choice (DNS vs. HTTP vs. timing) for each one attempted. Record every negative result (sink confirmed but no working gadget; format didn't match any fingerprint) as a `record_fact` so the planner knows what was covered.

## Evidence gate

Deserialization proof is one of the few primitives that can trivially become destructive if the gadget chain executes an attacker-chosen OS command — **never submit a gadget that deletes, overwrites, exfiltrates production data, or disrupts the service**, mirroring the standard non-destructive RCE-proof discipline (DNS/HTTP callback, or a benign command like `whoami`/`hostname`/`id`/`echo <marker>`, or a sleep/time-based primitive) rather than a reverse shell or any write/delete payload. Before `report_finding`:
- A non-destructive proof primitive demonstrating the gadget chain actually executed: a DNS or HTTP callback interaction with a unique token, or — if callbacks are unavailable — a measured timing delta from a `sleep`-equivalent primitive, or the output of a harmless read-only command (`whoami`, `hostname`, `id`) captured via the callback/response channel. A payload that merely crashes the deserializer or returns a parse error is **not** sufficient proof of code execution.
- The fingerprinted format evidence (the legitimate sample and its matched magic bytes/shape) and the specific library/version basis for the gadget chain used, so the chain of reasoning from "blob accepted" to "RCE confirmed" is reproducible.
- For PHP magic-method chains, the exact class/property structure of the serialized object and which magic method triggered the sink.
- Tag the finding `vulnclass: deserialization` and bind `traffic_refs`.

## Stop-authority

This skill decides *what deserialization vectors to try* and *how to verify*. It never decides when the entire vulnerability class is exhausted — that stays with `hunting-methodology`.
