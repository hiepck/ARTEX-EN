---
name: source-code-review
description: source-code-review reference material.
---

# Source Code Review Patterns

Part of `skills/hunt-xss/SKILL.md`. Read this ONLY when you have source-code / repository access to the target (Semgrep, ast-grep, ripgrep, and CodeQL patterns for XSS — not for black-box hunting).

---

## Source Code Review Patterns

### Semgrep rules

```yaml
rules:
  - id: xss-react-dangerouslysetinnerhtml-from-input
    pattern-either:
      - pattern: |
          <$EL dangerouslySetInnerHTML={{ __html: $X }} />
    message: |
      dangerouslySetInnerHTML with non-literal value is XSS unless $X is
      sanitized via DOMPurify.sanitize() with current version. Audit every
      occurrence; track $X's origin through the data flow.
    severity: ERROR
    languages: [javascript, typescript]
```

```yaml
rules:
  - id: xss-postmessage-no-origin-check
    pattern: |
      window.addEventListener('message', $HANDLER)
    pattern-not-inside: |
      function ($EVENT) {
        ...
        if ($EVENT.origin === $TRUSTED) { ... }
        ...
      }
    message: |
      postMessage handler without strict origin === check is XSS-eligible.
      .includes() / .endsWith() are also bypassable (CleverTap CVE-2025
      issue #424). Use exact === comparison against an allowlist.
    severity: ERROR
    languages: [javascript, typescript]
```

```yaml
rules:
  - id: xss-innerhtml-from-untrusted
    pattern-either:
      - pattern: $EL.innerHTML = $X
      - pattern: $EL.outerHTML = $X
      - pattern: $EL.insertAdjacentHTML($POS, $X)
      - pattern: document.write($X)
    pattern-not: $EL.innerHTML = "..."  # literal string is OK
    message: |
      Direct HTML assignment from non-literal source is DOM XSS unless
      sanitized. Use textContent for text, or DOMPurify.sanitize() before
      assigning HTML.
    severity: ERROR
    languages: [javascript, typescript]
```

```yaml
rules:
  - id: xss-dompurify-outdated-version
    pattern-regex: '"dompurify"\s*:\s*"\^?[12]\.|"dompurify"\s*:\s*"\^?3\.[01]\.|"dompurify"\s*:\s*"\^?3\.2\.0"'
    message: |
      DOMPurify version <3.1.3 is vulnerable to CVE-2024-47875 nesting mXSS
      and CVE-2024-45801 depth-bypass. Upgrade to current. Version 3.2.1
      with non-default `is` attribute config is also vulnerable per Yaniv
      Nizry Dec 2024 disclosure.
    severity: ERROR
    languages: [json]
    paths:
      include: ['package*.json']
```

```yaml
rules:
  - id: xss-trusted-types-passthrough-policy
    pattern: |
      trustedTypes.createPolicy($NAME, {
        ...
        createHTML: ($INPUT) => $INPUT,
        ...
      })
    message: |
      Trusted Types policy with pass-through createHTML defeats the entire
      protection. Replace with sanitization (DOMPurify) or accept only
      trusted constants.
    severity: ERROR
    languages: [javascript, typescript]
```

```yaml
rules:
  - id: xss-oauth-returnto-no-origin-check
    pattern-either:
      - pattern: location.href = $REQ.query.returnTo
      - pattern: window.location = $REQ.query.return_to
      - pattern: res.redirect($REQ.query.redirect_uri)
    message: |
      Unvalidated redirect from OAuth returnTo/redirect_uri parameter →
      open redirect → XSS via javascript: scheme. CVE-2025-67716 Auth0
      Next.js SDK pattern. Validate scheme (allowlist http/https), validate
      origin (allowlist trusted domains), reject javascript:/data:/vbscript:.
    severity: ERROR
    languages: [javascript, typescript]
```

