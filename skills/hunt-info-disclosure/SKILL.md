---
name: hunt-info-disclosure
description: Hunting skill for Information Disclosure / Sensitive Data Exposure (CWE-200 / CWE-209 / CWE-215 / CWE-538 / CWE-668 / CWE-798). Built from 106 corpus reports plus 8K shared-platform reports across HackerOne, Bugcrowd, Huntr, GitHub Security Advisories, plus 2024-2026 meta verified against NVD — Spring Boot Actuator family (CVE-2025-41253 SpEL info-disclosure CVSS 7.5, CVE-2025-41243 Spring Cloud Gateway property modification CVSS 10.0, CVE-2025-22235 EndpointRequest.to wrong matcher CVSS 7.3, CVE-2025-8525 Exrick xboot, CVE-2025-8738 microservices-platform), .git/.env mass exposure (Sysdig EmeraldWhale 15K cloud creds Oct 2024, Unit42 110K domain .env scan Aug 2024), Spring Boot Actuator heapdump → 9TB GPS data Volkswagen disclosure (Wiz Threat Research Dec 2024), debug endpoint family (Dgraph /debug/pprof GHSA-95mq-xwj4-r47p, MinIO LDAP brute-force GHSA-jv87-32hw-hh99, Glances /api/4/serverslist GHSA-r297-p3v4-wp8m, FUXA plaintext DB creds GHSA-c5gq-4h56-4mmx, Harbor default password GHSA-hj7x-hmf2-hc2p, NetBird VPN default admin GHSA-g3j4-58mp-3x25, PraisonAI WebSocket no-auth GHSA-cfh6-vr3j-qc3g, Gradio ACL bypass GHSA-j2jg-fq62-7c3h, Rancher cluster template credentials, ArgoCD Redis cache crypto), and the secrets-in-repo wave (GitGuardian 2026 State of Secrets: 28.65M new hardcoded secrets in 2025, GitHub 2024 secret-scanning report: 39M leaks). Use when hunting exposed credentials, leaked API keys, .git/.env files, debug endpoints, Spring actuator endpoints, S3 bucket misconfig, source-code disclosure, stack trace leakage, user/email enumeration, PII via API, server-side debug surfaces (phpinfo, /server-status), or any Confidentiality-impact-only finding.
sources: hackerone_public, github_advisories, github_deep, intigriti, huntr, bugcrowd, project_zero, microsoft_msrc, securitylab_github, nvd_verified, sysdig_threat_research, wiz_threat_research, gitguardian_research, trufflesecurity_research, palo_alto_unit42
report_count: 8298
generated_at: 2026-05-04
---

## Deeper reference (load only if this specific situation applies)

This file covers the methodology and patterns that apply to almost every information-
disclosure hunt. 5 topics were moved out because they only apply under a specific
condition -- read the matching file ONLY when that condition is true for the
current target, never by default:

- **`references/spring-boot-actuator.md`** -- the target fingerprints as a Spring Boot
  service (`Server:` header shows Tomcat/Jetty/Undertow, an `X-Application-Context`
  header is present, or `/actuator/health` returns 200). Covers the full Actuator
  endpoint exploit sequence (heapdump, env, gateway SpEL/CVE-2025-41243/41253/22235)
  and heap-dump/memory-dump secret extraction.
- **`references/source-code-review-patterns.md`** -- you have filesystem or repo
  access to the target's source (white-box engagement, OSS project, or a recovered
  `.git` dump). Semgrep/ast-grep/ripgrep/CodeQL rules for hardcoded credentials,
  actuator misconfig, debug-mode flags, and verbose error handlers. Never applies
  to pure black-box hunting.
- **`references/modern-expansion-platforms.md`** -- the target runs one of: shared-PID
  containers, an ML model-serving platform (MLflow/Hugging Face/BentoML), an agentic
  LLM/MCP tool-use surface, Next.js (RSC/Server Actions specifically), or a GitOps/K8s
  admission pipeline (ArgoCD/Tekton/Helm/kustomize). Narrow platform-specific
  info-disclosure deep-dives.
- **`references/chain-templates.md`** -- you already have ONE confirmed info-disclosure
  primitive and want to escalate it. Eight disclosed-pattern chain templates (actuator
  heapdump → AWS keys → S3 exfil, `.git` → trufflehog → live creds, etc.) with
  hunter's-note framing for each.
- **`references/case-studies.md`** -- you want full narrative write-ups of disclosed
  bugs for report-framing inspiration (Volkswagen, EmeraldWhale, ESHYFT, Dgraph, NASA,
  Glances). Not needed to find or validate the first bug.

If the core methodology below does not resolve the hunt and the target matches
one of the conditions above, read the matching reference file before declaring
the surface exhausted.

## Crown Jewel Targets

Information disclosure is the most-volume bug class in bug bounty (~30% of all disclosed reports across H1/Bugcrowd hacktivity), but the paying surface has shifted decisively toward **systemic credential exposure** rather than one-off PII leaks. Reflected stack traces on a 404 page are mid-three-figure or N/A on most programs; an exposed `/actuator/heapdump` containing AWS keys is mid-five-figure-class because it cascades to RCE on production cloud infrastructure. The 24-month meta crystallized around seven asset types. All CVEs below are NVD-verified.

**1. Spring Boot Actuator exposure (mid four-figure to mid five-figure when chained to cloud takeover).** Wiz Threat Research Dec 2024 analysis: **60% of cloud environments use Spring Boot Actuator, 11% expose instances publicly to the internet, 24% of exposed instances are misconfigured.** **Volkswagen 9TB GPS data disclosure** — single open `/actuator/heapdump` endpoint provided plaintext AWS keys via heap dump, attackers downloaded 9 TB of GPS data from hundreds of thousands of cars. Reference: https://www.syscrest.com/2025/02/securing-spring-boot-actuator/ (SYSCREST analysis Feb 2025), https://www.cyberkendra.com/2024/12/vulnerability-in-spring-boot-actuator.html (Wiz research summary Dec 2024). NVD-verified CVEs in the family:
- **CVE-2025-41243 Spring Cloud Gateway Server Webflux property modification (CVSS 10.0 CRITICAL)** — when actuator gateway endpoint exposed via `management.endpoints.web.exposure.include=gateway`, attackers modify Spring Environment properties remotely → potential RCE. Affects 4.3.x, 4.2.x, 4.1.x, 4.0.x, 3.1.x. Fix in 4.3.1 / 4.2.5 (OSS); 4.1.11 / 3.1.11 (Enterprise).
- **CVE-2025-41253 Spring Cloud Gateway info-disclosure (CVSS 7.5 HIGH)** — SpEL injection on actuator endpoint exposes environment variables and system properties (DB credentials, API keys, internal URLs).
- **CVE-2025-22235 Spring Boot EndpointRequest.to() wrong matcher (CVSS 7.3 HIGH)** — disabled actuator endpoint creates unprotected `/null/**` matcher path.
- **CVE-2025-8525 Exrick xboot Spring Boot Admin/Actuator info disclosure** (CVSS 5.5 MEDIUM).
- **CVE-2025-8738 zlt2000 microservices-platform actuator interface** (CVSS 5.5 MEDIUM).

The Wiz dashboard checklist for actuator endpoints: `/health` (low), `/prometheus` `/metrics` (medium), `/env` (HIGH — never expose), `/heapdump` (CRITICAL — never expose), `/beans` `/mappings` `/loggers` (high — internal/auth only), `/shutdown` (HIGH — disabled by default, never enable).

