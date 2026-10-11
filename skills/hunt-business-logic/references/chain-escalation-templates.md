---
name: chain-escalation-templates
description: chain-escalation-templates reference material.
---

# Chain & Escalation Templates

Part of `skills/hunt-business-logic/SKILL.md`. Read this ONLY when you already have ONE confirmed business-logic primitive and want to escalate it into a higher-severity chain before reporting.

---

## Chains & Multi-Bug Templates

Eight chain templates from disclosed reports.

**Chain 1 — `negative-quantity → free order → refund-amount-larger-than-purchase` (low five-figure on e-commerce — AlegroCart / Bagisto pattern combined with refund flow)**
- Bug A: Cart accepts negative quantity (`quantity=-1`) producing negative subtotal — AlegroCart 1.2.9 (Andrey Stoykov SecLists Apr 2025) / Bagisto v2.3.6 (Rudransh Sep 2025) pattern
- Bug B: Checkout flow accepts negative total — order placed with $0 or negative amount
- Bug C: Order processing creates payment record at original product value (not negative cart total)
- Bug D: Request refund — refund amount accepted from client request body, exceeds original payment
- Outcome: Free order plus refund larger than the original purchase = profit on every "purchase"
- Bounty range: low five-figure on e-commerce programs that triage as "financial loss potential"
- Disclosed source: Andrey Stoykov AlegroCart disclosure at https://seclists.org/fulldisclosure/2025/Apr/22; Rudransh Bagisto disclosure at https://medium.com/@rudranshsinghrajpurohit/cve-2025-56426-cart-price-manipulation-vulnerability-in-bagisto-cms-468b72311969; canonical pattern at https://bugbounty.info/Attack-Surface/Web/Business-Logic/Price-Manipulation

**Hunter's note:** the chain that pays here is combining the negative-quantity exploit with the refund flow. Most hunters report the negative quantity alone (mid four-figure), missing that the refund flow on the same target often accepts client-supplied refund_amount. The first attempt I made stopped at "$0 order placed" and triagers downgraded as "no real loss". Adding the refund step — show the app refunding $500 for a $0 order — made it critical-tier. Always test the full money-flow, not just the entry point.

**Chain 2 — `coupon stacking via race → near-zero cart → mass purchase abuse` (mid four-figure to low five-figure on e-commerce — Aditya Bhatt May 2025 pattern)**
- Bug A: Coupon-apply endpoint lacks usage counter increment (Lilishop CVE-2024-50654 / corpus pattern)
- Bug B: Burp Repeater Parallel Execution sends 20 concurrent coupon-apply requests
- Bug C: Server processes all 20, applying discount × 20 — cart value reduced to near-zero
- Bug D: Complete checkout — order placed with $0.05 cart total
- Bug E: Repeat for inventory exhaustion — exclusive product purchased at near-zero × 100 SKUs
- Outcome: Mass-purchase exploitation, can corner exclusive products (concert tickets, limited-edition merch)
- Bounty range: mid four-figure direct + low five-figure on programs where inventory exhaustion is high-impact (ticketing, sneakers, drops)
- Disclosed source: Aditya Bhatt May 2025 InfoSec writeup at https://medium.com/bugbountywriteup/bug-bounty-race-exploiting-race-conditions-for-infinite-discounts-a2cb2f233804 (references Tesla 2020 disclosed and Uber 2016 disclosed); CVE-2024-50654 Lilishop coupon overpurchasing

**Hunter's note:** the trick is the parallel-execution mode in Burp Repeater (right-click tab group → "Send group in parallel last byte sync"). Sequential 20× coupon application fails because each request commits before the next reads; only parallel exposes the race window. Combining with inventory exhaustion makes it a critical: "attacker can purchase entire stock of $1000-product for $5". The first time I tried this, programs paid mid four-figure; adding the mass-inventory-exhaustion angle in the impact section raised it to low five-figure.

**Chain 3 — `wallet TOCTOU → balance creation from nothing → bypass pay-per-view → free subscription` (mid five-figure on financial / fintech — WWBN AVideo CVE-2026-34368 pattern)**
- Bug A: `transferBalance()` reads sender balance, checks sufficiency, writes new balance — no transaction or row lock (WWBN AVideo CVE-2026-34368 / GHSA-h54m-c522-h6qr)
- Bug B: Attacker has $10 sender wallet + own recipient wallet. 20 concurrent transfer requests via threading.
- Bug C: All 20 read sender_balance=10, all pass check (10 >= 10), only 1 deduction effective (last writer wins), recipient credited 20× = $200
- Bug D: Use inflated balance to purchase pay-per-view content / subscription / paid features
- Outcome: Wallet ledger inconsistency, paid content accessed without paying
- Bounty range: mid five-figure when chained to actual paid-feature bypass; low five-figure for the TOCTOU primitive alone
- Disclosed source: GHSA-h54m-c522-h6qr (WWBN AVideo, 2026 critical); CVE-2026-34368 NVD-verified at https://nvd.nist.gov/vuln/detail/CVE-2026-34368; CVEReports analysis at https://cvereports.com/reports/CVE-2026-34368

