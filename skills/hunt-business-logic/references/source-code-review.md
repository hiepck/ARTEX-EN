---
name: source-code-review
description: source-code-review reference material.
---

# Business Logic — Source Code Review Patterns

> **Part of `skills/hunt-business-logic/SKILL.md`.** Read this ONLY when you have
> source/repo access (OSS bug, internal pentest, in-scope GitHub org). Not useful
> for pure black-box hunting.

---

## Source Code Review Patterns

### Semgrep rules

```yaml
rules:
  - id: bizlogic-trust-client-price
    pattern-either:
      - pattern: |
          $TOTAL = $REQ.body.total
      - pattern: |
          $TOTAL = $REQ.body.price
      - pattern: |
          $ORDER.total = $REQ.body.amount
    message: |
      Order total / price computed from client request body. Negative-quantity
      and price-manipulation attacks (AlegroCart 1.2.9 SecLists Apr 2025,
      Bagisto v2.3.6) bypass this. Recalculate server-side from cart items
      + tax + shipping + discount; never trust client-supplied totals.
    severity: ERROR
    languages: [javascript, typescript, python, php, java]
```

```yaml
rules:
  - id: bizlogic-no-quantity-validation
    pattern-either:
      - pattern: |
          quantity * price
      - pattern: |
          $QTY * $PRICE
    pattern-not-inside: |
      if ($QTY > 0) {
        ...
      }
    message: |
      Multiplication of quantity × price without sign / range validation.
      Negative quantity produces negative total — app may issue refund.
      Validate: assert quantity > 0 and quantity < MAX_REASONABLE_QTY and
      Number.isInteger(quantity).
    severity: ERROR
    languages: [javascript, typescript]
```

```yaml
rules:
  - id: bizlogic-toctou-balance-check
    pattern: |
      $BALANCE = $WALLET.getBalance()
      ...
      if ($BALANCE >= $AMOUNT) {
        ...
        $WALLET.deduct($AMOUNT)
      }
    message: |
      Read-check-write on wallet balance without database transaction or
      row-level locking. WWBN AVideo CVE-2026-34368 (GHSA-h54m-c522-h6qr)
      pattern: concurrent transfers all pass check, only one deduction
      effective. Wrap in BEGIN/COMMIT with SELECT ... FOR UPDATE on wallet
      row, OR use atomic UPDATE with WHERE balance >= amount.
    severity: ERROR
    languages: [php, python, javascript, typescript, java]
```

```yaml
rules:
  - id: bizlogic-coupon-no-usage-counter
    pattern: |
      $COUPON = $DB.find_coupon($CODE)
      if ($COUPON.valid) {
        $CART.apply_discount($COUPON.amount)
      }
    pattern-not-inside: |
      $COUPON.usage_count++
      $DB.update_coupon($COUPON)
    message: |
      Coupon application without usage-counter increment. Lilishop
      CVE-2024-50654 (CVSS 7.5) and Aditya Bhatt May 2025 InfoSec writeup
      pattern: stack same coupon N times via concurrent requests. Add
      atomic UPDATE coupons SET usage_count = usage_count + 1
      WHERE code = $CODE AND usage_count < max_usage RETURNING *.
    severity: ERROR
    languages: [python, javascript, ruby, java]
```

```yaml
rules:
  - id: bizlogic-2fa-skip-path
    pattern-either:
      - pattern-regex: 'skip[-_]?2fa|bypass[-_]?(?:2fa|mfa|otp)'
      - pattern: |
          if ($CONTEXT == "password_reset") {
            // skip 2FA
          }
    message: |
      2FA skip / bypass path detected. KhaledAhmed107 Jan 2026 disclosure:
      password reset flow with "Skip" 2FA option enables full ATO.
      Keycloak CVE-2025-3910 / GHSA-5jfq-x6xp-7rw2: org.keycloak.authorization
      package circumvents required actions including 2FA. Audit every flow
      that can authenticate a user — password reset, OAuth, recovery,
      mobile app login, API tokens — to enforce 2FA consistently.
    severity: ERROR
    languages: [python, javascript, typescript, java]
```

```yaml
rules:
  - id: bizlogic-trial-state-no-history-check
    pattern-either:
      - pattern: |
          $SUB = $DB.subscription.findOne({user_id: $UID})
          if (!$SUB.has_trialed) { grant_trial() }
      - pattern: |
          $SUB = $DB.subscriptions.first(user=$UID)
          if not $SUB.has_trialed:
              grant_trial()
    message: |
      Trial-history check uses findOne / .first which returns whichever
      record DB orders first. better-auth #6863 (Dec 2025) Stripe pattern:
      user with multiple subscriptions (one canceled with trial history,
      one new incomplete) bypasses check. Use findMany + .some() to check
      ALL subscriptions for trial history.
    severity: ERROR
    languages: [javascript, typescript, python]
```

```yaml
rules:
  - id: bizlogic-email-no-canonicalization
    pattern: |
      $USER.email = $REQ.body.email
    pattern-not-inside: |
      $REQ.body.email = canonicalize_email($REQ.body.email)
    message: |
      Email stored without canonicalization. Mahmoud Magdy Dec 2025
      pattern: user+a1@gmail.com, user+a2@gmail.com, user.dot@gmail.com,
      user@googlemail.com all deliver to same inbox but treated as distinct
      registrations enabling unlimited free-trial abuse. Strip +alias tags,
      strip dots in local-part for Gmail, normalize @googlemail.com to
      @gmail.com, lowercase entire email, enforce uniqueness on canonical form.
    severity: WARNING
    languages: [python, javascript, typescript, java, php, ruby]
```

