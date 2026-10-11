---
name: source-code-review
description: source-code-review reference material.
---

# IDOR — Source Code Review Patterns

> **Part of `skills/hunt-idor/SKILL.md`.** Read this ONLY when you have source/repo access (OSS bug, internal pentest, in-scope GitHub org). Not useful for pure black-box hunting.

---

## Source Code Review Patterns

When you have repo access (OSS bug, internal pentest, in-scope GitHub org), grep is faster than dynamic testing.

### Semgrep rules (paste into `.semgrep.yml`)

```yaml
rules:
  - id: idor-mongoose-findbyid-no-tenant-filter
    pattern-either:
      - pattern: |
          $MODEL.findById($ID)
      - pattern: |
          $MODEL.findOne({_id: $ID})
    pattern-not-inside: |
      $MODEL.find($X({_id: ..., _organizationId: ..., ...}))
    message: |
      Mongoose findById/findOne by _id only does NOT filter by tenant.
      See Novu GHSA-323c-xqcq-fpcp — the fix was to add _organizationId
      to every findById call. Mandatory: include tenant identifier in
      every cross-tenant repository query.
    severity: ERROR
    languages: [javascript, typescript]
```

```yaml
rules:
  - id: idor-uuidv1-token-generation
    pattern-either:
      - pattern: uuid.uuid1()
      - pattern: uuid1()
      - pattern-regex: 'uuidv1\(\)|UUID\.fromString.*Type\.TIME_BASED'
    message: |
      UUIDv1 is timestamp-based and predictable. Apache Answer CVE-2024-45719
      and GitHub Security Lab issue #816 (bananabr) catch this pattern with
      CodeQL. Tokens generated this way can be predicted by adjacent timestamp
      arithmetic. Use UUIDv4 (random) for any token, password reset, share
      link, or session identifier.
    severity: ERROR
    languages: [python, javascript, typescript, java]
```

```yaml
rules:
  - id: idor-scim-update-body-id-override
    pattern: |
      def update($PATH_ID, $BODY):
        ...
        $RESOURCE = parse($BODY)
        ...
        return $PROVIDER.update($RESOURCE.id, ...)
    message: |
      SCIM PUT pattern from Keycloak issue #46658 — handler validates
      URL path $PATH_ID exists then calls update() with body's $RESOURCE.id.
      Add explicit check: if body.id != path.id, reject with 400.
    severity: ERROR
    languages: [java, python, javascript]
```

```yaml
rules:
  - id: idor-graphql-resolver-no-context-check
    pattern-either:
      - pattern: |
          @ResolveField('$F')
          $F($PARENT, $ARGS) { return $REPO.find($ARGS.id) }
      - pattern: |
          $F: ($PARENT, $ARGS, $CTX) => $REPO.find($ARGS.id)
    pattern-not-regex: 'context\.user|context\.auth|requireAuth|@AuthGuard|isOwner|isAdmin'
    message: |
      GraphQL resolver returns object by ID without checking context.user
      ownership/permission. See HackerOne $12.5k Dec 2025 case (Harshdranjan)
      and tinopreter $1500 Feb 2026 case for nested-pivot exploitation.
      Add context.user check at every resolver, plus field-level auth via
      @AuthField directives or per-field guards.
    severity: ERROR
    languages: [javascript, typescript]
```

```yaml
rules:
  - id: idor-mass-assignment-spread-body
    pattern-either:
      - pattern: |
          $MODEL.update({...$REQ.body})
      - pattern: |
          $MODEL.findByIdAndUpdate($ID, $REQ.body)
      - pattern: |
          await $MODEL.update($REQ.body, {where: {...}})
    message: |
      Mass-assignment sink — entire request body flows to model update without
      field allowlist. Attacker submits `role`, `is_admin`, `tenant_id` and similar.
      Use explicit pick(): _.pick(req.body, ['display_name', 'avatar']).
    severity: ERROR
    languages: [javascript, typescript]
```

```yaml
rules:
  - id: idor-tenant-id-from-header-not-jwt
    pattern-either:
      - pattern-regex: 'req\.headers\[.tenantid.\]|req\.header\(.tenantid.\)|request\.headers\.get\(.tenantid.\)'
      - pattern-regex: 'req\.headers\[.x-tenant-id.\]|req\.headers\[.x-org-id.\]|req\.headers\[.is-multi-tenant'
    message: |
      Tenant identity is derived from request header, not from JWT/session
      claim. Client-controlled tenant context is BOLA — see CVE-2026-30956
      OneUptime, OnSecurity Tenantid disclosure. Derive tenant server-side
      from authenticated session or signed JWT claim only.
    severity: ERROR
    languages: [javascript, typescript, python]
```