```yaml
rules:
  - id: xss-llm-output-to-innerhtml
    pattern-either:
      - pattern: |
          $EL.innerHTML = $RESP.choices[0].message.content
      - pattern: |
          $EL.innerHTML = $LLM_RESPONSE
    message: |
      LLM output assigned to innerHTML is XSS via prompt injection. The LLM
      can be coerced (directly or via RAG content) to emit HTML. Use
      textContent or sanitize via DOMPurify with HTML-only allowlist.
    severity: ERROR
    languages: [javascript, typescript]
```

### ast-grep patterns

```bash
# innerHTML from variable (DOM XSS sink)
ast-grep --pattern '$EL.innerHTML = $X' --lang js
ast-grep --pattern '$EL.outerHTML = $X' --lang js

# postMessage handler without origin check
ast-grep --pattern 'addEventListener("message", $H)' --lang js
ast-grep --pattern "addEventListener('message', \$H)" --lang js

# React dangerouslySetInnerHTML
ast-grep --pattern 'dangerouslySetInnerHTML={{ __html: $X }}' --lang tsx
ast-grep --pattern 'dangerouslySetInnerHTML={{ __html: $X }}' --lang jsx

# Trusted Types policy pass-through
ast-grep --pattern 'createHTML: ($X) => $X' --lang js

# eval / new Function with non-literal
ast-grep --pattern 'eval($X)' --lang js
ast-grep --pattern 'new Function($X)' --lang js

# document.location / window.location set from request
ast-grep --pattern 'location.href = $X' --lang js
```

### ripgrep one-liners

```bash
# Every dangerous sink
rg -n -e '\.innerHTML\s*=' -e '\.outerHTML\s*=' -e 'document\.write\(' \
   -e 'insertAdjacentHTML\(' -e '\beval\(' -e 'new\s+Function\(' \
   --type js --type ts --type jsx --type tsx

# postMessage handlers — review origin checks
rg -n -B 2 -A 20 "addEventListener\\(['\"]message['\"]" --type js --type ts

# React dangerous HTML
rg -n 'dangerouslySetInnerHTML' --type js --type ts --type jsx --type tsx

# Vue v-html (equivalent danger)
rg -n 'v-html\s*=' --type vue --type html

# Angular bypassSecurityTrust
rg -n 'bypassSecurityTrust(?:Html|Url|Script|Style|ResourceUrl)' --type ts

# DOMPurify version pin
rg -n '"dompurify"' -g 'package*.json'

# Trusted Types policies
rg -n 'trustedTypes\.createPolicy' --type js --type ts

# OAuth redirect_uri / returnTo handling
rg -n -i -e 'redirect_uri' -e 'returnTo' -e 'return_to' -e 'RelayState' \
   --type js --type ts --type py --type java --type go

# LLM response → DOM
rg -n -B 2 -A 5 -e 'choices\[0\]\.message\.content' -e 'llm_response' \
   --type js --type ts | rg 'innerHTML|dangerouslySetInnerHTML'

# JSONP callback (CSP bypass surface)
rg -n -e 'callback=' -e '\.jsonp\(' --type js --type ts

# Markdown renderers in package.json (potentially outdated)
rg -n -e '"marked"' -e '"markdown-it"' -e '"markdown-to-jsx"' -e '"remark"' \
   -g 'package*.json'
```

### CodeQL hint

GitHub's pre-built `js/xss` query (`Security/CWE-079/Xss.ql`) is the standard CodeQL XSS analysis. Sources include `RemoteFlowSource` (URL params, request body, postMessage), sinks include `XssSink` (innerHTML, document.write, dangerouslySetInnerHTML, eval, plus Function constructor and similar). Run via `codeql database analyze --format=sarif-latest --output=xss.sarif`.

For Trusted Types bypass detection, write a custom predicate that flags `createHTML` callbacks where the input parameter flows directly to the return value without sanitization. Reference: PortSwigger Research has published several CodeQL patterns at portswigger.net/research.

For postMessage XSS specifically, GitHub's `js/dom/wrong-document-prepended` and `js/dom/wrong-document-written` queries cover related patterns. The bananabr GitHub Security Lab issues track newer patterns including JS prototype pollution gadget detection.