```yaml
rules:
  - id: bizlogic-phone-change-no-old-verify
    pattern: |
      def change_phone($USER, $NEW_PHONE):
        $OTP = generate_otp()
        send_sms($NEW_PHONE, $OTP)
    pattern-not-inside: |
      send_sms($USER.current_phone, $OLD_OTP)
      verify_otp($USER, $OLD_OTP)
    message: |
      Phone-change flow sends OTP only to NEW number, not also requiring
      verification from CURRENT number. H1 2024 critical: any phone takeover
      via this exact pattern. Always require OTP from BOTH old and new
      numbers (or use signed action token from authenticated session).
    severity: ERROR
    languages: [python, javascript, typescript, ruby, php, java]
```

### ast-grep patterns

```bash
# Client-supplied price/total (price manipulation surface)
ast-grep --pattern '$TOTAL = req.body.total' --lang js
ast-grep --pattern 'order.total = $REQ.body.amount' --lang js

# Multiplication without validation
ast-grep --pattern '$QTY * $PRICE' --lang js
ast-grep --pattern '$QTY * $PRICE' --lang python

# Read-check-write without transaction
ast-grep --pattern 'getBalance(); checkBalance(); deduct()' --lang js
ast-grep --pattern '$.balance -= $AMOUNT' --lang js

# Coupon-apply without atomic increment
ast-grep --pattern 'coupon.apply($CODE)' --lang js
ast-grep --pattern '$CART.apply_discount($COUPON.amount)' --lang js

# 2FA skip
ast-grep --pattern 'if ($CTX == "password_reset") return true' --lang js

# findOne for subscription (Stripe pattern)
ast-grep --pattern '$DB.subscription.findOne($X)' --lang js
ast-grep --pattern 'Subscription.objects.get(user=$U)' --lang python
```

### ripgrep one-liners

```bash
# Client-trust patterns (price/quantity/total from request)
rg -n -e 'req\.body\.(total|price|amount|quantity|tax|discount)' \
   -e 'request\.body\.(total|price|amount)' \
   --type js --type ts --type py --type java

# Negative-value validation missing
rg -n -B 2 -A 5 'quantity\s*\*\s*price|price\s*\*\s*quantity' \
   --type js --type ts --type py | rg -v '> 0|>= 0|isInteger|valid'

# TOCTOU patterns — read then write without lock
rg -n -B 5 -A 15 'getBalance\(\)|wallet\.balance' \
   --type js --type ts --type py --type php --type java | \
   rg -B 8 -A 8 'updateBalance|setBalance|wallet\.update' | \
   rg -v 'BEGIN|FOR UPDATE|lock|mutex|atomic|SERIALIZABLE'

# 2FA skip / bypass paths
rg -n -i -e 'skip.*2fa' -e 'skip.*mfa' -e 'bypass.*otp' \
   -e 'password.*reset.*skip' \
   --type js --type ts --type py --type java

# Email canonicalization missing
rg -n -e 'user\.email\s*=\s*req\.body\.email' \
   --type js --type ts --type py | rg -v 'normalize|canonical|strip|lower'

# Subscription / tier mass-assignment
rg -n '\.\.\.req\.body|\.\.\.body|spread.*body' \
   --type js --type ts | rg -B 2 -A 2 'subscription|tier|role|plan'

# Coupon usage counter missing
rg -n -B 3 -A 10 'coupon\.apply|apply.*discount' \
   --type js --type ts --type py | rg -v 'usage_count|usageCount|increment'

# Phone change endpoint
rg -n -B 2 -A 15 'change.*phone|update.*phone' \
   --type js --type ts --type py | rg -B 5 -A 5 'send.*sms|sendOtp'
```

### CodeQL hint

CodeQL has limited built-in business-logic queries because most business-logic flaws are application-specific. The most relevant standard queries:

- `js/race-condition` — flags read-check-write patterns on shared state without synchronization. Catches the WWBN AVideo CVE-2026-34368 class.
- `js/missing-rate-limiting` — flags endpoints without rate-limit middleware. Catches MinIO LDAP brute-force GHSA-jv87-32hw-hh99 class (cross-reference hunt-info-disclosure).
- `js/tainted-arithmetic-operands` — flags arithmetic on user-controlled values without sanitization. Catches negative-quantity manipulation.

For business-logic specifically, write custom CodeQL predicates targeting the exact pattern. Example sketch for "client-supplied total trusted by checkout":

```ql
import javascript
import semmle.javascript.security.dataflow.flow

class TrustedClientTotal extends TaintTracking::Configuration {
  TrustedClientTotal() { this = "TrustedClientTotal" }
  override predicate isSource(DataFlow::Node src) {
    src.asExpr() = any(HTTP::RequestInputAccess in)
  }
  override predicate isSink(DataFlow::Node sink) {
    exists(MethodCallExpr c |
      c.getMethodName() = ["createOrder", "processPayment", "chargeCustomer"]
      and c.getAnArgument() = sink.asExpr()
    )
  }
}
```

