---
name: source-code-review
description: source-code-review reference material.
---

# Source Code Review Patterns

Part of `skills/hunt-oauth/SKILL.md`. Read this ONLY when you have source code / repository access to the target (Semgrep, ast-grep, ripgrep, CodeQL).

---

## Source Code Review Patterns

### Semgrep rules (≥3, copy-pasteable YAML)

```yaml
rules:
  - id: oauth-state-not-validated
    pattern-either:
      - pattern: $REQ.query.code
      - pattern: $REQ.query['code']
    pattern-not-inside: |
      if (... $REQ.query.state ...) { ... }
    message: |
      OAuth callback handler reads `code` without validating `state` parameter.
      Per RFC 6749 §10.12, state MUST be validated against the value stored
      at flow initiation. Missing this check = CSRF on OAuth linking, leading
      to account takeover (bugbounty.ch May 2023, H1 #850022 launchpad CSRF).
    severity: ERROR
    languages: [javascript, typescript]
```

```yaml
rules:
  - id: jwt-decode-without-algorithm-pin
    pattern-either:
      - pattern: jwt.verify($TOKEN, $SECRET)
      - pattern: jwt.decode($TOKEN, verify=False)
      - pattern: jws.verify($TOKEN, $SECRET)
    message: |
      JWT verification without explicit `algorithms: ["RS256"]` allowlist.
      Vulnerable to alg=none (CVE-2025-61152), RS256→HS256 confusion
      (CVE-2024-37568, CVE-2026-22817 Hono CVSS 8.2). Always pin algorithms
      explicitly: jwt.verify(token, key, {algorithms: ["RS256"]}).
    severity: ERROR
    languages: [javascript, typescript, python]
```

```yaml
rules:
  - id: oauth-email-as-user-identifier
    pattern-either:
      - pattern: User.findOne({email: $CLAIMS.email})
      - pattern: User.find_or_create_by(email: $CLAIMS.email)
      - pattern: db.users.where(email: $CLAIMS["email"]).first
    message: |
      Email claim used as primary user identifier. Vulnerable to nOAuth
      (CVE-2024-21632 omniauth-microsoft_graph, Descope 2023). Microsoft
      Entra emits unverified email claims by default for pre-June-2023 apps.
      Use the immutable `sub` claim or Entra's `oid` claim instead.
      For Microsoft tokens, also check `xms_edov` claim before trusting email.
    severity: ERROR
    languages: [ruby, javascript, typescript, python, go]
```

```yaml
rules:
  - id: oauth-redirect-uri-substring-validation
    pattern-either:
      - pattern: $URI.startsWith($ALLOWED)
      - pattern: $URI.includes($ALLOWED)
      - pattern: $ALLOWED in $URI
      - pattern: re.match(r'.*' + $ALLOWED + '.*', $URI)
    metavariable-regex:
      metavariable: $URI
      regex: .*redirect.*
    message: |
      redirect_uri validated by substring match. Bypass with userinfo
      `https://allowed.com@attacker.com`, IDN homograph, or subdomain
      `https://allowed.com.attacker.com`. Per RFC 6749 §3.1.2.2, exact-match
      against pre-registered URIs is required. Authentik CVE-2024-52289
      (regex with unescaped dot) is the disclosed real-world example.
    severity: ERROR
    languages: [javascript, typescript, python, ruby, go]
```

```yaml
rules:
  - id: oidc-code-not-bound-to-client
    pattern: |
      def $TOKEN_HANDLER(...):
        ...
        $ENTRY = code_storage.get($CODE)
        ...
        return generate_token($ENTRY.user, ...)
    pattern-not: |
      if $ENTRY.client_id != $REQ.client_id: ...
    message: |
      Authorization code redeemed without verifying the redeeming client
      matches the client the code was issued to. Per RFC 6749 §4.1.3,
      this binding is mandatory. CVE-2026-32245 tinyauth had this exact
      bug — refresh-token flow had the check, auth-code flow did not.
    severity: ERROR
    languages: [python, go, ruby, javascript]
