---
name: chain-templates
description: chain-templates reference material.
---

# Chain & escalation templates

Part of `skills/hunt-info-disclosure/SKILL.md`. Read this ONLY after you already have ONE confirmed info-disclosure primitive and want to escalate it further — not needed to find the first bug.

---

## Chains & Multi-Bug Templates

Single-bug info-disclosure pays per-bug; chains pay 5-10x. Eight templates from disclosed reports and current-meta 2024-2026 chains.

**Chain 1 — `spring actuator heapdump → aws keys → s3 bucket exfil` (Volkswagen pattern, mid five-figure on enterprise programs)**
- Bug A: Subdomain enumeration finds Spring Boot service on `<svc>.target.com` with `Server: Tomcat` header
- Bug B: `/actuator/health` returns 200 — actuator exposed
- Bug C: `/actuator/heapdump` returns 200 with binary heap dump (~500MB-2GB)
- Bug D: `strings heapdump.bin | grep AKIA` reveals AWS access key in JVM memory
- Bug E: `aws sts get-caller-identity` confirms key is live; key has S3 read access
- Bug F: `aws s3 ls` lists customer-data buckets; download 3 sample records as proof
- Outcome: Single exposed actuator endpoint → full cloud infrastructure read access → terabytes of customer data exfil potential
- Bounty range: mid five-figure on enterprise programs (Volkswagen 9TB GPS data is the precedent)
- Disclosed source: https://www.syscrest.com/2025/02/securing-spring-boot-actuator/ (SYSCREST analysis Feb 2025); https://www.cyberkendra.com/2024/12/vulnerability-in-spring-boot-actuator.html (Wiz Threat Research Dec 2024 study: 60% of cloud envs use Spring Actuator, 11% expose publicly).

**Hunter's note:** the trick that makes this pay isn't finding the actuator — nuclei templates do that automatically. The trick is the heapdump analysis. Most hunters stop at "actuator exposed" (mid four-figure) when they should download the heapdump and grep for AWS keys (mid five-figure). The first time I tried this I downloaded a 1.2GB heapdump and ran `strings | grep -E 'AKIA[0-9A-Z]{16}'` — got 3 access keys in 30 seconds. Then `aws sts get-caller-identity` confirmed one was live with S3 access. Stop after 3 sample records. Don't pivot — that's unauthorized data access escalation.

**Chain 2 — `.git/ exposed → repo recovery → trufflehog → live aws keys → cloud exfil` (Sysdig EmeraldWhale pattern, low to mid five-figure when keys validate)**
- Bug A: Subdomain enumeration discovers `<old-app>.target.com`
- Bug B: `curl https://<old-app>.target.com/.git/HEAD` returns 200 with `ref: refs/heads/main` — `.git/` exposed
- Bug C: `git-dumper https://<old-app>.target.com/.git/ /tmp/dumped` recovers full repo
- Bug D: `trufflehog filesystem /tmp/dumped --only-verified --json` extracts 5 live secrets including AWS keys
- Bug E: AWS keys validate to staging-environment role with read access to internal data
- Outcome: Forgotten dev subdomain → full source code + active credentials → cloud-environment read
- Bounty range: low to mid five-figure when AWS keys validate to non-trivial role
- Disclosed source: https://sysdig.com/blog/emeraldwhale (Sysdig Threat Research Oct 2024 — EmeraldWhale stole 15K cloud credentials via this exact pattern); https://www.bleepingcomputer.com/news/security/hackers-steal-15-000-cloud-credentials-from-exposed-git-config-files/.

**Hunter's note:** the move that pays here is using `git-dumper` to recover the full repo, not just reading `.git/config`. Most hunters report "/.git/HEAD exposed" (mid three-figure informational) without realizing they can clone the entire codebase. Once you have the repo, TruffleHog with `--only-verified` filters out dead keys; the live ones are gold. The first time I ran this, the repo had been deleted from production years prior but the deploy server still had `.git/` from initial setup — keys were rotated since but the SSH private key in `.deploy/keys/` was still valid for the bastion.