**2. `.git/` and `.env` mass exposure (mid four-figure direct + supply-chain cascades).** Two large 2024-2025 disclosures show this is industrialized:
- **Sysdig EmeraldWhale Oct 2024** — automated scanning of IP ranges for exposed `/.git/config` files. Stole **15,000 cloud credentials from 67,000 URLs** (28K Git repos, 6K GitHub tokens, 2K validated active credentials). Stored stolen secrets in 1TB S3 bucket. Reference: https://sysdig.com/blog/emeraldwhale, https://www.bleepingcomputer.com/news/security/hackers-steal-15-000-cloud-credentials-from-exposed-git-config-files/.
- **Unit42 (Palo Alto) Aug 2024** — large-scale .env extortion campaign scanned 110,000 domains, identified **90,000 unique combos of leaked env-vars including 7,000 active AWS access keys**. Targeted Mailgun-mentioning .env files for legitimate-domain phishing. Reference: https://unit42.paloaltonetworks.com/large-scale-cloud-extortion-operation.

Pattern: web servers serving Laravel, Symfony, Rails, Django apps mistakenly serve the application root including `.env` and `.git/`. Hunt with: `curl -s https://target/.env`, `curl -s https://target/.git/config`, `curl -s https://target/.git/HEAD`. Then `git-dumper https://target/.git/ /tmp/dumped` to recover the full repo.

**Disclosed bug bounty cases**: NASA `_x3ro_` Bugcrowd disclosure (Aug 2025, P3) — publicly accessible `.env` on NASA Bitbucket exposed UAT credentials for `cmr.sit.earthdata.nasa.gov`. Reference: https://redpacketsecurity.com/bugcrowd-bugbounty-disclosure-publicly-accessible-env-file-exposing-hardcoded-credentials-on-nasa-s-git-repository.

**3. Source-code repository secret leakage (CWE-798, low five-figure on triage-friendly programs).** **GitGuardian 2026 State of Secrets Sprawl**: 28.65M new hardcoded secrets added to public GitHub repos in 2025 (34% YoY increase). **GitHub's own 2024 secret-scanning report**: 39M secret leaks. **IEEE S&P 2025 academic study**: up to 30% of projects at risk. **Starbucks H1 #716292 (2019, Cremit reference)** — single leaked JumpCloud API key in public GitHub repo, classified CWE-798, **CVSS 9.7 critical**, paid bounty — proof that bug bounty programs CAN treat credential exposure as paying class. Reference: https://www.cremit.io/blog/out-of-scope-loophole-credential-exposure (Apr 2026 Cremit research on this exact pattern).