```

```yaml
rules:
  - id: saml-signature-validate-discarded
    pattern-either:
      - pattern: $OBJ.validateSignature(...)
      - pattern: validate_signature(...)
    pattern-not-inside: |
      $RESULT = ...
    message: |
      SAML signature validation result discarded — Admidio CVE-style
      (GHSA-25cw-98hg-g3cg). validateSignature() returns string-on-error
      instead of throwing, so unsigned/forged AuthnRequests slip through.
      Always assign the result and check explicitly: `valid = validate(...)`.
    severity: ERROR
    languages: [php, python, ruby, javascript]
```

### ast-grep patterns (≥3, language-tagged)

```bash
# A.1 Detect missing audience verification on OIDC ID Token
ast-grep --pattern 'jwt.decode($TOKEN, $KEY, $$$OPTS)' --lang javascript \
  | rg -L 'audience|aud:'

# A.2 Detect Microsoft Graph email-claim trust (nOAuth CVE-2024-21632 family)
ast-grep --pattern 'token.User{ID: $$$, Email: $EMAIL}' --lang go

# A.3 Detect OAuth redirect_uri loose comparison
ast-grep --pattern '$URI.startsWith($PREFIX)' --lang javascript \
  | rg redirect_uri

# A.4 Detect SAML response handler reading assertion[0] (multi-assertion injection)
ast-grep --pattern 'assertion[0]' --lang typescript

# A.5 Detect Hono JWK middleware without explicit alg allowlist (CVE-2026-22818)
ast-grep --pattern 'jwk({ jwks_uri: $URI })' --lang typescript

# A.6 Detect PKCE bypass — code_verifier not required when code_challenge present
ast-grep --pattern 'if ($CODE_VERIFIER) { verify_pkce($$$) }' --lang python
```

### ripgrep one-liners (≥3)

```bash
# R.1 OAuth callback handlers missing state validation
rg -n '/oauth/callback|/auth/callback|/oidc/callback' --type js --type py --type rb -l \
  | xargs rg -L 'state'

# R.2 JWT libraries with verify=False (Python/Node/Go)
rg -n 'verify=False|noVerify|skipVerification|verify:\s*false' --type py --type js --type ts --type go

# R.3 Hardcoded OAuth client_secret in frontend bundle (CRA / Vite / Next.js)
rg -n 'REACT_APP_.*(SECRET|KEY)|VITE_.*(SECRET|KEY)|NEXT_PUBLIC_.*(SECRET|KEY)' \
  -g 'package.json' -g '*.env*' -g 'src/**' -g 'public/**'

# R.4 Mutable email claims used as account key
rg -n 'find_by.*email|findOne.*email|getOrCreate.*email|find_or_create_by.*email' \
  --type rb --type js --type py --type go | rg -i 'oauth|oidc|sso|claim|userinfo'

# R.5 SAML signature handlers using REXML + Nokogiri together (parser differential class)
rg -n "require ['\"]rexml" --type rb -l | xargs rg -l "require ['\"]nokogiri"

# R.6 OAuth implicit flow / response_type=token enabled
rg -n "response_type[\"']?\s*[:=]\s*[\"']?token|allow_implicit_flow\s*[:=]\s*true|allowImplicitFlow:\s*true" \
  --type js --type ts --type py --type rb

# R.7 OIDC nonce never checked
rg -n 'id_token' --type js --type py --type go -l | xargs rg -L 'nonce'

# R.8 Authorization code redeemed without client binding (CVE-2026-32245 tinyauth pattern)
rg -n 'authorization_code' --type go --type js --type py --type rb | rg -B5 -A20 'token' | rg -L 'client_id'
```

### CodeQL hint

CodeQL has shipped tutorials for OAuth/SAML auth bypass classes — the relevant base queries are `js/insufficient-key-size`, `js/jwt-missing-algorithm-restriction`, and `py/jwt-missing-algorithm`. Sketch a custom predicate for nOAuth: a `RemoteFlowSource` reaching a `User.findBy(email=...)` sink without an intermediate verification of `email_verified` or `xms_edov` claim should raise a query. The GitHub Security Lab `securitylab.github.com/advisories/GHSL-2024-329_GHSL-2024-330_ruby-saml` writeup documents the parser-differential predicate that found CVE-2025-25291 — adapt it as a generic `XmlParser1.parse(input).descendant() != XmlParser2.parse(input).descendant()` taint flow.

