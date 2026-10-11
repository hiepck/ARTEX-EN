---
name: chain-templates
description: chain-templates reference material.
---

# Chains & Multi-Bug Escalation Templates

Part of `skills/hunt-oauth/SKILL.md`. Read this ONLY when you already have ONE confirmed OAuth/OIDC/SAML/JWT primitive and want to escalate it into a higher-value multi-bug chain.

---

## Chains & Multi-Bug Templates

**Chain 1 — Open Redirect → OAuth Token Theft → Account Takeover (the universal $5K-$15K chain)**
- Bug A: Open redirect on `https://app.target.com/redirect?url=` (CWE-601), filed standalone = $150 Low
- Bug B: OAuth `redirect_uri` registered as `https://app.target.com/oauth/callback` with allowlist by host prefix
- Bug C: Set `redirect_uri=https://app.target.com/redirect?url=https://attacker.com` — AS validates host prefix, browser then follows the in-host redirect to attacker
- Outcome: Attacker captures victim's `code` or `access_token`, exchanges for session, full ATO including admin
- Bounty range: $5,000-$15,000 (DEV.to lucky_lonerusher 2026 reports $15K confirmed payout; H1 corpus consistently mid-four-figure to low-five-figure)
- Disclosure source: HackerOne #665651 (Stealing Users OAuth Tokens through redirect_uri parameter), HackerOne #770548 (Insecure OAuth redirection at admin.8x8.vc, $High), Pixiv H1 #1861974 ($2,000)

**Hunter's note:** This is the *first chain you should attempt on every program*. Don't file the open redirect alone — every triager kills standalone open redirects as Low. The pivot that separates a $150 Low from a $15K Critical is twenty minutes spent finding the OAuth flow on the same host. The open redirect must be on a host that's allowed as a redirect_uri; check the application's OAuth configuration page (often `/developers`, `/settings/applications`, or third-party login scope page) to confirm the host is whitelisted. The trick most hunters miss: the open-redirect endpoint doesn't have to be a *vulnerability* — many products ship intentional `?next=` redirects with naive same-origin checks. Lovable.dev H1 #3599248 demonstrated even a fix bypass (the `/..//` path-traversal trick) is enough.