### ast-grep patterns

```bash
# Mongoose findById without organization filter (Novu-pattern)
ast-grep --pattern '$MODEL.findById($ID)' --lang js
ast-grep --pattern '$MODEL.findOne({_id: $ID})' --lang js

# Sequelize findByPk without scoping
ast-grep --pattern '$MODEL.findByPk($ID)' --lang js

# Django queryset by pk only
ast-grep --pattern '$MODEL.objects.get(pk=$ID)' --lang python
ast-grep --pattern '$MODEL.objects.filter(id=$ID)' --lang python

# Rails ActiveRecord find without scope
ast-grep --pattern '$MODEL.find($ID)' --lang ruby

# JPA/Hibernate findById without tenant
ast-grep --pattern '$REPO.findById($ID)' --lang java

# Express handler reading id from request body without validation
ast-grep --pattern 'req.body.id' --lang js

# UUIDv1 calls
ast-grep --pattern 'uuid.uuid1()' --lang python
ast-grep --pattern 'UUID.randomUUID()' --lang java -A 3

# Mongoose update with spread body (mass assignment)
ast-grep --pattern '$M.findByIdAndUpdate($ID, {...$BODY})' --lang js
```

### ripgrep one-liners

```bash
# Tenant identity coming from request headers (OneUptime CVE-2026-30956 family)
rg -n -i 'req\.headers\[.(tenantid|tenant-id|x-tenant-id|x-org-id|x-project-id|environmentid|is-multi-tenant)' \
   --type js --type ts --type py --type go

# MongoDB queries by _id only (no _organizationId filter — Novu pattern)
rg -n 'findById\(|findOne\(\{[^}]*_id[^}]*\}' --type js --type ts | rg -v '_organizationId|_orgId|_tenantId|organization:|orgId:'

# SQL queries WHERE id without tenant_id (WeKnora pattern)
rg -n -B 2 -A 1 'WHERE\s+id\s*=' --type py --type java --type rb --type sql | rg -v 'tenant_id|org_id|user_id'

# UUIDv1 token-generation calls (CVE-2024-45719 family)
rg -n 'uuid\.uuid1\(\)|uuidv1\(\)|UUID\.fromString.*1xxx' --type py --type js --type java

# GraphQL resolvers without auth context check
rg -n -B 2 -A 8 '@Query\(|@Resolver\(|@ResolveField\(' --type ts --type js | rg -v 'context\.user|@AuthGuard|requireAuth|isOwner'

# Express body spread into model update (mass assignment)
rg -n '\.\.\.req\.body|\.\.\.body|spread.*body' --type js --type ts

# SCIM PUT handlers (Keycloak issue #46658 path-body mismatch)
rg -n -B 5 -A 10 'PUT.*scim|@PUT.*Users/{id}' --type java --type ts --type py

# Path-vs-body id mismatch (general)
rg -n -B 3 -A 10 '@PathParam.*id.*@RequestBody' --type java
rg -n -B 3 -A 10 'req\.params\.id.*req\.body\.id' --type js

# JWT signature not verified
rg -n 'jwt\.decode\(' --type js --type py | rg -v 'verify|jwt\.verify'

# Tenant fields hardcoded with wildcard scope
rg -n -i '"tenant_id"\s*:\s*null|tenant_id=\*|allTenants:\s*true' --type js --type py --type yaml
```

### CodeQL hint

The bananabr CodeQL queries from GitHub Security Lab issue #816 (linked to HackerOne #2513301 paid bounty) detect UUIDv1 token-generation patterns systematically. Use them for any in-scope OSS audit. The queries identify sinks where `uuid.uuid1()` (Python) or `uuidv1()` (Node) flow into a token attribute, password-reset field, or share-link generator.

For BOLA detection more broadly, write a custom CodeQL predicate (sketch):

```ql
import javascript
import semmle.javascript.security.dataflow.flow

class BolaConfig extends TaintTracking::Configuration {
  BolaConfig() { this = "BolaConfig" }
  override predicate isSource(DataFlow::Node src) {
    // request URL params and body fields
    src.asExpr() = any(HTTP::RequestInputAccess in)
  }
  override predicate isSink(DataFlow::Node sink) {
    exists(MethodCallExpr c |
      c.getMethodName() = ["findById", "findOne", "findByPk", "find"] and
      c.getAnArgument() = sink.asExpr() and
      // No tenant filter in the same call
      not c.getAnArgument().toString().regexpMatch(".*(_organizationId|tenantId|orgId).*")
    )
  }
}
```

