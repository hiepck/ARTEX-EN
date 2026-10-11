---
name: source-code-review-patterns
description: source-code-review-patterns reference material.
---

# Source-code review patterns for info-disclosure

Part of `skills/hunt-info-disclosure/SKILL.md`. Read this ONLY when you have filesystem or repo access to the target's source (white-box engagement, OSS project, or a recovered `.git` dump) — never for black-box hunting.

---

## Source Code Review Patterns

### Semgrep rules

```yaml
rules:
  - id: info-spring-actuator-exposure
    pattern-either:
      - pattern-regex: 'management\.endpoints\.web\.exposure\.include\s*=\s*\*'
      - pattern-regex: 'management\.endpoints\.web\.exposure\.include\s*=\s*[^=]*(?:env|heapdump|gateway|threaddump|beans)'
    message: |
      Spring Boot Actuator endpoint exposure includes high-risk endpoint
      (env, heapdump, gateway, threaddump, beans). CVE-2025-41243 Spring
      Cloud Gateway property modification (CVSS 10.0), CVE-2025-41253
      info disclosure (CVSS 7.5). Restrict via management.endpoints.web.
      exposure.include=health,info only; secure others via Spring Security.
    severity: ERROR
    languages: [yaml, java]
    paths:
      include: ['application*.{yml,yaml,properties}']
```

```yaml
rules:
  - id: info-debug-mode-enabled-prod
    pattern-either:
      - pattern-regex: 'DEBUG\s*=\s*True'
      - pattern-regex: 'app\.config\[.DEBUG.\]\s*=\s*True'
      - pattern-regex: 'debug:\s*true'
      - pattern-regex: '"debug":\s*true'
    message: |
      Debug mode enabled. Stack traces, framework version, file paths
      will be exposed in error responses. Set DEBUG=False in production.
      Use environment-specific config (settings.production.py for Django,
      RAILS_ENV=production for Rails, NODE_ENV=production for Express).
    severity: WARNING
    languages: [python, javascript, yaml]
```

```yaml
rules:
  - id: info-hardcoded-credentials
    pattern-either:
      - pattern-regex: '(?i)(password|passwd|pwd|secret|api[_-]?key|access[_-]?key|access[_-]?token)\s*[:=]\s*["\'][^"\']{8,}["\']'
      - pattern-regex: 'AKIA[0-9A-Z]{16}'
      - pattern-regex: 'AIza[0-9A-Za-z_-]{35}'
      - pattern-regex: 'sk_live_[0-9a-zA-Z]{24,}'
      - pattern-regex: 'ghp_[0-9a-zA-Z]{36}'
    message: |
      Hardcoded credential detected. CWE-798 Use of Hard-coded Credentials.
      Move to environment variables, secret managers (AWS Secrets Manager,
      HashiCorp Vault, GCP Secret Manager). Reference: GitGuardian 2026
      State of Secrets Sprawl — 28.65M secrets in public repos in 2025.
      Starbucks H1 #716292 paid bounty for single hardcoded JumpCloud key
      (CVSS 9.7).
    severity: ERROR
    languages: [python, javascript, typescript, java, go, ruby, php]
```

```yaml
rules:
  - id: info-pprof-handler-exposed
    pattern-either:
      - pattern: 'import _ "net/http/pprof"'
      - pattern: 'http.ListenAndServe(":6060", nil)'
      - pattern-regex: 'pprof\.(?:Index|Cmdline|Profile|Symbol|Trace|Handler)'
    message: |
      Go net/http/pprof exposed. /debug/pprof/cmdline leaks process
      command line including secrets passed via --flag="value".
      Dgraph GHSA-95mq-xwj4-r47p (2026 critical) is canonical example.
      Restrict to internal-only mux or remove import in production builds.
    severity: ERROR
    languages: [go]
```

```yaml
rules:
  - id: info-error-handler-leaks-stack
    pattern-either:
      - pattern: |
          app.use(function(err, req, res, next) {
            res.send(err.stack);
          })
      - pattern: |
          @app.errorhandler($CODE)
          def $H($E):
            return str($E)
    message: |
      Error handler returns full stack trace / exception details to client.
      Log internally, return generic error message externally. Stack-trace
      disclosure leaks framework version, file paths, internal logic.
    severity: ERROR
    languages: [javascript, typescript, python]
```

```yaml
rules:
  - id: info-over-fetching-sensitive-fields
    pattern-either:
      - pattern: |
          $MODEL.findAll()
      - pattern: |
          $MODEL.find({})
      - pattern: |
          $MODEL.objects.all()
      - pattern: |
          $REPO.findAll()
    message: |
      Wildcard model fetch returns ALL fields including sensitive ones
      (password_hash, tokens, internal_notes). Use explicit field selection
      or serializer with field allowlist. Over-fetching is mid-four-figure
      info-disclosure on most programs.
    severity: WARNING
    languages: [javascript, typescript, python, java]
```