**Chain 2 — nOAuth + Account Merge → Cross-Tenant Microsoft Account Takeover**
- Bug A: Target uses "Sign in with Microsoft" (Entra ID multi-tenant)
- Bug B: Target's OAuth user-mapping uses `email` claim instead of `oid` / `sub` (CVE-2024-21632 pattern)
- Bug C: Account merge logic links new OAuth login to existing local account by email
- Outcome: Attacker creates free Entra tenant, sets mail attribute to victim's email, logs in to target, fully takes over victim's account — bypassing MFA, password, all conditional access
- Bounty range: $5,000-$75,000 (Descope's coordinated nOAuth disclosure paid $75K+ across multiple programs; CVE-2024-21632 GHSA disclosed bug in omniauth-microsoft_graph, Recognize.app program; Semperis 2025 found 9% of Entra Gallery apps still vulnerable)
- Disclosure source: CVE-2024-21632, GHSA-5g66-628f-7cvj, Descope blog post June 20 2023, Semperis blog June 25 2025

**Hunter's note:** What worked: registering a brand-new Entra tenant takes 5 minutes via free.azure.microsoft.com — no credit card for nOAuth testing. Set the user's `mail` attribute (Contact Information) to the victim's email; do NOT use the UPN. The xms_edov claim was introduced post-June-2023 to mitigate this, but Microsoft set `removeUnverifiedEmailClaim=true` only for *new* app registrations after June 2023; **the default for older apps is still vulnerable**. Why this pays where the single bug doesn't: standalone "you trust the email claim" without a working PoC is theoretical and gets closed Informative. Demonstrate the merge against a real victim account *you own* (your second account on the target) and the bug becomes Critical.

**Chain 3 — SAML Parser Differential → Unauthenticated Admin on GitLab Enterprise (the SAML Roulette chain)**
- Bug A: Target uses `ruby-saml < 1.18.0` (or `omniauth-saml` referencing ruby-saml < 1.18.0) — fingerprint via login response Server header or behavior
- Bug B: Obtain *any* legitimately signed SAML assertion (your own login on a smaller tenant of the same IdP, or any partner-org assertion from federated SSO)
- Bug C: Modify XML with DOCTYPE `<!ATTLIST>` injection (CVE-2025-25291) or namespace confusion (CVE-2025-25292) so REXML and Nokogiri see different documents — REXML validates signature on the original element, Nokogiri reads attacker's NameID
- Bug D: Set NameID to admin user (often known: `admin@target.com`, `root@target.com`, first user enumerated)
- Outcome: Unauthenticated admin access to the target SAML SP — for GitLab Enterprise, this is unauthenticated cluster-admin
- Bounty range: GitHub Security Lab paid undisclosed five-figure-plus for CVE-2025-25291/25292 (private bug bounty engagement, ahacker1 + Peter Stöckli + p-); GitLab Enterprise SSRF/SAML paths historically pay $10K-$20K via H1 #gitlab program
- Disclosure source: CVE-2025-25291, CVE-2025-25292, GHSL-2024-329, GHSL-2024-330, PortSwigger SAML Roulette by Gareth Heyes + Zakhar Fedotkin (March 2025), GitHub Security Lab advisory by Peter Stöckli

**Hunter's note:** What worked: SAML Raider is the right Burp extension; do not try to craft XML by hand. The hardest part of this chain isn't the parser-differential payload — it's getting *any* signed assertion. Set up a free Okta developer org or use the IdP's public sandbox; the signature on your own assertion is what you'll wrap. The DOCTYPE technique only works on Ruby < 3.4.2 because REXML truncates `!ATTLIST` differently in newer Ruby versions; combine namespace confusion with DOCTYPE for max coverage. Why this pays: SAML is the auth protocol enterprises *trust the most*, so any signature bypass is a perfect storm of "production critical" and "no detection signature." GitLab paid premium because their entire enterprise tier sells on SAML SSO security.

**Chain 4 — Subdomain Takeover → OAuth redirect_uri whitelist bypass → Federated Account Takeover**
- Bug A: Find dangling subdomain in `*.target.com` (S3, Heroku, GitHub Pages, Azure App Service, Vercel) via `subzy` / `nuclei takeover templates`
- Bug B: Claim it (e.g., `git push` to abandoned-app.heroku.com matches the dangling CNAME)
- Bug C: Target's OAuth provider whitelists `*.target.com` for redirect_uri (or uses substring match)
- Bug D: Set `redirect_uri=https://abandoned-subdomain.target.com/oauth/callback` — AS approves; victim's code lands on attacker host
- Outcome: Federated SSO account takeover for any user who clicks attacker's auth URL
- Bounty range: $2,000-$10,000 disclosed via HackerOne (subdomain takeover alone $200-$5K HackerOne program range, but chained to OAuth ATO crosses into mid-four to low-five-figure)
- Disclosure source: HackTricks OAuth wiki (`*.example.com wildcard`) at https://hacktricks.wiki/en/pentesting-web/oauth-to-account-takeover.html, HackerOne corpus pattern (multiple disclosed reports across H1 with the wildcard-takeover→OAuth chain)

**Hunter's note:** What worked: most programs treat subdomain takeover as Low/Medium standalone, but combining with OAuth ATO escalates to High/Critical because you can prove victim impact. The hardest part: programs differ on whether they accept the chain — some say "we already paid for the subdomain takeover, the OAuth angle is duplicate." Counter this in your report by emphasizing *blast radius*: standalone takeover affects only users who click attacker links going to that subdomain; OAuth chain affects every user who clicks "Login with Target" anywhere. What failed first: I tried takeover → cookie injection on the apex first; modern `__Host-` cookies blocked it. Pivot to OAuth was 30 minutes of redirect_uri fuzzing. Why this pays: it demonstrates a real-world attack path, not a hypothetical CWE-601.

**Chain 5 — MCP server misconfig → OAuth code interception → Agentic LLM API abuse → Credential exfil**
- Bug A: Find MCP server (HTTP endpoint advertising `WWW-Authenticate: Bearer realm="MCP"` or `/.well-known/oauth-protected-resource`)
- Bug B: MCP server uses `cloudflare/workers-oauth-provider < 0.0.5` → CVE-2025-4143 (no redirect_uri validation) + CVE-2025-4144 (PKCE downgrade)
- Bug C: Send victim crafted authorize URL with `redirect_uri=https://attacker.com` and no `code_challenge`
- Bug D: Capture victim's code, exchange at token endpoint with attacker's `code_verifier` (PKCE downgrade allows it)
- Bug E: Use stolen MCP token to invoke MCP tools (e.g., D1 database read, GitHub repo content read, AWS Lambda invoke)
- Bug F: MCP tool output containing private repo contents flows back to attacker; if MCP server is bridged to LangChain agent with shell exec → also RCE
- Outcome: Account takeover on the MCP server + exfil of all data the OAuth scopes covered + potential RCE if agent executes arbitrary code
- Bounty range: low-five-figure to mid-five-figure (Anthropic CVE-2025-49596 MCP Inspector RCE in 38K weekly downloads, classed Critical; JFrog disclosed CVE-2025-6514 mcp-remote RCE affecting 558K downloads; Cloudflare paid for workers-oauth-provider class internally)
- Disclosure source: CVE-2025-4143, CVE-2025-4144, CVE-2025-6514, CVE-2025-49596, GHSA-qgp8-v765-qxx9 (Cloudflare), GHSA-4pc9-x2fx-p7vj (Cloudflare), GHSA-5h2m-4q8j-pqpj (FastMCP), GHSA-2h78-5wx8-jccc (Google Security Research → Cloudflare)

**Hunter's note:** What worked: MCP servers are *brand new attack surface* and most of them are written by developers who've never deployed OAuth before. The Anthropic ecosystem (Claude Desktop, Cursor, Windsurf, claude.ai) connects to thousands of self-hosted MCP servers, and Obsidian Security's research shows the *common* implementation pattern is broken in at least one of: anonymous-cookie session binding, PKCE enforcement, audience validation, or redirect_uri checking. Why this pays: the OAuth bug becomes a tool-use compromise, which in agentic LLM context means private data exfil + cross-tenant pivot. The pivot most hunters miss: the MCP server's OAuth token is often passed *through* to a downstream API (Salesforce, GitHub, Slack), so stealing it gives you access to the underlying SaaS, not just the MCP layer. Audit `WWW-Authenticate: Bearer realm="MCP"` responses for `resource_metadata` URL — the metadata document tells you exactly which downstream APIs the token is valid against.

**Chain 6 — JWT alg confusion → JWKS extraction → Service-to-Service Token Forgery → Cross-Service Privilege Escalation**
- Bug A: Target uses RS256-signed JWTs with public JWKS at `/.well-known/jwks.json`
- Bug B: Backend library is `python-jose` ≤ 3.3.0 (CVE-2025-61152), or Hono < 4.11.4 (CVE-2026-22817), or any library not pinning `algorithms: ["RS256"]`
- Bug C: Extract public key from JWKS, format as PEM
- Bug D: Forge HS256 token signed with the PEM as HMAC secret using `jwt_tool -X k -pk public.pem`
- Bug E: Token's `aud` claim is missing or wildcard → token replays across multiple internal services in same JWT trust circle (CVE-2025-27370 audience injection class)
- Outcome: Forge JWT for any user including admin; replay across all services in the JWT trust circle; cross-service privilege escalation
- Bounty range: $5,000-$25,000 depending on services accessible (H1 #1080786 jwt header merchant_id misconfig $High; H1 #1889161 JWT audience claim not verified Critical; widespread internal-service RCE chains have paid $25K+ on bug bounty programs that cover internal infrastructure)
- Disclosure source: CVE-2026-22817 (Hono CVSS 8.2), CVE-2026-22818 (Hono JWK middleware GHSA-3vhc-576x-3qv4), CVE-2024-37568 (Authlib HMAC bypass milliesolem), CVE-2024-54150 (cjwt), CVE-2025-27370/27371 (OpenID Federation audience injection)

**Hunter's note:** What worked: the alg=none variant is *still* a working bug despite being known since 2015 — python-jose accepts it on `verify_signature: False` configurations (CVE-2025-61152). The case-bypass variants `nOnE`/`NoNE`/`NULL` work against many homegrown JWT validators that string-compare without lowercasing. The RS256→HS256 trick has the highest impact but requires the public key to be in PEM-compatible format (some JWKS endpoints emit raw modulus/exponent only — you need to convert). Why this pays where solo "alg=none accepted" doesn't: chaining to *cross-service* replay shows blast radius. Triagers see "you forged a JWT for service X" and ask "so what?" — if you also demonstrate "and X's token is accepted by service Y because both sit behind the same JWT validator," it becomes a multi-system breach.

**Chain 7 — Race-Condition OAuth Token Mint → Multiple Sessions → Persistence**
- Bug A: Target's `/oauth/token` endpoint check-then-use code redemption (no DB lock)
- Bug B: Submit 50 parallel POST `/oauth/token` requests with same `code`, different `code_verifier` values, via Turbo Intruder single-packet HTTP/2 attack
- Bug C: 2+ requests succeed, returning 2+ valid `access_token` values
- Bug D: Each access_token is for a different effective session; deleting one doesn't kill the others (depending on backend session model)
- Outcome: Persistent access via second token even after victim revokes first; OAuth token mint for arbitrary scopes if scope is also in race window
- Bounty range: $5,000-$10,000 disclosed via HackerOne ($8,500 P1 paid to Anmol Singh Yadav April 2025 on Fortune 500 cloud business-management platform; race-condition severity scales with what the resulting token grants)
- Disclosure source: InfoSec Write-ups Anmol Singh Yadav April 20 2025 disclosed at https://infosecwriteups.com/how-i-hijacked-oauth-tokens-through-a-parallel-auth-flow-race-condition-8500-p1-bug-bounty-7af1cccc4d4c — $8500 P1 OAuth race condition disclosed program writeup, HackerOne corpus race-condition OAuth pattern (multiple disclosed)

**Hunter's note:** What worked: Turbo Intruder's `engine=Engine.BURP2 concurrentConnections=1 requestsPerConnection=50 pipeline=False` configuration with HTTP/2 single-packet (`gate.openGate(150)`) reliably wins token-endpoint races. The endpoints to attack are: `/oauth/token`, `/sso/token`, `/connect/token`, GraphQL `exchangeToken` mutations, and any `/refresh` endpoint that swaps a refresh token for an access token. What failed first: standard race attempts via `requests` Python library never won — needed Turbo Intruder's last-byte-sync. Why this pays: most OAuth race-condition bugs are *invariants violations* (RFC 6749 §4.1.2 says codes are single-use); demonstrating the violation with two different tokens is high-impact because it breaks the security model of OAuth. The triager doesn't have to decide "is this serious?" — RFC text decides for them.

