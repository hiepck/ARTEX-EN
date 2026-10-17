---
name: hunt-ssti
description: Hunting skill for Server-Side Template Injection (SSTI) across Jinja2/Twig, FreeMarker/Velocity/Thymeleaf (Java), and ERB/Handlebars (Ruby/Node). Covers polyglot engine fingerprinting, the parse-error-vs-runtime-evaluation diagnostic, and sandbox-escape via object introspection. Use on any feature rendering user input through a template engine: comments, email templates, PDF generation, admin-customizable templates, PR-description rendering.
---

# Server-Side Template Injection (SSTI) Hunting Methodology

SSTI happens when user-controlled text is evaluated as template source — expressions, attribute access, sometimes full statements — rather than rendered as inert content. Impact ranges from information disclosure to full code execution depending on the engine and whether it runs sandboxed. A single stack often has several template engines in play at once (a view layer, a separate email-template renderer, a PDF generator), so fingerprinting which engine actually evaluates a given field comes before any escalation attempt.

## When to invoke

- Any feature rendering user-controlled text through a template engine distinct from plain string interpolation: rich comment/message formatting, outbound email templates, PDF/report generation, admin-customizable templates (invoice/notification layouts), PR- or issue-description rendering, CI/report generators that accept a user-supplied template or format string.
- Fields named `template`, `format`, `layout`, `subject`, or `body` on an endpoint advertising "customizable" output.
- An error naming a template engine class (Jinja2 `TemplateSyntaxError`, Twig `Twig\Error\SyntaxError`, FreeMarker `freemarker.core.*`, Velocity `org.apache.velocity.*`, Thymeleaf `org.thymeleaf.exceptions.*`), or a stack trace showing template compilation.
- Rendered output that subtly differs when template metacharacters (`{{`, `${`, `<%`, `#{`) are added, even with no visible error.

## Technique: polyglot fingerprint first, then escalate per engine

Fire a polyglot payload — syntactically distinct across engine families — into the same field before trying anything engine-specific. The differential in which sub-expression evaluates (and which is ignored, escaped, or errors) fingerprints the engine family cheaper than guessing from the stack alone. The same input can also reach more than one template pass (e.g. both an email-template engine and a logging-template engine) — don't stop at the first evaluation found.

| Payload | Engine family it evaluates in |
|---|---|
| `${7*7}` | Java EL (JSP/Spring), Velocity, FreeMarker-family |
| `{{7*7}}` | Jinja2, Twig, Handlebars-family (strict Handlebars returns it literal/escaped) |
| `#{7*7}` | Some Ruby templating contexts (Slim); occasionally Thymeleaf link-expression context |
| `<%= 7*7 %>` | ERB |
| `*{7*7}` / `@{7*7}` | Thymeleaf variable/link expressions, distinct from its also-valid `${}` |
| `{{7*'7'}}` | Jinja2 → `7777777`; Twig → type error — separates Python- from PHP-family `{{}}` engines |

A combined differential string such as `${{<%[%'"}}%\` dropped into one field shows which brackets parse, error, or pass through unmodified, narrowing the candidate engine list before spending attempts on any one of them.

### Parse-time error vs. runtime evaluation — the key diagnostic

Record this distinction explicitly, since engines differ on how they fail:
- A **parse/syntax error** (malformed template, e.g. unmatched `{{`) proves the input reaches the template compiler — confirms injection exists but says nothing about sandboxing.
- A **clean runtime evaluation** (`7*7` renders as `49`) proves expression evaluation is live — the baseline for then testing whether a sandbox blocks attribute access, imports, or calls.
- **Silent neutering** (payload echoed back unevaluated, no error) usually means the field never reaches that engine, or escaping/literal mode applies — rule this out before escalating further payloads in the same field.

### Sandbox-escape direction (Jinja2/Twig-family)

Once arithmetic evaluates, escalate via object introspection rather than guessing a literal payload: walk from an accessible object to its class and that class's loaded subclasses (the general `__class__` → `__mro__`/`__bases__` → `__subclasses__()` chain a sandboxed Python-family evaluator exposes unless hardened), looking for a subclass wrapping an importable/callable primitive already loaded in-process. Treat this as a *category* of technique and a registry to search, not a literal index reusable across targets — the available gadget depends on what's imported in that specific process. The same "walk attributes to reach a callable" category applies conceptually to other reflection-capable engines, with engine-specific attribute names.

### Java engines (FreeMarker / Velocity / Thymeleaf)

- FreeMarker: test whether `freemarker.template.utility.Execute`-style reachability exists via the built-in `new()` — the configured `TemplateClassResolver` policy determines this; a hardened config returns a `TemplateException` naming the restriction.
- Velocity: look for a context exposing `$class`/introspection utilities or a reflection-capable helper object already bound into context.
- Thymeleaf: SpringEL expressions (`${...}`, `*{...}`) can reach Spring's expression evaluator in natural-templating contexts — test whether `T(...)` static-method-call syntax is permitted rather than assuming it's blocked; restrictions vary by version and configuration.

### Ruby / Node (ERB, Handlebars)

- ERB has no sandbox in most common integrations — if a field reaches `ERB.new(input).result`, runtime evaluation is close to full code execution; confirm non-destructively before treating it as such.
- Handlebars has gone through several generations of hardening against compile-time/prototype-pollution-driven escapes; describe what the target's current version actually does (helper injection permitted vs. a compile-time-only sandbox) rather than assuming a specific historical escape still applies — verify empirically against the live response.

## Depth floor

Do not report this surface exhausted until you have tried **at least 25 distinct payload/engine-family combinations** per candidate field — the polyglot probes, each engine-specific escalation variant above, and both the parse-error and runtime-evaluation forms of each. Record every negative result (polyglot didn't evaluate anywhere; evaluated but sandbox held) as a `record_fact`.

## Evidence gate

Before `report_finding`:
- The exact payload submitted and the response showing it was **evaluated**, not merely echoed: literal evaluated arithmetic (e.g. `49` where `{{7*7}}` was submitted) or command output via a non-destructive proof primitive — a DNS/HTTP callback interaction, a `sleep`-based timing delta, or the output of a harmless read-only command (`whoami`, `id`, `echo <marker>`).
- **Never run a destructive command** (file delete/overwrite, service disruption, data exfiltration) to prove SSTI in production — the same non-destructive discipline applies here as for any other code-execution-class primitive.
- Which engine family was fingerprinted and on what basis (polyglot differential, error-message class name, or version banner).
- Tag the finding `vulnclass: SSTI` and bind `traffic_refs`.

## Stop-authority

This skill decides *what SSTI vectors to try* and *how to verify*. It never decides when the entire vulnerability class is exhausted — that stays with `hunting-methodology`.
