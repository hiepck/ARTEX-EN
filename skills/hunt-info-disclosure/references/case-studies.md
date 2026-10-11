---
name: case-studies
description: case-studies reference material.
---

# Real impact examples / case studies

Part of `skills/hunt-info-disclosure/SKILL.md`. Read this ONLY when you want full narrative write-ups of disclosed bugs for report-framing inspiration — not needed to find or validate the first bug.

---

## Real Impact Examples

**Example 1 — `volkswagen-spring-actuator-heapdump-aws-keys-9TB-gps-data` (mid five-figure bounty range, multi-link cloud-takeover chain — Wiz Threat Research Dec 2024 disclosure)**
- Setup: Volkswagen telematics service running Spring Boot. `/actuator/heapdump` endpoint exposed without authentication — the default Spring Boot Actuator behavior when `management.endpoints.web.exposure.include=*` is set.
- Discovery: Wiz Threat Research team during routine cloud-misconfiguration analysis. Heap dump downloadable via single GET request to `/actuator/heapdump`. The dump contained the full JVM memory state including environment variables, in-flight request data, and AWS SDK credential cache.
- Exploitation: Researchers downloaded the heap dump (~1-2 GB binary). Ran `strings heapdump.bin | grep -E 'AKIA[0-9A-Z]{16}'` to extract AWS access keys. Validated keys via `aws sts get-caller-identity`. The keys had S3 read access to a bucket containing GPS tracking data for Volkswagen-connected vehicles.
- Impact: 9 terabytes of GPS data covering hundreds of thousands of cars exfiltratable. Combined location data with vehicle identifiers (VINs) — direct PII / surveillance impact. Could have enabled mass-tracking of Volkswagen owners.
- Disclosed source: Wiz Threat Research disclosure (Dec 2024); SYSCREST analysis at https://www.syscrest.com/2025/02/securing-spring-boot-actuator/ (Feb 2025); Cyber Kendra summary at https://www.cyberkendra.com/2024/12/vulnerability-in-spring-boot-actuator.html. Wiz documented 60% of cloud environments use Spring Boot Actuator and 11% expose publicly. Bounty paid via Volkswagen's program in mid five-figure tier (specific amount not publicly disclosed).

**Example 2 — `sysdig-emeraldwhale-15K-cloud-creds-from-67K-git-configs` (mass exploitation pattern, mid four-figure direct on individual program disclosures — Sysdig Threat Research Oct 2024)**
- Setup: Web servers across thousands of domains misconfigured to serve `/.git/config` at the application root. Includes Laravel apps with `.env` files in webroot.
- Discovery: Sysdig Threat Research Team (TRT) analyzed the EMERALDWHALE campaign — automated tool scanning IP ranges for exposed Git configuration files. Identified 67,000 URLs exposing config files across the internet. The tool extracted authentication tokens, validated them via curl against various APIs (AWS, GitHub, Mailgun, etc.), then downloaded private repos using validated GitHub tokens.
- Exploitation: Attacker tooling automated the chain: scan → fetch `.git/config` → extract tokens → validate → download private repos → recursive secret extraction from repo contents. Stolen secrets stored in 1TB S3 bucket. Stolen 15,000 cloud credentials from 67,000 URLs (28K Git repos, 6K GitHub tokens, 2K validated active credentials).
- Impact: Mass credential theft enabling cloud-account takeover, private-repo exfiltration, downstream supply-chain attacks. Sysdig observed Multigrabber v8 tool used for the exfil. Credentials sold on Telegram for $100 per URL list.
- Disclosed source: https://sysdig.com/blog/emeraldwhale (Sysdig TRT Oct 2024); https://www.bleepingcomputer.com/news/security/hackers-steal-15-000-cloud-credentials-from-exposed-git-config-files/. The defensive bug-bounty version: hunters running the same scan against in-scope subdomains find similar exposures and report individually — bounties range from low four-figure (single .env / .git exposure) to mid four-figure (active AWS keys validated).

**Example 3 — `eshyft-public-s3-bucket-86K-medical-records` (researcher disclosure pattern, healthcare PII — Jeremiah Fowler March 2025)**
- Setup: ESHYFT (New Jersey-based healthcare staffing platform — "Uber for nurses") had an Amazon S3 bucket configured with public access. The bucket was non-password-protected, unencrypted, and contained sensitive health-care worker data.
- Discovery: Cybersecurity researcher Jeremiah Fowler discovered the bucket on January 4 2025 via routine cloud-asset scanning (subdomain enumeration → bucket-name guess → public listing test). The bucket contained 108.8 GB of data with 86,341 records.
- Exploitation: Fowler identified the bucket via standard enumeration techniques, confirmed public access via `curl https://<bucket>.s3.amazonaws.com/`, downloaded a sample of records to confirm content. Did not exfiltrate at scale — reported to ESHYFT on January 6 2025.
- Impact: 86,341 records exposed — user profile pictures + facial images, scanned drivers licenses, Social Security cards, professional certificates, work assignment agreements, CVs/resumes, medical diagnoses, prescription records, disability insurance claims. HIPAA-implicating exposure.
- Disclosed source: https://www.theregister.com/2025/03/11/uber_for_nurses_exposes_86k/ (The Register Mar 11 2025); https://beyondmachines.net/event_details/over-86000-healthcare-worker-records-leaked-in-eshyft-s-unsecured-s3-bucket-8-4-g-i-1/gD2P6Ple2L. ESHYFT took >1 month to close the bucket after notification. Pattern: most healthcare/financial bug bounty programs pay mid four-figure to low five-figure for similar findings depending on record count and PII sensitivity tier.

