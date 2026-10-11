---
name: case-study-writeups
description: case-study-writeups reference material.
---

# Real Impact Case-Study Write-Ups

Part of `skills/hunt-business-logic/SKILL.md`. Read this ONLY when you want full narrative write-ups of real disclosed findings for report-framing inspiration (not needed to find the first bug).

---

## Real Impact Examples

**Example 1 — `wwbn-avideo-wallet-toctou-balance-creation-from-nothing` (low five-figure bounty range, 1-link TOCTOU primitive — CVE-2026-34368 NVD-verified, GHSA-h54m-c522-h6qr)**
- Setup: WWBN AVideo video-CMS platform with YPTWallet plugin enabled. Wallet supports user-to-user transfers via `transferBalance()` method in `plugin/YPTWallet/YPTWallet.php`. Authenticated users can transfer funds to other users. Captcha required per transfer (`$_SESSION['palavra']` validation).
- Discovery: Researcher analyzed wallet plugin source code on GitHub. Identified read-check-write pattern in `transferBalance()`: SELECT current balance → PHP-side check sufficiency → UPDATE deducted balance. No `BEGIN TRANSACTION`, no `SELECT ... FOR UPDATE`, no row-level locking. Secondary defect: captcha token not unset after validation, enabling reuse within same session.
- Exploitation: Attacker logged in with two PHP sessions (two PHPSESSID cookies for same user account). Generated one captcha, captured the validated token. Sent 20 concurrent transfer requests via Python threading with same captcha token. All 20 requests entered `transferBalance()` in parallel; all read sender_balance=$10, all passed check (10 >= 10 transfer amount), only 1 deduction effective (last writer wins for sender), recipient credited 20× = $200.
- Impact: Wallet balance creation from nothing — $10 starting balance produces $200 in recipient wallet. Inflated balance bypasses pay-per-view charges, subscription fees, paid-feature gates. Financial integrity compromised — total balances across all users no longer match total deposits.
- Disclosed source: GHSA-h54m-c522-h6qr (WWBN/AVideo, 2026 critical) at https://github.com/WWBN/AVideo/security/advisories/GHSA-h54m-c522-h6qr; CVE-2026-34368 (NVD-verified, CVSS 5.3 medium per NVD though impact-class is critical) at https://nvd.nist.gov/vuln/detail/CVE-2026-34368; CVEReports analysis at https://cvereports.com/reports/CVE-2026-34368. Affects WWBN AVideo through 26.0; fix in commit `34132ad5159784bfc7ba0d7634bb5c79b769202d`.

**Example 2 — `lilishop-coupon-overpurchase-concurrent-collection` (low five-figure bounty range on programs that triage as financial-fraud-class — CVE-2024-50654 NVD-verified, CVSS 7.5)**
- Setup: Lilishop e-commerce platform v4.2.4 and earlier. Coupon collection feature with per-user / per-coupon quantity limit enforced server-side.
- Discovery: Researcher Yllxx03 identified Incorrect Access Control in coupon collection logic. Limit check happens before atomic decrement of available_count.
- Exploitation: Captured coupon-collection HTTP request via proxy. Replayed in high concurrency (Burp Suite Repeater Parallel Execution / custom script). All concurrent requests passed the limit check before any committed the decrement; attacker collected coupons beyond the enforced quantity limit.
- Impact: Attackers obtain unlimited coupons per user (intended limit: 1 or 5 per user; attacker collects N). Direct e-commerce financial loss when these coupons are redeemed at checkout.
- Disclosed source: CVE-2024-50654 (NVD-verified CVSS 7.5 HIGH, published Nov 15 2024) at https://cvefeed.io/vuln/detail/CVE-2024-50654; original analysis at https://github.com/Yllxx03/CVE/blob/main/lilishop/CouponLogicVulnerability.md; affects Pickmall lilishop through version 4.2.4.

**Example 3 — `samsung-account-2fa-bypass-via-imei-leak` (low five-figure bounty range from Samsung Security Bounty program, 2-link chain — Gregory Greekas Dec 2024 disclosure)**
- Setup: Samsung Account global 2FA system. 2FA request API endpoint accessible without authentication. Trusted-device verification uses `deviceUniqueId` field derived deterministically from device IMEI through known transformation. Username (publicly-discoverable) → 2FA request API → exposes IMEI.
- Discovery: Gregory Greekas during account-security review identified the 2FA-request endpoint disclosed sensitive device info (IMEIs, phone numbers) to anyone with a victim's username. No authentication required for the leak. Reverse-engineered the deviceUniqueId derivation to confirm IMEI → deviceUniqueId is deterministic.
- Exploitation: Step 1: query 2FA-request API with victim's username → response contains IMEI. Step 2: compute expected deviceUniqueId from IMEI via known transformation. Step 3: include computed deviceUniqueId in authentication request → authenticate as if using victim's trusted device → 2FA bypassed entirely. Final result: full session tokens (AccessToken, UserAuthToken, RefreshToken) for victim account, with only the victim's password required.
- Impact: 2FA fundamentally undermined — security model meant to protect when password is compromised was nullified. Globally affected Samsung Account and Samsung Cloud. Scalable to any account count with just usernames.
- Disclosed source: Gregory Greekas blog post at https://www.hackingadventures.ca/posts/samsung-2fa-bypass; Samsung paid cash bounty (specific amount not publicly disclosed; high-severity tier). Samsung patched December 2024. No CVE assigned (typical for auth-bypass on consumer cloud services).