**Hunter's note:** the secondary vulnerability matters — the AVideo writeup notes captcha tokens can be reused (`$_SESSION['palavra']` not unset after validation). Without that, you'd need fresh captchas for each concurrent request. With it, you generate one captcha and all 20 requests reuse it. Always check companion vulnerabilities when finding TOCTOU; race conditions often work because of secondary defects (no captcha rotation, no rate limit, no idempotency keys).

**Chain 4 — `2FA bypass via password reset → ATO → mass account exfil` (low to mid five-figure — KhaledAhmed107 Jan 2026 pattern combined with horizontal privilege)**
- Bug A: Target supports 2FA; password reset flow has "Skip 2FA" option (KhaledAhmed107 Jan 2026 disclosure at https://systemweakness.com/2fa-bypass-via-reset-password-daba828b10f3)
- Bug B: Trigger password reset for victim's email
- Bug C: Open reset link, click "Skip" → set new password → no 2FA challenge → logged in as victim
- Bug D: Iterate over victim emails harvested from public sources / data breaches
- Outcome: Mass-ATO via 2FA bypass + bulk email harvesting
- Bounty range: low to mid five-figure on programs that triage 2FA bypass as critical
- Disclosed source: KhaledAhmed107 disclosure at https://systemweakness.com/2fa-bypass-via-reset-password-daba828b10f3 (reported via Bugcrowd private program, VRT P3); CVE-2025-3910 Keycloak (GHSA-5jfq-x6xp-7rw2) for the framework-level pattern

**Hunter's note:** the trick is finding programs that explicitly enabled 2FA as their security model — those triage 2FA bypass as critical (because the security guarantee is broken). Programs that have 2FA as optional may downgrade to medium ("user can choose to set 2FA, this just bypasses optional feature"). Always frame the impact as "the 2FA security model is broken", not "I bypassed an optional feature". The Samsung Account 2FA bypass (Gregory Greekas 2024) was framed as "fundamentally undermined the security model that 2FA is supposed to provide" — explicit framing earned high-severity classification.

**Chain 5 — `email-alias trial abuse → unlimited free tier → resource exhaustion` (mid four-figure on SaaS programs that pay this class — Mahmoud Magdy Dec 2025 pattern)**
- Bug A: Target accepts `user+a1@gmail.com`, `user+a2@gmail.com` as distinct registrations; doesn't canonicalize email
- Bug B: Script registers 1000 trial accounts with `user+trial<i>@gmail.com` aliases
- Bug C: Each trial provides 14 days of premium features → 14000 cumulative trial-days from 1 mailbox
- Bug D: Use trial accounts at scale for resource consumption (compute, storage, API calls)
- Outcome: Resource exhaustion / revenue loss / mass-account-creation impact
- Bounty range: mid four-figure on programs that pay this class (many don't — check scope first); referenced Cremit Apr 2026 analysis at https://www.cremit.io/blog/out-of-scope-loophole-credential-exposure for the broader "out-of-scope" pattern
- Disclosed source: Mahmoud Magdy Dec 2025 at https://medium.com/@mahmoudmagdy45456/violation-of-secure-design-principles-unlimited-free-trial-abuse-via-email-aliases-3de0756eb58c; Doppler trial-reset disclosure (Aditya Sunny Dec 2024) at https://adityasunny06.medium.com/how-i-identified-a-revenue-loss-bug-in-dopplers-free-trial-system-b88919aa161f

**Hunter's note:** the move that pays this is showing scale, not just the alias trick. Most hunters report "I created 5 accounts with aliases" and triagers shrug — every SaaS knows about `+aliases`. Showing 1000-account scripted abuse with measurable resource cost (compute hours, API calls, storage) elevates to actual impact. Also test other canonicalization gaps: dot variants (`u.s.er@gmail.com`), `@googlemail.com` vs `@gmail.com`, Unicode-confusable domains (`gmail.com` vs `gmail.с0m`).

**Chain 6 — `pre-account takeover via SSO migration → persistent ATO across victim signup` (mid five-figure on SSO-enabled enterprise SaaS — Giongnef Jan 2024 pattern)**
- Bug A: Target supports both direct-login AND SSO/OAuth; `sso_type:null` defaults to direct-login flow
- Bug B: Attacker pre-registers `victim@victim-company.com` via direct-login signup with attacker-chosen password, before victim signs up
- Bug C: Attacker logs in, uses "Migrate" function to transfer the account to SSO database
- Bug D: Victim eventually signs up via Google SSO with `victim@victim-company.com` — succeeds because account already exists in SSO DB; from victim's perspective everything works normally
- Bug E: Attacker still has direct-login access via the password set in step B; the "Migrate" preserved the password
- Outcome: Persistent ATO that survives victim's "secure" SSO signup; attacker reads/writes alongside victim
- Bounty range: mid five-figure on enterprise SaaS where SSO is the primary security model
- Disclosed source: Giongnef Jan 2024 at https://giongfnef.medium.com/business-logic-bypass-2fa-to-ato-e0dc7131b10e

**Hunter's note:** the prerequisite is finding SaaS that supports both auth modes simultaneously. Hunt: any "Login with SSO" + "Sign up with email" on the same product. The pre-registration window must be large enough — works best for B2B SaaS where companies onboard in waves. The "Migrate" function is the key step; without it, the direct-login account is bypassed when SSO records take precedence. Test by reading the SSO documentation for the target — many implementations describe the Migrate flow explicitly.

**Chain 7 — `phone-change OTP flaw → phone takeover → password reset via SMS → ATO` (low to mid five-figure on programs with SMS-based recovery — H1 2024 critical pattern)**
- Bug A: Change-phone API sends OTP only to NEW phone number (not also requiring OTP from OLD number)
- Bug B: Attacker obtains victim's user-id (via enumeration / public data / IDOR — cross-reference hunt-idor)
- Bug C: Attacker submits change-phone request with attacker-controlled new number
- Bug D: Server sends OTP to attacker's phone; attacker enters OTP — victim's account now has attacker phone
- Bug E: Trigger password reset via SMS — OTP goes to attacker phone — set new password — victim ATO
- Outcome: Account takeover via SMS-recovery flow exploitation
- Bounty range: low to mid five-figure on programs where SMS recovery is the primary backup
- Disclosed source: HackerOne 2024 disclosed pattern "Change phone number OTP flaw leads to any phone number takeover" (corpus-extracted); pattern documented in OWASP Authentication Cheat Sheet and across multiple H1 hacktivity disclosed 2023-2024

**Hunter's note:** the missing step is OTP from the OLD phone. Modern flows require dual-OTP: confirm from current number AND new number. Older flows (and many SaaS that have weaker SMS recovery) only confirm new number. Always test by requesting a phone change while logged in as your test account; if the OTP only goes to the new number, the account is one-step from ATO. Combine with username enumeration (cross-reference hunt-info-disclosure) for mass exploitation.

**Chain 8 — `WAF bypass + XSS + business-logic email collision → ATO` (low to mid five-figure — Ali Hussain Sep 2025 chain)**
- Bug A: Business-logic flaw — email collision in password reset functionality (initially low-impact alone)
- Bug B: Reflected XSS via comment injection — URL path reflected in HTML comments
- Bug C: WAF bypass technique — character encoding / alternative syntax to evade filtering (e.g., adding `+` at start of payload disabled WAF filtering on this target)
- Bug D: XSS executes JS in victim's browser with authenticated session
- Bug E: JS makes API call to change victim's profile email to attacker-controlled
- Bug F: Email change succeeds (no verification on email change in this target)
- Bug G: Attacker triggers password reset for the new email — gets reset token
- Outcome: 1-click XSS-triggered email collision → ATO via password reset to attacker's email
- Bounty range: low to mid five-figure on SaaS programs (this report was a pen-test, not bug bounty, but pattern documented at https://medium.com/@ghostxploiter/waf-bypass-xss-business-logic-flaw-account-takeover-04577cb53b18)
- Disclosed source: Ali Hussain Sep 2025 writeup at https://medium.com/@ghostxploiter/waf-bypass-xss-business-logic-flaw-account-takeover-04577cb53b18

**Hunter's note:** the chain shows that low-impact business-logic flaws (email collision in password reset → reset email goes to attacker, but only if attacker can change victim's email) become critical when chained with XSS that auto-triggers the email change. The first attempt I tried reported the email-collision bug as low; programs closed it because "attacker can't change victim's email". Adding the XSS-as-delivery layer turned it into ATO. Always look for delivery vectors (XSS, CSRF, postMessage, OAuth open-redirect) when business-logic flaws require victim action.