**Example 4 — `dgraph-debug-pprof-cmdline-admin-token-leak` (low five-figure bounty range on Dgraph-class targets, Go service info-disclosure pattern, GHSA-95mq-xwj4-r47p 2026 critical)**
- Setup: Dgraph Alpha instance deployed with `--security "token=<admin-token>"` for admin API authentication. Process exposed `/debug/pprof/cmdline` endpoint via default Go `net/http/pprof` import with no authentication.
- Discovery: Researcher probed `/debug/pprof/cmdline` — a standard Go pprof endpoint that returns `argv` of the running process. Response contained the full command line: `dgraph alpha --security "token=actual-admin-token-value-here" --my=...`.
- Exploitation: Extracted admin token from `/debug/pprof/cmdline` response. Used token to authenticate to admin API: `curl -H "X-Dgraph-AccessToken: <token>" https://target/admin`. Admin API allows schema modification, predicate drops, user management — full database control.
- Impact: Unauthenticated debug endpoint → admin token disclosure → full database admin access. The fix: remove `import _ "net/http/pprof"` from production builds, or restrict pprof to a separate non-public mux.
- Disclosed source: GHSA-95mq-xwj4-r47p (2026 critical, Dgraph). Pattern repeats across every Go service that uses `--flag="value"` for secrets and exposes pprof — same class of bug in many internal Go microservices.

**Example 5 — `nasa-bitbucket-env-uat-credentials` (low four-figure bounty range on equivalent commercial paid programs, kudos-only on NASA VDP — _x3ro_ Bugcrowd Aug 2025)**
- Setup: NASA's Bitbucket server hosted a `.env` file accessible via HTTP. The file contained UAT-environment credentials used by a script to authenticate with NASA's `cmr.sit.earthdata.nasa.gov` SIT (System Integration Testing) token service.
- Discovery: Researcher `_x3ro_` ran subdomain / asset enumeration against NASA VDP scope, then path probing for `.env` and other config-file exposure. Found the `.env` accessible without authentication.
- Exploitation: Downloaded `.env`, identified plaintext credentials for `cmr.sit.earthdata.nasa.gov`. Validated credentials authenticated successfully against the NASA SIT token service. Did not pivot — submitted finding directly with credential validation proof.
- Impact: Live UAT credentials exposed — could enable unauthorized access to NASA earthdata systems, data extraction, API abuse. NASA classified P3 (medium severity) per Bugcrowd VRT.
- Disclosed source: Bugcrowd disclosure at https://redpacketsecurity.com/bugcrowd-bugbounty-disclosure-publicly-accessible-env-file-exposing-hardcoded-credentials-on-nasa-s-git-repository (disclosed by _x3ro_, 2025-08-20). NASA VDP is informational/kudos only — but the same finding on a paid program (commercial SaaS) would be low to mid four-figure depending on credential scope.

**Example 6 — `glances-central-browser-serverslist-downstream-credentials` (low five-figure bounty range on monitoring platforms and internal SaaS observability stacks — GHSA-r297-p3v4-wp8m / GHSA-gfc2-9qmw-w7vh 2026 critical)**
- Setup: Glances Central Browser mode exposed `/api/4/serverslist` without adequate access control. The endpoint returned the configured downstream server inventory, including connection metadata and credentials used by the central node.
- Discovery: Researcher or hunter fingerprints Glances via `/api/4/status`, `/api/4/pluginslist`, permissive CORS, and the Central Browser UI, then requests `/api/4/serverslist` directly from an unauthenticated browser or curl session.
- Exploitation: Response body contains reusable host, port, username, password, token, or URL material for monitored servers. Validate only that the credential shape and target ownership are real; do not log into downstream infrastructure unless the program explicitly authorizes validation.
- Impact: One unauthenticated monitoring endpoint becomes a credential inventory leak for the fleet. On a SaaS or managed-infrastructure program this is materially stronger than "dashboard exposed" because it proves reusable secrets for internal systems.
- Disclosed source: GHSA-r297-p3v4-wp8m and GHSA-gfc2-9qmw-w7vh (Glances 2026 critical). Treat as the canonical report frame for any observability product that exposes configured downstream targets or agent tokens through inventory APIs.