**Example 4 — `doppler-free-trial-reset-via-plan-tier-roundtrip` (mid four-figure bounty range on SaaS programs that pay this class — Aditya Sunny Dec 2024 disclosure)**
- Setup: Doppler secrets-management platform with 14-day Team Mode (premium) free trial. Free Developer Mode tier exists for indefinite use of basic features. Subscription tier-switching is a single API call.
- Discovery: Aditya Sunny tested trial-cancellation flow expecting trial-state to persist as "consumed". Tested switching to Developer Mode (free tier) after trial cancellation. Then tested switching BACK to Team Mode — found trial regranted.
- Exploitation: Sign up for new account → activate 14-day Team Mode trial → cancel before trial expiry → switch to free Developer Mode → use developer tools / specific workflow to revert to Team Mode → trial restored without payment validation. Repeat indefinitely.
- Impact: Direct revenue loss for Doppler — users can use premium features perpetually without payment. Brand impact on paying customers who discover non-paying users have same access.
- Disclosed source: Aditya Sunny disclosure at https://adityasunny06.medium.com/how-i-identified-a-revenue-loss-bug-in-dopplers-free-trial-system-b88919aa161f (responsible disclosure to Doppler Nov 14 2024). Specific bounty amount not disclosed; pattern is mid four-figure on SaaS programs that triage trial-abuse as paying class. Many SaaS programs explicitly exclude this class — check scope per Cremit Apr 2026 analysis at https://www.cremit.io/blog/out-of-scope-loophole-credential-exposure.

**Example 5 — `alegrocart-negative-quantity-price-manipulation` (mid four-figure bounty range on equivalent commercial e-commerce — Andrey Stoykov Apr 2025 disclosure)**
- Setup: AlegroCart 1.2.9 e-commerce platform. Add-to-cart endpoint accepts quantity parameter from URL. Cart subtotal computed as sum of `quantity × unit_price` per line item.
- Discovery: Andrey Stoykov tested add-to-cart with negative quantity values via URL manipulation: `GET /alegrocart/index.php?controller=addtocart&action=add&item=10&quantity=-100 HTTP/1.1`. Server processed the negative quantity literally; cart line item showed `-100 x Featured-product $-1,599.00`.
- Exploitation: Add legitimate $50 product (positive quantity) to cart. Add high-value product with `quantity=-100` to cart → subtotal becomes negative. Proceed to checkout → checkout flow accepts negative total. Some payment integrations process this as a refund; others process as a $0 free order.
- Impact: Free orders or refund-without-purchase exploitation of e-commerce sites running AlegroCart. Equivalent pattern affects Bagisto v2.3.6 (Rudransh Singh Rajpurohit Sep 2025 disclosed) and many other CMS-driven e-commerce platforms.
- Disclosed source: Andrey Stoykov SecLists Full Disclosure post at https://seclists.org/fulldisclosure/2025/Apr/22 (April 23 2025); related Bagisto disclosure by Rudransh Singh Rajpurohit at https://medium.com/@rudranshsinghrajpurohit/cve-2025-56426-cart-price-manipulation-vulnerability-in-bagisto-cms-468b72311969; canonical pattern documentation at https://bugbounty.info/Attack-Surface/Web/Business-Logic/Price-Manipulation. Bounty range mid four-figure on equivalent commercial e-commerce (the original AlegroCart disclosure was OSS, no bounty).

**Example 6 — `better-auth-stripe-hasevertried-incomplete-subscription-reset` (mid four-figure bounty range on SaaS programs that pay trial-abuse — better-auth issue #6863 Dec 2025)**
- Setup: SaaS application uses better-auth's Stripe plugin for subscription state. Business rule: one free trial per billing identity. Plugin stores subscription records and computes `hasEverTrialed` from the first subscription object returned by database lookup.
- Discovery: Issue #6863 showed `findOne` can return a newly-created incomplete subscription before an older used-trial subscription. The app asks "has this user ever trialed?" and receives false because it inspected the wrong row.
- Exploitation: User starts a trial, cancels, then triggers a new checkout session that creates an incomplete subscription row. The next trial-eligibility check reads the incomplete row first, treats the user as trial-naive, and grants another free trial. Repeating the flow yields indefinite paid-tier access without payment.
- Impact: Direct SaaS revenue loss and fairness break for paying customers. In a team product, one attacker can repeatedly mint trial workspaces and invite users who should require paid seats.
- Disclosed source: better-auth GitHub issue #6863 (Dec 2025) at https://github.com/better-auth/better-auth/issues/6863; Stripe subscription-state integration pattern. Bounty range mid four-figure on equivalent commercial SaaS when the program pays subscription-abuse findings; trial-abuse exclusions are common, so scope acceptance is mandatory.