```yaml
rules:
  - id: info-username-enumeration-error-delta
    pattern-either:
      - pattern: |
          if not user:
            return "User not found"
          if not check_password(user, $PWD):
            return "Invalid password"
      - pattern: |
          if (user == null) return "User not found";
          if (!checkPassword($PWD)) return "Invalid password";
    message: |
      Different error messages for "user not found" vs "wrong password"
      enable username enumeration. Return same generic message for both.
      MinIO GHSA-jv87-32hw-hh99 LDAP brute-force pattern.
    severity: WARNING
    languages: [python, javascript, typescript, java]
```

### ast-grep patterns

```bash
# Hardcoded AWS access keys
ast-grep --pattern '"AKIA$$$"' --lang python
ast-grep --pattern '"AKIA$$$"' --lang js

# Spring actuator exposure config
ast-grep --pattern 'management.endpoints.web.exposure.include = $X' --lang yaml

# Go pprof import
ast-grep --pattern 'import _ "net/http/pprof"' --lang go

# Express error handler exposing stack
ast-grep --pattern 'res.send($E.stack)' --lang js
ast-grep --pattern 'res.json($E)' --lang js

# Django DEBUG setting
ast-grep --pattern 'DEBUG = True' --lang python

# Rails verbose errors
ast-grep --pattern 'config.consider_all_requests_local = true' --lang ruby
```

### ripgrep one-liners

```bash
# Spring actuator exposure
rg -n 'management\.endpoints\.web\.exposure\.include' -g 'application*.{yml,yaml,properties}'
rg -n 'spring-boot-starter-actuator' -g 'pom.xml' -g '*.gradle*'

# .env files in repo (should be gitignored)
rg --files | rg -e '\.env$' -e '\.env\.local$' -e '\.env\.production$'

# Hardcoded secret patterns (AWS, Google, Stripe, GitHub, JWT)
rg -n -e 'AKIA[0-9A-Z]{16}' -e 'AIza[0-9A-Za-z_-]{35}' -e 'sk_live_[0-9a-zA-Z]{24,}' \
   -e 'ghp_[0-9a-zA-Z]{36}' -e 'github_pat_[0-9a-zA-Z]{82}' \
   -e 'eyJhbGciO[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+' \
   -g '!*.lock' -g '!node_modules' -g '!vendor' -g '!.git'

# Database connection strings with embedded creds
rg -n -i 'jdbc:.*://[^"\s]*:[^@\s]+@'
rg -n -i 'mongodb(\+srv)?://[^"\s]*:[^@\s]+@'
rg -n -i 'postgres(?:ql)?://[^"\s]*:[^@\s]+@'

# Go pprof exposure
rg -n -e 'net/http/pprof' -e '"/debug/pprof"' --type go

# DEBUG flags enabled
rg -n -i -e 'debug\s*[=:]\s*true' -g '*.{yml,yaml,toml,ini,env,properties,json}'

# Verbose error handlers
rg -n -e 'res\.send\(.*err\.stack' -e '@app\.errorhandler' --type js --type ts --type py

# Console.log with sensitive var names
rg -n -e 'console\.log\(.*password' -e 'console\.log\(.*token' -e 'console\.log\(.*secret' \
   --type js --type ts

# Comments containing developer-action markers next to credential references
rg -n -e 'remove.*before.*deploy.*credential' \
   -e 'FIXME.*password' \
   -e 'remove.*key.*before.*push' \
   --type-add 'all:*'
```

### CodeQL hint

GitHub's pre-built `js/hardcoded-credentials` (`Security/CWE-798/HardcodedCredentials.ql`), `py/hardcoded-credentials`, `java/hardcoded-credentials`, and `go/hardcoded-credentials` queries cover the literal-credential pattern. Run via `codeql database analyze --format=sarif-latest --output=info-disclosure.sarif`.

For Spring actuator exposure, GitHub Security Lab maintains custom queries at github.com/github/codeql for Java; search for `cwe-200` in the query catalog. Wiz Threat Research published patterns for actuator endpoint detection.

For Go pprof exposure, write a custom predicate that flags `import _ "net/http/pprof"` in any non-test file. Reference: https://blog.detectify.com/2024/10/30/google-pprof-go-debug-information-leakage/ for the broader detection pattern.

For mobile (Android Smali / iOS Mach-O), use **MobSF** for static analysis with built-in secret-detection rules.

