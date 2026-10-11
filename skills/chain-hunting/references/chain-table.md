---
name: chain-table
description: Capability-to-next-bug chain anchors: for each confirmed vuln class, the next hops and terminal impacts to pursue.
---

# Chain Table — Capability → Next Bug

Reference file for chain-builder agent and autopilot. Do not duplicate this content elsewhere.

## The Chain Walk Algorithm

1. START with confirmed bug A
2. Map what A GIVES you (capabilities/primitives)
3. Search this table for what takes A's output as input
4. Test the top candidate (B)
5. If B confirmed → map combined capabilities → check terminal impact → if not terminal, B becomes new A → go to 3
6. If B fails → try next candidate (max 3 failures per depth)
7. Report chain so far when terminal impact reached or candidates exhausted

## Capability → Next Bug Table

| You Have (Capability) | Look For (Next Link) | Combined Gives You |
|---|---|---|
| **JS execution in victim context** | HttpOnly not set? → cookie theft | Session token |
| | CSRF token accessible → forge requests | Authenticated actions |
| | postMessage listener unchecked → inject messages | Control over app state |
| | DOM access → read sensitive data | PII, tokens, keys |
| **Arbitrary text injection** | Input evaluated/executed → code execution | JS execution |
| | Input rendered in another context → stored XSS | JS execution in other users |
| | Input sent to API → parameter injection | API abuse |
| **Control over URL/redirect** | OAuth redirect_uri → steal auth code | OAuth token |
| | Open redirect → phishing from trusted domain | Credential theft |
| | iframe src control → clickjacking | UI manipulation |
| **Cookie control (set/read)** | Cookie bomb (overflow headers) → block callbacks | Force error pages |
| | Session fixation → set known session ID | Session hijack |
| | Cookie tossing → override subdomain cookies | Auth confusion |
| **Cross-origin window reference** | window.location readable → URL theft | Tokens in URL |
| | postMessage to window → inject data | State manipulation |
| | window.opener control → tabnabbing | Phishing |
| **SSRF (make server requests)** | Hit cloud metadata → IAM credentials | Cloud access |
| | Hit internal services → access admin panels | Internal access |
| | Hit localhost → bypass IP allowlists | Auth bypass |
| **IDOR (read other user's data)** | Read auth tokens → impersonate | ATO |
| | Read PII → data breach | Privacy violation |
| | Write to other user → modify account | Account manipulation |
| **File write/upload** | Write to web root → web shell | RCE |
| | Write SVG → stored XSS | JS execution |
| | Write config → modify app behavior | App takeover |
| **DNS control (subdomain)** | Subdomain is OAuth redirect_uri → token theft | ATO |
| | Subdomain serves content → trusted phishing | Credential theft |
| | Subdomain has wildcard cert → MitM | Traffic interception |

## Terminal Impacts (stop chaining, report)

- **Account Takeover (ATO)**: stolen session, OAuth token, password reset
- **Remote Code Execution (RCE)**: server-side code exec, web shell
- **Mass Data Exfiltration**: bulk PII, financial data, credentials
- **Full Admin Access**: privilege escalation to admin role
- **Infrastructure Compromise**: cloud creds → full environment access

## Known Deep Chains (real-world examples)

### 9-Link: Self-XSS → ATO (Renwa 2026)
A: Self-XSS in code editor → B: Cross-origin drag-drop injection → C: Scroll-to-fragment focus → D: Unchecked postMessage listener → E: Victim clicks Evaluate → F: DOM-XSS reads CSRF + OAuth → G: Cookie bomb blocks callback → H: Same-origin URL read extracts OAuth code → I: Exchange code → ATO

### 4-Link: S3 → OAuth → ATO
A: S3 bucket publicly listable → B: JS bundles contain OAuth client_secret → C: OAuth flow doesn't enforce PKCE → D: Intercept auth code via manipulated redirect_uri → ATO

### 5-Link: Subdomain Takeover → ATO
A: Dangling CNAME → claim subdomain → B: Subdomain is OAuth redirect_uri → C: Cookie tossing on parent domain → D: Session fixation via tossed cookie → E: Victim authenticates → ATO

### 6-Link: Prompt Injection → Admin
A: LLM chatbot follows injected instructions → B: IDOR via AI (other user data) → C: Markdown image exfil → D: Exfiltrated API keys → E: Internal service access → F: Admin promotion endpoint → Admin

## Process Rules

1. Confirm each link with exact HTTP request/response
2. Map capabilities after each link
3. Search writeup DB at each step: `search_writeups "<capability> escalation"`
4. 20-minute time box per link
5. Max 3 failed candidates per depth
6. Each link must be DIFFERENT (endpoint, mechanism, or impact)
7. Each link must be PROVABLE (exact request/response)
8. Report the FULL chain as one submission — chains pay more

## Per-Class Chain Anchors (FEEDER discipline)

When a hunter confirms a finding in any of the classes below, that finding
is **a feeder, not a report**. The hunter MUST immediately probe the listed
anchors before declaring the finding complete. If any anchor returns signal,
the result is a chain candidate; dispatch chain-builder. If all anchors
fail, the finding is informational at best — apply the never-submit rule.

The hunt and autopilot dispatchers inject these anchors into the hunter's
task preamble so the hunter knows what to test next without an extra round
trip.

### `open-redirect` — anchors

Standalone is on the never-submit list. Anchors:

1. **OAuth redirect_uri reflection** — find every OAuth/OIDC client in the target. Try `?redirect_uri=<your-redirected-domain>`. If the auth code is delivered to your domain → ATO chain confirmed.
2. **`returnTo` / `next` / `continue` after auth** — submit `returnTo=javascript:alert(document.cookie)`. If the SDK uses `location.href = returnTo`, that's CVE-2025-67716 class.
3. **SAML RelayState / OIDC `post_logout_redirect_uri`** — try `RelayState=<svg onload=...>` or `post_logout_redirect_uri=<external>`.
4. **Login flow CSRF anchor** — does the redirect happen post-login? Self-XSS on the redirect target + login CSRF = ATO.
5. **Cookie tossing prerequisite** — does the redirect target a sibling subdomain? If yes + you control any subdomain → cookie tossing chain.

### `cors-hunter` — anchors

CORS wildcard alone is on the never-submit list. Anchors:

1. **Credentialed authenticated endpoint** — find an endpoint behind auth with `Access-Control-Allow-Credentials: true`. Without this, CORS misconfig is informational.
2. **Sensitive data endpoint** — does the over-permissive origin reach `/api/me`, `/api/users/<id>`, billing, secrets? Document the exact data exfiltrated.
3. **Origin-reflection + `null`** — tests with `Origin: null` (sandboxed iframes / data: URIs) reveal weakly-coded origin checks.
4. **Subdomain wildcard regex flaw** — `https://target.com.attacker.com`, `https://nottarget.com`, `https://target.com\.attacker.com` — origin-check regex bypasses.
5. **Cross-origin postMessage handler** — find a `window.addEventListener('message', ...)` without origin validation and abuse it as the data-theft sink.

### `info-disclosure` — anchors

Standalone info disclosure is always-rejected unless chained. Anchors:

1. **Bundle / source / config containing OAuth secrets** → pursue an OAuth code-interception hop (client_secret + missing PKCE).
2. **Stack trace revealing internal IPs / service names** → ssrf-hunter (now you know what to point SSRF at).
3. **Debug endpoint returning request headers** → IDOR / session fixation candidate (sessions visible to attacker).
4. **`.git`, `.env`, `backup.tar.gz`, `wp-config.php` exposed** → if it leaks DB credentials → privilege-escalation; if it leaks signing keys → JWT alg confusion → pursue an OAuth/JWT hop.
5. **Cloud metadata reachable via XSS context (window.fetch)** → IMDS theft chain (XSS + open SOP to 169.254.169.254).
6. **API key in JS bundle with active scope** — verify the scope. If it accesses other-user data → IDOR-via-key.

### `csrf` — anchors

CSRF on isolated forms is low/medium. Anchors:

1. **CSRF on password / email / phone change** → ATO chain (changes the recovery vector).
2. **CSRF on MFA disable / second-factor enrollment** → MFA bypass.
3. **CSRF on role change / permission grant / team invite** → privilege escalation.
4. **CSRF on payment-method change / withdrawal address** → financial impact.
5. **CSRF on OAuth client registration / API key creation** → backdoor-credential chain.

### `subdomain-takeover` — anchors

Standalone takeover is medium without chain. Anchors:

1. **Subdomain is OAuth redirect_uri / SAML ACS / OIDC issuer** — claim → ATO chain.
2. **Parent domain shares cookies (`.target.com`)** → cookie tossing → session fixation → ATO.
3. **Subdomain has wildcard cert** → MitM / TLS-confusion chain.
4. **Subdomain is referenced from prod domain JS** (CDN, asset, config) → trusted-domain phishing → credential theft.
5. **Subdomain is in CSP `script-src`** → CSP bypass on the parent → stored XSS escalation.

### `xxe-hunter` — anchors

In-band XXE alone (read /etc/passwd) is informational on cloud-hosted apps. Anchors:

1. **SSRF via XXE → cloud metadata** (`SYSTEM "http://169.254.169.254/..."`) → IAM creds → infrastructure compromise.
2. **OOB exfil to attacker DTD** → blind XXE on cookie / config / private files.
3. **SAML XXE** (assertion parsing) → authentication bypass via signed-assertion forgery.
4. **DOCX / XLSX / SVG XXE upload** — payload survives the upload pipeline → triggers on internal viewer (admin context).
5. **PHP wrapper / Java JNDI** — `php://filter/read=convert.base64-encode/...` for source code, `jar://` / `ldap://` for classloader RCE.

### `file-upload` — anchors

Bypassing extension/MIME alone is informational unless the upload goes somewhere useful. Anchors:

1. **Upload to web root + executable** → web shell chain → RCE.
2. **Upload SVG / HTML rendered inline** → pursue a stored-XSS hop (viewer context).
3. **Path traversal in filename → overwrite config / cron / .ssh/authorized_keys** → privilege escalation / RCE.
4. **Upload metadata renders in admin panel** (filename, EXIF, EXIF GPS) → stored XSS in admin context → ATO.
5. **Upload triggers server-side processor** (PDF render, image resize, antivirus) → SSRF / RCE via processor CVE (libheif, ImageMagick, Ghostscript).

### `race-condition` — anchors

Race confirmed on a low-impact action is low. Anchors (the multiplier):

1. **Race on financial action** (transfer, withdrawal, balance debit) → quantify the dollar amount.
2. **Race on coupon / gift-card / referral redemption** → quantify (free-product * N).
3. **Race on one-shot tokens** (password-reset, invite-accept, MFA-enrollment) → ATO if redeemed twice.
4. **Race on file write check** → TOCTOU → privilege escalation / RCE.
5. **Race on rate-limit / quota check** → bypass quota → mass enumeration / mass scraping → IDOR amplification.

### `business-logic` — anchors

Standalone business-logic findings (price manipulation, coupon abuse) need impact quantification. Anchors:

1. **Public archive / share-with-admin trigger** (listmonk pattern) — does the manipulated artifact get shown to a higher-privilege user?
2. **State carries to other context** — manipulated price → stored on server → renders in admin panel where the price is the source-of-truth.
3. **Workflow skip → access feature you didn't pay for** — quantify dollar value (premium feature × users).
4. **Negative / huge values → integer overflow / sign flip** → financial chain.
5. **Time-of-check / time-of-use on balance** → race-condition chain.

### `privilege-escalation` — anchors (when found via parameter manipulation, mass assignment, etc.)

Vertical privilege escalation is usually terminal — but check:

1. **Mass-assignment to set role / permission / tenant** — does the escalated account see other-tenant data? → IDOR amplification.
2. **JWT claim manipulation works** — verify which other claims are unprotected (sub, aud, iss) → cross-tenant chain.
3. **Admin endpoint reachable but rate-limited** — confirm full admin actions, not just GET.
4. **Forced-browsing admin URL works** → check write operations (DELETE, PATCH) too.
5. **HTTP method override → bypass auth** — try the same trick on every other admin endpoint.