Hunt with **TruffleHog** (https://github.com/trufflesecurity/trufflehog, 26K+ stars, 800+ detectors, **active credential verification** against provider APIs to confirm still-live keys). Same pattern via **GitGuardian**, **Snyk Code SAST**, **GitHub Secret Scanning**. Caveat from Cremit Apr 2026 analysis: **most bug bounty programs still classify credential exposure as out-of-scope** — check program scope before reporting; many specifically EXCLUDE this finding class.

**4. Cloud-bucket misconfig (S3, GCS, Azure Blob — mid four-figure when PII counted; informational P5 when generic).** **ESHYFT March 2025** — 108GB / 86,341 healthcare-worker records (medical IDs, drivers licenses, SSNs, prescription records, disability claims) in unsecured S3 bucket; researcher Jeremiah Fowler discovered, took >1 month for org to close. Reference: https://www.theregister.com/2025/03/11/uber_for_nurses_exposes_86k/. **NASA Bugcrowd disclosure Feb 2025** — PDF in public S3 bucket (P5 informational only — NASA's VDP triages cloud-bucket findings strictly). Reference: https://bugcrowd.com/disclosures/aa45924f-8b67-4f22-8dff-dac7dc9d60e2/exposure-of-pdf-file-in-a-public-amazon-s3-bucket-associated-with-nasa.

Hunt with **`s3scanner`**, **`AWSBucketDump`**, subdomain enumeration → `<name>.s3.amazonaws.com`, `<name>.blob.core.windows.net`, `<name>.storage.googleapis.com`. The Wiz Cloud Security Index annual report tracks the most-common misconfig classes.

**5. Debug endpoint family (Spring actuator, Go pprof, Glances, FUXA, NetBird, Harbor — low four-figure to low five-figure depending on what's exposed).** All NVD-verified or GHSA-verified, all 2025-2026:
- **Dgraph `/debug/pprof/cmdline` (GHSA-95mq-xwj4-r47p)** — unauthenticated debug endpoint exposes full process command line including admin token from `--security "token=..."`. Critical.
- **Glances `/api/4/serverslist` (GHSA-r297-p3v4-wp8m, GHSA-gfc2-9qmw-w7vh)** — Central Browser mode REST API returns raw server objects with reusable downstream credentials; permissive CORS allows any origin.
- **FUXA plaintext DB credentials (GHSA-c5gq-4h56-4mmx)** — unauthenticated remote attacker retrieves administrative database credentials.
- **Harbor default password (GHSA-hj7x-hmf2-hc2p)** — GoHarbor v2.15.0 and below allows default password for web UI login.
- **NetBird VPN (GHSA-g3j4-58mp-3x25)** — installation script fails to remove ZITADEL-created default admin password.
- **MinIO LDAP brute-force (GHSA-jv87-32hw-hh99)** — `AssumeRoleWithLDAPIdentity` STS endpoint vulnerable to brute-forcing due to user enumeration + missing rate limit.
- **PraisonAI WebSocket Gateway (GHSA-cfh6-vr3j-qc3g)** — `/ws` and `/info` endpoints serve agent topology with no auth; any network client can connect, enumerate registered agents.
- **Gradio ACL bypass (GHSA-j2jg-fq62-7c3h)** — file path ACL bypassed via case alteration; lack of case normalization.
- **Rancher cluster template credentials in answers** — credentials not properly sanitized.
- **ArgoCD Redis cache risky/missing crypto** — credentials cached without proper encryption.
- **`/server-status`** Apache mod_status endpoint — H1 report 2473173 (2026 High) — exposed at `https://203.137.128.240/server-status` leaks request URLs, IPs, vhosts.

**6. PII exposure via API misconfiguration (CVSS-Confidentiality-only, low four-figure to low five-figure depending on record count).** Recent disclosed examples:
- **ASBS soldiers PII** (H1 2026 critical) — viewing other soldiers' Personnel Information / Board / Board Voters via the Army Body Score System.
- **`/talos/api/v1/files/upload` Critical Information Disclosure** (H1 report 3228011, 2025 critical).
- **IBM Aspera HTTP Gateway** (H1 report 3340797, 2026 high) — sensitive information stored in clear text in easily obtainable files.
- **ORDER_ERROR_LOG PII Data Exposure** (H1 2026 high) — error-log endpoint returns PII for any order ID.
- **Session Cookie Leakage via Static Header Field in WebViewerFragment** (H1 2026 high) — mobile WebView leaks session via static header.

Hunt: every API endpoint that returns user data, every error/log endpoint, every export feature, every `/me` and `/users/{id}` route.

**7. WordPress wp-config.php exposure (mid three-figure to mid four-figure; reliable VDP filler).** Multiple H1 reports 2026: report 3328408 (National Guard website), report 3252302. Pattern: `.bak`, `.old`, `~`, `.swp` backup files served alongside `wp-config.php` containing DB credentials. Hunt with: `curl https://target/wp-config.php{,.bak,.old,~,.swp}`. The BackupFinder ffuf wordlist covers the common variants.

**Memory disclosure (Heartbleed-class, when found pays mid five-figure).** **Dgraph `/debug/pprof/cmdline`** above. **`.NET Framework ObjRefs Disclosure (CVE-2024-29059)`** — H1 2026 high — .NET Remoting ObjRefs reveal internal endpoints. Modern memory-disclosure findings rare but high-impact when chained.

**Mobile / game / IoT disclosure** — **ASLR leak in Mario Kart World through LAN mode** (H1 2026 high). Mobile-app reverse-engineering finds API keys, internal URLs in compiled binaries. Game-engine memory disclosure via LAN/multiplayer protocols.

**What pays the most:** Spring actuator heapdump → cloud takeover (mid five-figure when chained — Volkswagen-class). Source-code repo with active AWS keys (low five-figure on programs that accept the class — Starbucks H1 #716292 precedent). Mass-PII via API misconfig (mid four-figure × record-count multiplier on healthcare/financial). `.env` exposure with active credentials (low four-figure direct + chain to AWS infrastructure for upgrade). `.git/` exposure (low four-figure direct + recover full repo + extract secrets). Generic stack trace, version banners, README leak — N/A on most programs.

## Attack Surface Signals

Greppable signals that this surface might exist:

```bash
# Spring Boot Actuator endpoints (CVE-2025-41243 family + Volkswagen pattern)
rg -n -e 'management\.endpoints\.web\.exposure\.include' \
   -e 'spring-boot-starter-actuator' \
   -g 'application*.{yml,yaml,properties}' -g '*.gradle' -g 'pom.xml'

# Spring actuator endpoints exposed in code
rg -n -e '@Endpoint' -e '@WebEndpoint' -e '@ReadOperation' --type java

# Hardcoded secrets / credentials in source
rg -n -e 'AKIA[0-9A-Z]{16}' \
   -e 'AIza[0-9A-Za-z_-]{35}' \
   -e 'sk_live_[0-9a-zA-Z]{24,}' \
   -e 'ghp_[0-9a-zA-Z]{36}' \
   -e 'github_pat_[0-9a-zA-Z]{82}' \
   -e 'xox[bpoa]-[0-9]{12}-[0-9]{12}-[0-9a-zA-Z]{24}' \
   -e 'eyJhbGciO[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+' \
   -g '!*.lock' -g '!node_modules' -g '!vendor'

# Database connection strings
rg -n -i -e 'jdbc:(?:mysql|postgresql|mariadb|oracle|mssql)://[^"\s]*:[^@\s]+@' \
   -e 'mongodb(\+srv)?://[^"\s]*:[^@\s]+@' \
   -e 'redis://[^"\s]*:[^@\s]+@' \
   -e 'amqp://[^"\s]*:[^@\s]+@'

# .env file references in code (suggest .env exists in deploy)
rg -n -e 'process\.env\.\w+' -e 'os\.environ\[' -e 'env\(\'?\w+\'?\)' \
   --type js --type ts --type py --type php

# Debug / pprof endpoints in Go code
rg -n -e 'net/http/pprof' -e '"/debug/pprof"' -e '_pprof' --type go

# Verbose error / debug mode in framework configs
rg -n -i -e 'debug\s*=\s*true' -e 'app_debug=true' \
   -e 'DEBUG\s*:\s*True' -e 'DJANGO_DEBUG=True' \
   -g '*.{yml,yaml,toml,ini,env,properties}'

# Stack-trace exposure in error handlers (Express/Flask)
rg -n -e 'app\.use\(function\(err' -e 'errorhandler\(' \
   -e '@app\.errorhandler' -e 'send.*error\.stack' \
   --type js --type ts --type py
```

HTTP-level signals on a live target:

- `Server: Tomcat`, `X-Application-Context:`, `Server: Jetty` + `/actuator/health` returns 200 → **Spring Boot Actuator surface** (probe `/actuator/heapdump`, `/actuator/env`, `/actuator/beans`)
- `Server: Apache` + Apache `mod_status` enabled → **`/server-status` info disclosure** (H1 report 2473173, 2026 High)
- `X-Powered-By: PHP/X.Y.Z` + `phpinfo()` reachable at common paths → **CVE-class info disclosure**
- `X-Powered-By: ASP.NET`, `.NET Remoting` enabled → **CVE-2024-29059 .NET Framework ObjRefs disclosure** family
- Subdomain returns Laravel default page → probe `https://target/.env` directly
- Any 200 OK on `https://target/.git/HEAD` or `https://target/.git/config` → **`.git/` exposure** (Sysdig EmeraldWhale class)
- `Vary: Accept-Encoding` + `text/html` response on `https://target/wp-config.php{,.bak,.old,~}` → **WordPress wp-config exposure** (H1 reports 3328408, 3252302, 2026 High)
- Open `<bucket>.s3.amazonaws.com`, `<bucket>.blob.core.windows.net`, `<storage>.googleapis.com` returns XML directory listing → **S3 / Azure / GCS bucket misconfig** (ESHYFT-class)
- `/api/v1/info`, `/api/4/serverslist`, `/info`, `/version`, `/build-info` returning verbose data → **Glances-class** (GHSA-r297-p3v4-wp8m)
- `/debug/pprof/cmdline` returns process command line including secrets → **Dgraph-class** (GHSA-95mq-xwj4-r47p)
- Login page returning different responses for valid vs invalid usernames → **user enumeration** surface (MinIO GHSA-jv87-32hw-hh99 LDAP variant)
- `/swagger.json`, `/api-docs`, `/openapi.json`, `/.well-known/openapi` exposed in production → **API documentation exposure**
- 500 error response includes full stack trace, file paths, framework version → **stack-trace info disclosure**
- `WWW-Authenticate: Basic realm="..."` exposing internal app names → **internal hostname disclosure**
- `Set-Cookie: <SESSION>=<value>; Domain=.target.com; Path=/` (no `Secure`, no `HttpOnly`) → **session-cookie leakage surface**

## Insertion Point Taxonomy

Every place sensitive content leaks for info-disclosure:

- **Source-code repos** (.git/.svn/.hg directories served by web server, public GitHub repos with embedded secrets, leaked private repo via dependency) — Sysdig EmeraldWhale 2024
- **Configuration files** (.env, wp-config.php, web.config, application.properties, settings.py, config.json) — Unit42 110K-domain scan 2024
- **Backup files** (.bak, .old, ~, .swp, .swo, .DS_Store, _bak, .backup, .copy) — manual fuzzing target
- **Debug / health / actuator endpoints** (/actuator/*, /debug/pprof/*, /api/health, /api/info, /api/build-info) — Wiz Threat Research 2024
- **Error pages / stack traces** (any 500 / 404 / unhandled exception) — framework-version + path disclosure
- **Server-status / management consoles** (/server-status, /server-info, /jenkins/script, /actuator/*) — multiple H1 2026 reports
- **Cloud storage buckets** (S3, GCS, Azure Blob, OSS, R2 — by name guessing or subdomain enum) — ESHYFT-class
- **API responses with verbose fields** (`/users/{id}` returning password_hash, internal_notes, role, tokens) — over-fetching pattern
- **Background-job logs** (Sidekiq, Celery, BullMQ admin UI exposing job arguments) — credential leakage in job params
- **Browser-side: JS bundles, Service Workers, source maps** (`.map` files, `__webpack_require__` exposing module names) — secrets in client-side
- **Mobile app binaries** (APK reverse-engineering, IPA Mach-O strings, embedded URLs and API keys)
- **Memory dumps** (heapdump, core dumps, profiler output, browser tab snapshots) — Volkswagen Spring actuator pattern
- **Process command lines** (/proc/PID/cmdline, /debug/pprof/cmdline, ps output) — Dgraph GHSA-95mq-xwj4-r47p
- **DNS records** (TXT records exposing internal service names, SPF records exposing service vendors, CNAME chains)
- **Headers** (Server, X-Powered-By, X-AspNet-Version, X-Framework, custom internal-name headers)
- **Email headers and bounces** (Received chains exposing internal mail server names, NDR responses leaking user existence)
- **Webhooks / callback URLs** (test webhook endpoints exposing internal service URLs in retry logs)
- **CORS preflight responses** (Access-Control-Allow-Origin reflecting attacker origin reveals trusted-origin allowlist)
- **Sitemap, robots.txt** (admin paths, dev environment URLs, staging hostnames)
- **Public-by-design APIs accidentally returning private fields** — over-fetching, GraphQL field-level (cross-references hunt-idor and hunt-xss)
- **Enumeration endpoints** — different responses for existing vs non-existing user/email/UUID enable username harvesting (MinIO LDAP GHSA-jv87-32hw-hh99 pattern)
- **Cache headers leaking auth state** (`X-Cache: HIT` / `Vary` mismatches reveal authenticated content cached publicly)

For each surface, send: `curl -sI https://target/<path>` (header inspection), `curl -s https://target/<path> | head -100` (body inspection), `curl -s -X POST -H "Content-Type: application/json" -d '{}' https://target/<endpoint>` (error elicitation), and run `nuclei -t exposures/` against the host for the canonical exposure templates.

## Step-by-Step Hunting Methodology

1. **Subdomain enumeration first.** `subfinder + amass + chaos + crt.name` → every subdomain. Each is a candidate for `.git/`, `.env`, `/actuator/`, `/server-status`. The exposed-config attack surface scales linearly with subdomain count; legacy / dev / staging subdomains are the highest-yield targets.

2. **Run nuclei `exposures/` template set on every subdomain.** `nuclei -t http/exposures/` covers `.git/HEAD`, `.env`, `/server-status`, `/actuator/*`, `/swagger.json`, `/.well-known/`, `/phpinfo`, hundreds more. Single command, ~30 seconds per host. The H1 2026 wp-config disclosures (reports 3328408, 3252302) and `/server-status` disclosure (2473173) are all canonical nuclei-template hits.

3. **Spring Boot Actuator deep-probe.** If host responds with `Server:` containing Tomcat/Jetty/Undertow OR `X-Application-Context:` header OR `/actuator/health` returns 200 — probe the full endpoint set with priority ordering: `/actuator/heapdump` (CRITICAL — full memory dump), `/actuator/env` (HIGH — env vars), `/actuator/configprops`, `/actuator/beans`, `/actuator/threaddump`, `/actuator/mappings`, `/actuator/loggers`, `/actuator/gateway/routes` (CVE-2025-41243). For `/actuator/heapdump`, download the binary, analyze with Eclipse Memory Analyzer (MAT) or `strings | grep -E 'AKIA|AIza|sk_live|ghp_'` for secret patterns.

4. **`.env` and `.git/` direct probe on every subdomain.** `curl -s https://target/.env` and `curl -s https://target/.git/HEAD`. If 200 OK with content, run `git-dumper https://target/.git/ /tmp/dumped` to recover the entire repo. Then `trufflehog filesystem /tmp/dumped --only-verified` to extract live credentials.

5. **`wp-config.php` and backup-file fuzzing.** Use `ffuf -w /usr/share/wordlists/seclists/Discovery/Web-Content/raft-medium-files.txt -u https://target/FUZZ -mc 200`. Append common backup extensions: `wp-config.php{,.bak,.old,~,.swp,.swo,.copy,.backup}`. The H1 2026 wp-config reports came from this exact technique against forgotten subdomains.

6. **Source-code repo secret scanning.** For OSS targets and any program with GitHub presence in scope: `trufflehog github --org=<org> --only-verified` (uses 800+ detectors with active credential validation). Cross-check with `gitleaks` on cloned repos. Reference TruffleHog: https://github.com/trufflesecurity/trufflehog. Caveat: most bug bounty programs classify credential exposure as out-of-scope; check program policy first (see Cremit Apr 2026 analysis at https://www.cremit.io/blog/out-of-scope-loophole-credential-exposure).

7. **Cloud bucket discovery.** From subdomain list, derive bucket-name candidates: `<subdomain>`, `<subdomain>-prod`, `<subdomain>-staging`, `<subdomain>-backup`, `<subdomain>-logs`, `<subdomain>-data`. Test each against `https://<name>.s3.amazonaws.com`, `https://<name>.blob.core.windows.net`, `https://storage.googleapis.com/<name>`. Use `s3scanner` or `AWSBucketDump` for automation. ESHYFT-class findings come from this exact enumeration on healthcare/financial targets.

8. **Debug endpoint family.** For Go services: `/debug/pprof/cmdline` (Dgraph GHSA-95mq-xwj4-r47p — exposes admin token in process args). For Spring: actuator family above. For Glances/Prometheus/Grafana monitoring: `/api/4/serverslist` (GHSA-r297-p3v4-wp8m), `/metrics`, `/prometheus`, `/dashboards/api/`. For Kubernetes: `/healthz/secret`, `/api/v1/secrets` against kubelet anonymous-auth misconfigs.

9. **API over-fetching audit.** For every authenticated API endpoint, request the data and inspect response. Look for fields that shouldn't be there: `password_hash`, `password_salt`, `tokens[]`, `api_keys[]`, `internal_notes`, `admin_comments`, `kyc_documents`, `mfa_secrets`, `recovery_codes`. The over-fetch pattern is mid four-figure on most programs.

10. **User / email enumeration.** Compare login response (timing, body, status) for known-existing vs non-existing username. Same for password reset, registration ("email already in use" vs "verification sent"). MinIO GHSA-jv87-32hw-hh99 (LDAP brute-force via enumeration + missing rate limit) is the canonical 2026 case. Document the timing delta or response delta.

11. **Stack trace / debug page elicitation.** Send malformed requests to every endpoint: `'`, `null`, `[]`, `{"a":}`, large bodies, missing required fields, content-type mismatches. Watch for 500 responses with full stack traces, file paths, framework versions, DB type. These are mid three-figure direct unless they reveal something materially sensitive (DB connection string, internal IP, framework + version → CVE replay candidate).

12. **Mobile app reverse engineering.** Pull APK with `apktool d <apk>` or IPA with `ipsw`. Run `strings <binary> | grep -iE '(api[_-]?key|secret|token|password|bearer|aws_access_key)'`. Inspect `assets/`, `res/raw/`, `META-INF/` for plaintext config. Mobile-app secret leaks are mid four-figure on iOS/Android program scope.

13. **Validate before reporting.** Demonstrable impact: count the records (PII), validate the credential is live (`aws sts get-caller-identity` for AWS keys; `curl https://api.github.com/user -H "Authorization: token <ghp>"` for GitHub tokens), show the exfil path. Don't dump the whole bucket — three records is enough proof. See Gate 0.

## Payload & Detection Patterns

### Sub-technique A — `.env` and config-file direct probe

```bash
# Direct .env probe
curl -s -o /dev/null -w "%{http_code}\n" https://target/.env
curl -s https://target/.env | head -50

# Common variants — try each
for path in .env .env.local .env.production .env.development \
           .env.bak .env.old .env~ .env.example .env.sample \
           env .environment app.env config.env; do
  curl -s -o /dev/null -w "%{http_code} %s\n" $path https://target/$path
done

# Laravel-specific (Unit42 Aug 2024 attack pattern)
curl -s https://target/.env | grep -E '^(APP_KEY|DB_PASSWORD|MAIL_PASSWORD|AWS_ACCESS_KEY|AWS_SECRET|MAILGUN)'

# Symfony / NextJS variants
curl -s https://target/.env.local
curl -s https://target/.next/server/.env

# WordPress wp-config (H1 2026 reports 3328408, 3252302)
for ext in '' .bak .old '~' .swp .swo .copy .backup; do
  curl -s -o /dev/null -w "%{http_code} %s\n" "wp-config.php$ext" "https://target/wp-config.php$ext"
done
```

### Sub-technique B — `.git/` directory exposure (Sysdig EmeraldWhale 2024 pattern)

```bash
# Detect exposed .git
curl -s https://target/.git/HEAD
curl -s https://target/.git/config
curl -s https://target/.git/index | head -c 100  # binary index

# Recover full repo using git-dumper
pip install git-dumper
git-dumper https://target/.git/ /tmp/dumped-target

# Inspect for secrets in commit history
cd /tmp/dumped-target
git log --all --oneline
git log --all -p -S 'AKIA' -- '*'        # find AWS keys ever committed
git log --all -p -S 'password' -- '*'    # find password mentions
git log --all -p -S 'sk_live' -- '*'     # find Stripe live keys
git log --all -p -S 'ghp_' -- '*'        # find GitHub PATs

# TruffleHog pass for systematic secret extraction (live verification)
trufflehog filesystem /tmp/dumped-target --only-verified --json > /tmp/secrets.json

# Reference: https://sysdig.com/blog/emeraldwhale (Sysdig Threat Research Oct 2024)
# 15K cloud creds stolen from 67K URLs via this exact pattern
```

### Sub-technique D — Cloud bucket misconfig (S3/GCS/Azure)

```bash
# AWS S3 — check public listing
aws s3 ls s3://<target-bucket-name> --no-sign-request
curl -s https://<bucket-name>.s3.amazonaws.com/
# XML response with <ListBucketResult> → public listing enabled

# Get specific file (when listing disabled but read enabled)
aws s3 cp s3://<bucket>/secret.txt /dev/stdout --no-sign-request

# Discovery via subdomain enumeration (ESHYFT-class, Mar 2025)
# For each subdomain, try as bucket name
for sub in $(cat subdomains.txt); do
  for suffix in '' '-prod' '-staging' '-backup' '-logs' '-data' '-uploads'; do
    curl -s -o /dev/null -w "%{http_code} %s\n" "$sub$suffix" \
      "https://$sub$suffix.s3.amazonaws.com/"
  done
done

# Google Cloud Storage
curl -s https://storage.googleapis.com/<bucket-name>/
gsutil ls gs://<bucket-name>

# Azure Blob
curl -s "https://<account>.blob.core.windows.net/<container>?restype=container&comp=list"

# AWSBucketDump for systematic enum
git clone https://github.com/jordanpotti/AWSBucketDump
python AWSBucketDump.py -l buckets.txt -g grep_patterns.txt -D
```

### Sub-technique E — Source-code secret scanning (TruffleHog pattern)

```bash
# Scan a GitHub org for verified secrets
trufflehog github --org=target-org --only-verified --json > secrets.json

# Scan single repo with full git history
trufflehog github --repo=https://github.com/target/repo --only-verified --json

# Scan a local clone (faster)
git clone --mirror https://github.com/target/repo /tmp/repo.git
trufflehog filesystem /tmp/repo.git --only-verified

# Scan S3 bucket contents
trufflehog s3 --bucket=<bucket-name>

# Scan Docker image
trufflehog docker --image=<image-name>

# Common manual regex patterns (what TruffleHog detects with API verification)
# AWS access key
grep -rE 'AKIA[0-9A-Z]{16}' .
# AWS secret (40 chars b64-ish)
grep -rE '[A-Za-z0-9/+=]{40}' . | grep -i -E 'aws_secret|secret_key|secretAccessKey'
# Google API key
grep -rE 'AIza[0-9A-Za-z_-]{35}' .
# Slack token
grep -rE 'xox[bpoa]-[0-9]{12}-[0-9]{12}-[0-9a-zA-Z]{24}' .
# Stripe live key
grep -rE 'sk_live_[0-9a-zA-Z]{24,}' .
# GitHub Personal Access Token
grep -rE 'ghp_[0-9a-zA-Z]{36}' .
grep -rE 'github_pat_[0-9a-zA-Z]{82}' .
# JWT token
grep -rE 'eyJhbGciO[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+' .
# Private key
grep -rE '-----BEGIN (RSA|EC|OPENSSH|PRIVATE) (PRIVATE )?KEY-----' .

# Validate AWS key is live (don't pivot — just confirm)
AWS_ACCESS_KEY_ID=AKIA... AWS_SECRET_ACCESS_KEY=... \
  aws sts get-caller-identity

# Validate GitHub PAT is live
curl -s https://api.github.com/user -H "Authorization: token ghp_..."

# Reference: https://github.com/trufflesecurity/trufflehog
# Reference: https://snyk.io/articles/state-of-secrets/ (28.65M secrets in 2025 per GitGuardian)
```

### Sub-technique F — Debug / pprof / management endpoint family

```bash
# Go pprof — Dgraph pattern (GHSA-95mq-xwj4-r47p, 2026 critical)
curl -s https://target/debug/pprof/
curl -s https://target/debug/pprof/cmdline   # process command line — leaks --security="token=..."
curl -s https://target/debug/pprof/goroutine?debug=2  # goroutine stacks
curl -s https://target/debug/pprof/heap > heap.bin    # heap dump
go tool pprof -text heap.bin                          # analyze

# Glances /api/4/* family (GHSA-r297-p3v4-wp8m, GHSA-gfc2-9qmw-w7vh, GHSA-7p93-6934-f4q7, 2026 critical/high)
curl -s https://target:61208/api/4/serverslist     # leaks downstream credentials
curl -s https://target:61208/api/4/all             # full system info
curl -s https://target:61208/api/4/processlist     # running processes
# CORS bypass: even from attacker origin, Access-Control-Allow-Origin: * lets attacker exfil

# FUXA plaintext DB credentials (GHSA-c5gq-4h56-4mmx, 2026 critical)
curl -s https://target/api/settings  # vulnerable endpoint returns DB creds

# MinIO LDAP brute-force via enumeration (GHSA-jv87-32hw-hh99, 2026 critical)
# AssumeRoleWithLDAPIdentity returns different errors for valid vs invalid LDAP user
curl -X POST https://minio.target/?Action=AssumeRoleWithLDAPIdentity \
  -d "LDAPUsername=alice&LDAPPassword=test&Version=2011-06-15"
# Different response code/message for "user exists, wrong password" vs "user doesn't exist"

# Apache mod_status / server-info (H1 2026 report 2473173)
curl -s https://target/server-status
curl -s https://target/server-info

# phpinfo
for path in phpinfo.php info.php php-info.php test.php phpinfo.html; do
  curl -s -o /dev/null -w "%{http_code} %s\n" $path https://target/$path
done

# Spring Boot Admin
curl -s https://target/admin/login
curl -s https://target/wallboard

# /jenkins/script — Jenkins Groovy console
curl -s https://target/jenkins/script
```

### Sub-technique G — User / email enumeration via response delta

```bash
# Login response delta (existing user vs non-existing)
curl -s -X POST https://target/api/login -d '{"email":"existing@target.com","password":"wrong"}'
# Response: "Invalid password" (HTTP 401)
curl -s -X POST https://target/api/login -d '{"email":"nonexistent@target.com","password":"wrong"}'
# Response: "Invalid email or password" (HTTP 401)
# Different messages → enumeration

# Password reset response delta
curl -s -X POST https://target/api/password-reset -d '{"email":"existing@target.com"}'
# "Verification email sent"
curl -s -X POST https://target/api/password-reset -d '{"email":"nonexistent@target.com"}'
# "User not found"
# Or both return same message — but timing differs (DB lookup vs not)

# Registration enumeration
curl -s -X POST https://target/api/signup -d '{"email":"existing@target.com","password":"x"}'
# "Email already in use"

# MinIO LDAP variant (GHSA-jv87-32hw-hh99)
# Different error code for "user not in LDAP" vs "user exists, wrong password"
# Then brute-force the existing user's password without rate limiting

# SAML response: different error for "no user found" vs "wrong assertion"
curl -X POST https://target/saml/acs -d 'SAMLResponse=<base64>'

# OAuth: client_id enumeration via authorize endpoint
curl -s "https://target/oauth/authorize?client_id=valid&redirect_uri=..."
curl -s "https://target/oauth/authorize?client_id=invalid&redirect_uri=..."

# Time-based enumeration (when responses are identical)
for email in "alice@target.com" "nonexistent@target.com"; do
  time curl -s -o /dev/null -X POST https://target/api/login \
    -d "{\"email\":\"$email\",\"password\":\"x\"}"
done
# Existing user takes longer (bcrypt hash comparison vs immediate fail)
```

### Sub-technique H — Stack trace / verbose error elicitation

```bash
# Send malformed JSON
curl -s -X POST https://target/api/users \
  -H 'Content-Type: application/json' \
  -d 'not json'

# Send wrong content-type
curl -s -X POST https://target/api/users \
  -H 'Content-Type: application/xml' \
  -d '<x/>'

# Send oversized body
python3 -c 'print("a"*10**6)' | curl -s -X POST https://target/api/users \
  -H 'Content-Type: application/json' --data-binary @-

# Send invalid characters in URL params
curl -s "https://target/api/items?id=%00"
curl -s "https://target/api/items?id='"
curl -s "https://target/api/items?id=[]"

# Send invalid IDs — type confusion
curl -s "https://target/api/users/null"
curl -s "https://target/api/users/undefined"
curl -s "https://target/api/users/{}"

# Trigger internal-error path — query for known-broken state
curl -s "https://target/api/orders?status=FOOBAR_INVALID_ENUM"

# Look for: file paths (/var/www/, C:\inetpub\), framework versions (Spring/Django/Rails),
# DB hostnames (db.internal.target.com), DB type (PostgreSQL 14.x), full stack traces.
```

### Sub-technique I — Mobile app reverse engineering for embedded secrets

```bash
# Android APK
apktool d target.apk -o /tmp/apk-decoded
strings /tmp/apk-decoded/classes.dex | grep -iE '(api[_-]?key|secret|token|password|bearer)'
grep -rE 'AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{35}' /tmp/apk-decoded/

# JADX for decompilation (better strings extraction)
jadx -d /tmp/apk-jadx target.apk
grep -rE '(api[_-]?key|secret|token)' /tmp/apk-jadx/sources/

# Inspect res/raw/, assets/, META-INF/ for plaintext config
ls /tmp/apk-decoded/res/raw/
ls /tmp/apk-decoded/assets/

# iOS IPA
ipsw extract --ipa target.ipa
strings /path/to/binary | grep -iE '(api[_-]?key|secret|token|password)'

# MobSF for automated mobile app analysis (https://github.com/MobSF/Mobile-Security-Framework-MobSF)
docker run -it --rm -p 8000:8000 opensecurity/mobile-security-framework-mobsf

# Reference: H1 2026 Session Cookie Leakage in WebViewerFragment (mobile WebView)
```

## Modern Meta — Cloud-Native, CI/CD, OSS Pipeline

This is where 2024-2026 info-disclosure meta lives. Coverage required: GitHub Actions, GitLab CI, Jenkins, ArgoCD/Flux, Kubernetes, IAM/IMDS, supply chain.

**GitHub Actions info-disclosure** — workflow logs leaking secrets when echo statements include `${{ secrets.X }}` (GitHub's masking only catches exact string match; `echo $TOKEN | base64` exposes encoded form unmasked). Artifact upload exposing `.env` files. Public artifact downloads from previous workflow runs. `workflow_dispatch` inputs reflected in run history. Reference: GitHub Actions Hardening guide.

**GitLab CI info-disclosure** — `CI_JOB_TOKEN` echoed in logs, `.gitlab-ci.yml` referencing artifacts that contain secrets, GitLab Pages deploying secret-containing files, shared-runner exposing job environment to other tenants.

**Jenkins info-disclosure** — `/script` endpoint exposed (Groovy console with full system access), agent JNLP secret leak via `/computer/<agent>/slave-agent.jnlp`, build-parameter values reflected in console output, `manage/configure` showing credentials in plaintext.

**ArgoCD / Flux / Tekton (GitOps controllers) info-disclosure**:
- **ArgoCD Risky/Missing Cryptographic Algorithms in Redis Cache** (2024 critical, GHSA-cited in corpus) — credentials cached without proper encryption.
- **Rancher cluster template answers** (2026 critical, GHSA-cited) — credentials not properly sanitized.
- **PraisonAI WebSocket Gateway** (GHSA-cfh6-vr3j-qc3g, 2026 critical) — `/ws` and `/info` endpoints expose agent topology with no auth.
- Kubernetes Secret access via misconfigured RoleBinding scope (cross-namespace read).
- ConfigMap with embedded credentials (anti-pattern but common).

**Kubernetes info-disclosure** — kubelet anonymous auth (`--anonymous-auth=true`) exposes `/pods`, `/runningpods`, `/exec` endpoints. etcd direct access bypasses RBAC. ConfigMap / Secret read across namespaces via misconfigured RoleBinding scope. Public-by-default `kubectl get` for low-privilege ServiceAccounts in shared clusters.

**Cloud IAM / IMDS** — IMDSv1 reachable from Lambda or container with SSRF chain (`curl http://169.254.169.254/latest/meta-data/iam/security-credentials/<role>`) → AWS keys. CSP / network policy usually blocks; when it doesn't, becomes RCE-class.

**Supply chain (npm/pip/RubyGems)** — secrets leaked in package metadata (`npm pack` includes files outside `files` allowlist), test fixtures with real credentials, `.npmrc`/`.pypirc` with publish tokens published in package, postinstall scripts logging env vars. **Telnyx PyPI typosquat 2026 critical** (GHSA cited in corpus) — malicious code in versions 4.87.1 and 4.87.2.

**OSS hunting workflow:** `socket dev <package>` → audit recent versions for new file additions → diff with previous → if `.env` or test-fixtures with credentials added, file as info-disclosure / supply-chain incident.

## Common Root Causes

Why developers introduce info-disclosure — patterns visible across the corpus and 2024-2026 meta:

1. **`management.endpoints.web.exposure.include=*`** — Spring Boot Actuator default exposure for "internal" environments leaked to production. Restrict to `health,info` only; secure others.
2. **`.env` served by web server** — application root contains `.env`; web server (Nginx, Apache) serves it because no explicit deny rule. Add `location ~ /\.env { deny all; }` or equivalent.
3. **`.git/` in webroot** — deployment via `git pull` on production server leaves `.git/` in webroot. Use `rsync --exclude='.git'` or proper CI/CD deploy.
4. **Hardcoded credentials in source** — CWE-798. `password = "actual_password"` committed to repo. Use environment variables, secret managers. Reference: GitGuardian 2026 28.65M secrets in 2025.
5. **Verbose error handling in production** — `app.use(function(err, req, res, next) { res.send(err.stack); })` left from development. Toggle via NODE_ENV / Django DEBUG / Rails RAILS_ENV.
6. **Public S3 buckets via misconfig** — bucket policy allows `Principal: *` for `s3:GetObject` or `s3:ListBucket`. Use AWS Block Public Access account-wide setting.
7. **`/debug/pprof/cmdline` exposed in Go services** — `import _ "net/http/pprof"` adds default mux handler at `/debug/pprof/`. Remove import in production builds OR use separate internal-only mux.
8. **Over-fetching API responses** — model serializer returns all fields by default. Use explicit field allowlist (`fields = ['id', 'name']` in Django REST Framework).
9. **Different error messages for valid vs invalid usernames** — login flow does `if not user: return 401` then `if not check_password(): return 401` with different bodies. Use single generic message.
10. **Missing rate limit on enumeration-friendly endpoints** — login, password reset, registration. MinIO LDAP brute-force GHSA-jv87-32hw-hh99 is the canonical example.
11. **Default credentials not removed during deployment** — installation creates `admin/admin` or `admin/changeme` and never forces change. Harbor GHSA-hj7x-hmf2-hc2p, NetBird GHSA-g3j4-58mp-3x25.
12. **CORS `Access-Control-Allow-Origin: *` on credentialed APIs** — Glances GHSA-r297-p3v4-wp8m and GHSA-7p93-6934-f4q7 patterns. Allows any origin to read credentialed responses cross-origin.
13. **Backup files not gitignored / not deny'd by web server** — `.bak`, `.old`, `~`, `.swp` extensions served alongside originals (wp-config.php.bak pattern).
14. **Source maps shipped to production** (`/_next/static/.../*.map`) — exposes full source code, internal variable names. Disable source map upload in production CI/CD.
15. **Cache-control headers leaking auth state** — `X-Cache: HIT` on authenticated content reveals it was cached publicly; `Vary` header mismatches enable cache poisoning that exposes other users' content.

## Bypass Techniques

Defense bypasses observed in disclosed reports.

- **Path normalization for ACL bypass** — Gradio GHSA-j2jg-fq62-7c3h pattern: file path ACL checks lowercase but filesystem is case-insensitive (Windows, macOS HFS+); upload `Config.YML` to bypass blocklist of `config.yml`.
- **Backup-file fuzzing past the canonical filename** — when `wp-config.php` is denied, try `wp-config.php.bak`, `wp-config.php.old`, `wp-config.php~`, `wp-config.php.swp`. Server's deny rule covers exact filename only. Reference: H1 reports at https://hackerone.com/reports/3328408 and https://hackerone.com/reports/3252302 (2026 High disclosed).
- **Trailing-slash bypass on protected paths** — `/admin` blocked but `/admin/` reachable (or vice versa). Common on misconfigured nginx `location` blocks; documented across PortSwigger Web Security Academy labs.
- **Case mutation on path-based ACL** — `/Admin/`, `/ADMIN/`, `/aDmIn/` may bypass Apache/nginx case-sensitive deny rules when application is case-insensitive. Documented in PortSwigger access-control labs.
- **URL-encoding bypass** — `/.env` blocked but `/%2eenv` or `/.%65nv` reachable. Double-encoding bypasses some normalizers; same class as Apache CVE-2024-38472 path-confusion (Orange Tsai BHUSA 2024).
- **Subdomain bypass for production-only deny rules** — `admin.target.com/.env` blocked but `staging.target.com/.env` allowed because deny rule only deployed to production env. Documented across HackerOne hacktivity disclosed 2024-2025 (Shopify, GitHub, GitLab subdomain reports).
- **Range header to read partial protected file** — `curl -H "Range: bytes=0-100" https://target/.env` may return 206 even when GET returns 403 due to nginx misconfig.
- **HEAD vs GET asymmetry** — HEAD request bypasses ACL that only checks GET; HEAD response includes Content-Length revealing file size. Documented across PortSwigger Web Security Academy labs and HackerOne hacktivity disclosed 2024.
- **HTTP/2 and HTTP/3 path-handling differences** — HTTP/2 may strip path prefix differently than HTTP/1.1; ACL written for one protocol bypassed by the other. James Kettle PortSwigger Research 2023-2024 documents this class.
- **Cookie tossing for cross-subdomain credential exposure** — see hunt-xss.md Chain 9; subdomain attacker can override session cookie scope. Reference: H1 reports https://hackerone.com/reports/3321406 and https://hackerone.com/reports/3423950 (2026 High disclosed).
- **CORS `null` origin bypass** — when `Access-Control-Allow-Origin: null` is set, sandboxed iframes (`<iframe sandbox>`) get null origin and are allowed. Documented at PortSwigger Web Security Academy CORS lab series.
- **TLS SNI mismatch** — Apache vhosts dispatched by Host header may serve different config when SNI doesn't match Host; secrets-restricted vhost reachable via spoofed SNI. Same class as Apache CVE-2024-38472/38476 Confusion Attacks (Orange Tsai BHUSA 2024).
- **Authentication-only-on-frontend** — frontend hides admin endpoints but API doesn't enforce; direct API call returns full data. Documented at https://owasp.org/API-Security/editions/2023/en/0xa1-broken-object-level-authorization/ and across HackerOne hacktivity disclosed 2024.
- **Time-based enumeration when responses are identical** — bcrypt hash comparison takes ~100ms; missing-user response is immediate. Time login responses to enumerate. MinIO GHSA-jv87-32hw-hh99 LDAP variant is the canonical case.
- **TruffleHog actively-validated false-negatives** — TruffleHog only validates secrets it has detectors for; custom internal API keys without detectors won't be flagged. Manually grep for organization-specific patterns. Reference: https://github.com/trufflesecurity/trufflehog supports 800+ detectors but custom org formats need manual rules.
- **Heap-dump pattern matching for non-standard secret formats** — `strings heapdump.bin | grep -E '<your-org-prefix>'` for internal token formats. Volkswagen pattern via SYSCREST analysis https://www.syscrest.com/2025/02/securing-spring-boot-actuator/.

## Gate 0 Validation

Before you write the report, prove these five things:

1. **Concrete demonstration with measurable impact.** "I see `/server-status` exposed" without showing what it leaks is informational. Show the specific data: 3 IPs from the request log, the framework version revealed, the credential extracted. For credentials: validate they're live (`aws sts get-caller-identity`, `curl https://api.github.com/user -H "Authorization: token X"`). Don't pivot — confirm + stop.

2. **Business loss mapping.** Map to one of: customer PII exposure (count records), credential theft (active key validated), source-code leak (commits + file count), financial/healthcare data exposure (regulatory implication: GDPR, HIPAA, PCI, GLBA), credential reuse risk (key has access to N additional resources). Quantify.

3. **Reproducibility in 10 minutes.** Curl one-liner for the leak. If credentials need validation, document the validation command. Triagers close anything they can't repro at lunch.

4. **Scope check.** Asset in scope (info-disclosure on out-of-scope subdomains is N/A even if real). Credentials belong to target organization (not a third-party SaaS the target uses). Re-check after 24h — orgs sometimes patch within hours of submission, so confirm the leak is still present at submit time. **Critical for credentials**: confirm the program treats credential exposure as in-scope (see Cremit Apr 2026 analysis — several programs including Atlassian, Shopify, GitLab explicitly exclude this class).

5. **PoC artifacts**: 30-60 second screen recording showing the leak fired (asciinema or mp4 — no edits, no zooms hiding the URL). Burp request/response screenshot with non-sensitive headers visible. Curl command in plain text. For credentials: redacted credential value with length confirmed (e.g., `AKIA****************` 20 chars). For PII: 3 redacted sample records. **Never** include actual sensitive data in the report — describe it but redact values.

If any of the 5 fails: **stop**. You have a finding, not a report. Common kill-list items:

- "I found .env exposed" without confirming credentials inside are live → low-tier (informational)
- "Stack trace leaks /var/www/" without revealing material — informational
- "Swagger exposed" without showing what hidden endpoints it reveals — informational
- "Username enumeration via login response" without timing data or response delta — needs proof

## Anti-Targets / What's Dead

The kill-list. Where NOT to point the cannon.

- **Generic stack trace on a 404 page** — N/A or informational on most programs. Don't submit unless the trace reveals material info (DB connection string, internal IP, framework version mapping to an unpatched CVE). Always demonstrate what's exfiltrable.
- **`X-Powered-By: PHP/7.4.x` header alone** — version disclosure is N/A on its own. Don't submit unless you can chain to a known CVE for that version (then it's a CVE-replay report, not info-disclosure).
- **`/robots.txt` referencing `/admin/`** — won't pay; admins know about robots.txt by definition. Stop hunting unless robots.txt reveals unguessable paths or dev-only URLs.
- **Email enumeration alone via "email already in use"** — N/A on most modern programs because it's inherent to the UX. Don't submit alone; chain to mass-account-extraction for impact.
- **`/.git/HEAD` exposed but `git-dumper` returns 0 files** — informational only / N/A. Many servers serve `.git/HEAD` as default-file but block subdirectory access — without recoverable content the exposure is dead.
- **TruffleHog "potential secrets" without `--only-verified`** — won't pay because false-positive rate is huge. Stop submitting unverified TruffleHog matches; always pre-validate.
- **CORS `Access-Control-Allow-Origin: *` on public-by-design API** — N/A when API has no credentials and no per-user data. Don't submit; CORS is intentional for public APIs.
- **Source maps in production** — won't pay on most programs because frontend code is client-shipped anyway. Stop submitting bare `*.map` exposure; only paying when the map reveals internal API URLs or server-side code paths.
- **Bug-bounty programs that explicitly exclude credential exposure** — out-of-scope on several major programs (Atlassian, Shopify, GitLab per public scope statements). Check scope BEFORE reporting; the Cremit Apr 2026 analysis at https://www.cremit.io/blog/out-of-scope-loophole-credential-exposure documents this pattern.
- **WordPress wp-config.php exposed but salts already rotated** — N/A without live DB credentials. Don't submit; always validate creds before claiming paying impact.
- **Public S3 bucket with public-by-design content** (marketing assets, public docs) — N/A; misconfig only matters when private data exposed. Stop reporting bare bucket-listing on public-content buckets.
- **`/debug/pprof/` accessible but no secrets in process args** — N/A. The pay value is the secret leaked, not the endpoint. Don't submit pprof exposure alone.
- **NASA / DoD / government VDPs paying $0 / kudos** — these are informational-only programs; bounties are dead. Submit for hall-of-fame credit, not money. The same finding on a commercial paid program is low to mid four-figure.
- **Spring actuator `/health` exposed** — by design, used for K8s liveness probes. Won't pay; stop reporting bare `/health`. Only `/env`, `/heapdump`, `/configprops`, `/beans`, `/threaddump`, `/mappings`, `/loggers`, `/gateway` pay.
- **HTTP TRACE method enabled (XST class)** — mostly hardened across the modern web; XST won't pay on programs running modern HTTP servers. Don't submit unless you can demonstrate end-to-end XST exploit in a realistic browser environment.
- **Verbose 500 error revealing only Apache/Nginx version** — N/A; vendor doesn't consider banner disclosure a finding. Stop reporting bare server-version banners.

## Notes for the hunter

**24-month meta call-out.** The defining 2024-2026 info-disclosure story is **systemic credential exposure** via `.git/`/`/env`/Spring Boot Actuator/heap dumps — Sysdig EmeraldWhale (15K cloud creds), Unit42 (110K domain scan, 90K env-var combos, 7K active AWS keys), Wiz Volkswagen (9TB GPS data via single `/actuator/heapdump`), GitGuardian (28.65M secrets in 2025). If you hunt one new info-disclosure surface in the next quarter, it's running `nuclei -t http/exposures/spring-boot/` against every in-scope subdomain plus `git-dumper` against every 200-OK `.git/HEAD`. The second-place meta is **debug-endpoint family** — Dgraph `/debug/pprof/cmdline`, Glances `/api/4/serverslist`, FUXA plaintext DB creds, Harbor / NetBird default passwords (all 2025-2026 critical GHSAs). The third-place meta is **PII-via-API** with bounty scaling by record count on healthcare/financial programs (ESHYFT 86K records pattern).

**OSS targets where the next 6 months of paying bugs likely are.** Spring Boot deployments not pinned to current (CVE-2025-41243 / 41253 / 22235 family). Go services using `import _ "net/http/pprof"` in production. Self-hosted monitoring stacks (Glances, Prometheus, Grafana) with permissive CORS. Self-hosted GitOps (Rancher, ArgoCD) with cluster-template credential leakage. AI / ML platforms exposing model-registry credentials in metadata. Any SaaS using Vercel / Netlify with `_next/static/.../*.map` shipped.

**Anti-patterns reminder.** See the Anti-Targets section above. Most-common kills: stack trace without material info, version-banner-only disclosure, `/.git/HEAD` without recoverable repo, TruffleHog without `--only-verified`, programs that exclude credential exposure (always check scope first).

**Ground rule for impact in 2026:** generic info-disclosure pays mid three-figure to low four-figure; chained to active credential validation pays mid four-figure; chained to cloud-infrastructure read pays low to mid five-figure; full database access via leaked credentials pays mid five-figure on programs that pay this class. Always validate credentials before reporting — `aws sts get-caller-identity`, `curl https://api.github.com/user -H "Authorization: ..."`, etc. Dead credentials reduce the report to informational.

**Currency tip:** ~12 of the verified CVEs/GHSAs cited in this skill are from 2024-2026. Re-verify with `verify_citations.py` before finalizing any report; Spring releases security patches frequently and version constraints may shift.

## Top-Tier Operating Manual

**90-minute hunt loop**
1. 0-10 min: choose asset class by payout: actuator/heapdump, `.git`, `.env`, cloud bucket, debug endpoint, source repo, mobile binary, API over-fetch, stack trace.
2. 10-25 min: fingerprint every in-scope subdomain for exposed files and management endpoints. Prioritize staging, admin, old, dev, preview, and regional hosts.
3. 25-45 min: pull only the minimum artifact needed: first 50 lines of `.env`, `.git/config`, actuator index, bucket listing headers, one redacted API object.
4. 45-65 min: validate materiality. Check whether a key is live, whether PII is private, whether source is proprietary, whether endpoint reveals credentials or tenant data.
5. 65-80 min: upgrade or kill. Upgrade through active credential, cloud role identity, record count, regulatory class, or cross-tenant impact. Kill version banners and generic paths.
6. 80-90 min: package redacted proof. Do not download bulk data. Do not pivot beyond identity validation.

**Decision tree**
- If the leak is only banner/version/path, kill unless it maps to an exploitable unpatched CVE.
- If the leak contains credentials, validate liveness safely and stop.
- If the leak contains source code, scan for secrets and proprietary logic, then stop at counts.
- If the leak contains PII, count records from metadata or listing names; sample only your own or three redacted records.
- If the leak is `.git`, recover repo only when object files are reachable; `HEAD` alone is not enough.
- If actuator exposes `/health` only, kill. If `/env`, `/heapdump`, `/configprops`, or `/gateway` is exposed, prioritize immediately.

**False-positive graveyard**
- Public S3 bucket with marketing assets: kill.
- Source map containing only client code shipped to users: kill unless it exposes server secrets or private endpoints.
- Dead credential from TruffleHog: kill or report as hygiene only if program accepts it.
- Stack trace with framework name only: kill.
- Swagger docs for public API: kill unless hidden private endpoints or auth bypass are proven.
- `/actuator/health` or `/metrics` without sensitive labels: kill.

**Program economics**
- Active cloud credentials beat almost every other info-disclosure proof.
- PII record count matters: healthcare, finance, children, government IDs, and location data raise severity.
- Credential exposure is heavily policy-dependent. Confirm the program accepts leaked-secret reports before spending time.
- Debug endpoints pay when they reveal secrets, request data, process args, or internal admin topology.
- Source disclosure pays when it includes private repo contents, deploy secrets, or proprietary business logic.

**Report framing**
- Weak: "The `.env` file is public."
- Strong: "The public `.env` exposes a live AWS access key. `aws sts get-caller-identity` proves the key belongs to the target account and role `prod-app-readonly`. I did not list or download buckets; the role identity is sufficient to prove credential compromise."
- Expected pushback: "No customer data shown." Rebuttal: "I intentionally stopped at credential validation. The exposed role identity demonstrates unauthorized access potential without unsafe data access."
- Expected pushback: "This is a public bucket." Rebuttal: "The listed objects include private KYC document names and three redacted samples from my own account prove unauthenticated read."

**Automation harness**
- Use a subdomain-to-exposure scanner with per-path safe fetch limits: max bytes, max lines, no recursive download by default.
- Pipe secrets through verified detectors only, then validate with provider identity calls rather than data reads.
- Keep `evidence.json`: URL, status, content-type, bytes read, sensitive pattern, validation command, redaction status.
- Add a hard stop flag when a live credential is found; the next action should be report writing, not pivoting.