**Chain 3 — `subdomain enum → s3 bucket guess → public listing → pii exfil` (ESHYFT pattern, low to mid five-figure on healthcare/financial programs)**
- Bug A: Subdomain enumeration identifies `app.target.com`, `api.target.com`, `prod.target.com`, `staging.target.com`
- Bug B: For each subdomain, try as bucket name with common suffixes (`-prod`, `-backup`, `-uploads`, `-data`)
- Bug C: `https://target-uploads.s3.amazonaws.com/` returns XML directory listing
- Bug D: List contains PII — KYC documents, ID scans, customer-uploaded files
- Bug E: Download 3 sample files (own test account preferred) to demonstrate read access
- Outcome: Public S3 bucket with PII; reportable as critical given record count
- Bounty range: low to mid five-figure on healthcare / financial / fintech programs (ESHYFT 86K records is the pattern)
- Disclosed source: https://www.theregister.com/2025/03/11/uber_for_nurses_exposes_86k/ (ESHYFT March 2025 — 108GB / 86,341 records); Bugcrowd NASA disclosure at https://bugcrowd.com/disclosures/aa45924f-8b67-4f22-8dff-dac7dc9d60e2/exposure-of-pdf-file-in-a-public-amazon-s3-bucket-associated-with-nasa.

**Hunter's note:** the trick is the subdomain-to-bucket-name derivation. Most hunters guess `<orgname>` and `<orgname>-bucket` and stop. Real value is in derivative names — `<subdomain>-prod`, `<subdomain>-staging`, `<subdomain>-backup`, `<subdomain>-uploads`, `<subdomain>-logs`, `<subdomain>-data`. ESHYFT's bucket was discoverable via this pattern. Always count the records before reporting — programs scale bounty by record count for healthcare/financial PII.

**Chain 4 — `actuator env exposed → db credentials → external db connection → read prod data` (Spring config-property pattern, mid four-figure to low five-figure)**
- Bug A: `/actuator/env` returns 200 with full environment dump
- Bug B: Response contains `spring.datasource.url=jdbc:postgresql://db.internal.target.com:5432/prod_db` and `spring.datasource.password=<plaintext>`
- Bug C: Internal hostname `db.internal.target.com` resolves publicly (DNS misconfig); or, more commonly, port forward via SSRF chain
- Bug D: Connect with the leaked credentials, read schema, query 3 sample records
- Outcome: Full production database access via single info-disclosure
- Bounty range: mid four-figure to low five-figure depending on data type
- Disclosed source: Wiz Threat Research Dec 2024 documented `/actuator/env` exposure in 4% of publicly accessible Spring Boot Actuator instances (https://www.cyberkendra.com/2024/12/vulnerability-in-spring-boot-actuator.html).

**Hunter's note:** the bottleneck is reaching the internal DB. Sometimes the DB hostname resolves publicly (DNS misconfig — file separately as info-disclosure). When it doesn't, you need an SSRF in the same target to pivot. Even without the SSRF, the credential leak alone is mid four-figure. Always try connecting from outside; many "internal" DBs are accidentally public.

**Chain 5 — `debug pprof cmdline → admin token → unauthorized api access → full account takeover` (Dgraph pattern, low five-figure on Dgraph-class targets)**
- Bug A: `/debug/pprof/cmdline` returns 200 with full process command line
- Bug B: Response contains `--security "token=admin-secret-xxx"`
- Bug C: Use the token: `curl -H "X-Dgraph-AccessToken: admin-secret-xxx" https://target/admin`
- Bug D: Admin API access — read schema, drop predicates, modify users
- Outcome: Unauthenticated debug endpoint → admin token → full database control
- Bounty range: low five-figure when admin actions confirmed
- Disclosed source: GHSA-95mq-xwj4-r47p (Dgraph 2026 critical — referenced in corpus); same pattern in any Go service that uses `--flag="value"` style for secrets and exposes pprof.

**Hunter's note:** every Go service that uses `import _ "net/http/pprof"` and accepts secret arguments via `--flag` is vulnerable to this pattern. The Dgraph case showed admin tokens; the same pattern leaks DB connection strings, encryption keys, OAuth client secrets. Always probe `/debug/pprof/cmdline` on Go services. The fix is to remove pprof from production builds entirely, not "secure" it via `mux.Handle`.

**Chain 6 — `wp-config.php backup → db credentials → wp-admin login → arbitrary plugin upload → rce` (mid four-figure on WordPress targets)**
- Bug A: `curl https://target/wp-config.php.bak` returns 200 with PHP source
- Bug B: Extract DB credentials, prefix, salts, AUTH_KEY, secret keys
- Bug C: Use AUTH_KEY to forge admin session cookies (wp_set_auth_cookie equivalent)
- Bug D: Login to wp-admin, upload malicious plugin → RCE
- Outcome: backup-file exposure → DB credentials + AUTH_KEY → arbitrary RCE
- Bounty range: mid four-figure to low five-figure when RCE confirmed
- Disclosed source: H1 reports 3328408 (National Guard) and 3252302 (2026 high) at https://hackerone.com/reports/3328408 and https://hackerone.com/reports/3252302.

**Hunter's note:** WordPress is everywhere, and admin/dev/staging subdomains often serve `wp-config.php.bak` or `~wp-config.php` because someone edited via vim/nano on the live server. The chain to RCE requires either DB access (pivot to wp_users table to set admin password hash) or AUTH_KEY forgery. Both work; AUTH_KEY forgery is faster but requires understanding WordPress's nonce/cookie generation. Always check both `.bak` and `~` and `.swp` extensions.

**Chain 7 — `swagger json exposed → endpoint enum → admin api → full data dump` (low to mid four-figure direct + chain to other paying classes)**
- Bug A: `/swagger.json`, `/api-docs`, `/openapi.json` exposed in production
- Bug B: Schema reveals admin endpoints not in public docs (`/api/v1/admin/users/all`, `/api/v1/admin/export`)
- Bug C: Test admin endpoints with regular user JWT — some return 200 (BFLA → cross to hunt-idor for full chain)
- Bug D: Admin endpoint returns full user dump or system info
- Outcome: Hidden admin API discovered → cross-priv access → bulk data exfil
- Bounty range: low to mid four-figure direct + per-bug-class chains
- Disclosed source: Pattern documented across H1 hacktivity 2024-2025; canonical reference at https://owasp.org/API-Security/editions/2023/en/0xa1-broken-object-level-authorization/ (OWASP API1:2023 — see hunt-idor.md for the BFLA half of the chain).

**Hunter's note:** Swagger / OpenAPI exposure alone is mid three-figure informational. The value is in the endpoints it reveals. Compare the public docs to the swagger schema — anything in swagger that's NOT in public docs is an undocumented surface, often missing the same auth checks as the documented endpoints. Always download the full schema and test the new endpoints with low-priv credentials.

**Chain 8 — `mobile app reverse → embedded api key → cloud service abuse → bulk data exfil` (low to mid four-figure on mobile programs)**
- Bug A: Pull APK from Play Store / IPA from app store
- Bug B: `apktool d` + `strings classes.dex | grep -E 'AKIA|AIza|sk_live'` — extract embedded keys
- Bug C: Validate keys against vendor API (`aws sts get-caller-identity`, Stripe API, etc.)
- Bug D: Abuse the key — list S3 buckets, query Stripe for customer charges, etc. — limited to demonstration
- Outcome: Mobile app secret leak → cloud service abuse
- Bounty range: low to mid four-figure on mobile programs (Shopify, GitLab, Vercel)
- Disclosed source: Pattern documented across H1 mobile hacktivity 2024-2026; tools at https://github.com/MobSF/Mobile-Security-Framework-MobSF.

**Hunter's note:** modern mobile apps use ProGuard / R8 obfuscation, but secrets often remain in plaintext in `assets/`, `res/raw/`, or environment-config XML. Always check `META-INF/` for build-time leakage. The H1 2026 "Session Cookie Leakage via Static Header Field in WebViewerFragment" is a related class — mobile WebView leaks session cookie via static custom header even after logout.

