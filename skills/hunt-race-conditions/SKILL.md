---
name: hunt-race-conditions
description: Hunting skill for race conditions and TOCTOU — concurrent request bursts, single-packet attacks, idempotency-key replay, and state-changing endpoint races. Use when a feature updates a limited resource, applies a discount, or transitions state in multiple steps.
---

# Race Condition Hunting Methodology

Race conditions occur when two or more requests are processed concurrently and the outcome depends on their interleaving. They often lead to double-spend, limit bypass, privilege escalation, or inconsistent state.

## When to invoke

- State-changing endpoints: `POST /purchase`, `POST /redeem`, `POST /transfer`, `POST /withdraw`, `POST /apply-coupon`, `POST /cancel`, `POST /refund`.
- Limited resources: inventory, coupons, gift cards, credits, quota, tickets.
- Multi-step flows where a check and an update are separate operations.
- Features with optimistic locking, idempotency keys, or balance checks.
- Authentication/session endpoints where token creation or role assignment may race.

## Crown jewel targets

1. **Payment/coupon flows** — double redemption, over-refund, negative balance.
2. **Inventory/cart** — overselling, negative stock.
3. **Quota/rate-limit** — exceeding allowed limits.
4. **Account state** — double email verification, duplicate account creation, role escalation.
5. **Crypto/fintech** — double-spend, withdrawal race, deposit credit race.

## Detection strategies

### 1. Parallel identical requests

Send N identical requests simultaneously to a state-changing endpoint:

```bash
# 10 concurrent identical requests
for i in {1..10}; do
  curl -s -X POST https://target.com/api/redeem -H "Authorization: Bearer $TOKEN" -d '{"code":"DISCOUNT50"}' &
done
wait
```

Look for:
- Multiple successful responses when only one should succeed.
- Balance/credit/quota decreasing by more than expected.
- Inventory count going negative.

### 2. Single-packet attack

For HTTP/2 or HTTP/1.1 keep-alive, send multiple requests in one TCP packet or one TLS record to minimize jitter:

```bash
# Conceptual: use a tool that can send multiple requests in one packet
# (turbo-intruder, racepwn, or a custom script)
```

If the backend processes the requests in parallel before any locks are acquired, the race succeeds.

### 3. Last-byte sync

For HTTP/1.1, send request headers first, then send bodies of multiple requests at the same time. Some reverse proxies batch bodies and forward them together.

### 4. Idempotency-key replay

If the API uses `Idempotency-Key`:

1. Send a request with `Idempotency-Key: K1`.
2. Immediately send a second request with the same key but a slightly different body.
3. If the server processes both because the key lookup races with the operation, you may get double execution.

Also test:
- Reusing an idempotency key after the original window expires.
- Sending the same key from different sessions/users.

### 5. State-transition race

Find flows with separate check and act:

```
1. GET  /balance     -> $100
2. POST /withdraw/50 -> pending
3. POST /withdraw/50 -> should fail, but may race with #2
```

Concurrent withdrawals checked against the same balance can both succeed.

### 6. Race with cancellation/refund

```
1. POST /purchase   -> success
2. POST /refund     -> success
3. Race #1 and #2: can you get both the item and the refund?
```

Or:

```
1. POST /apply-coupon
2. POST /remove-coupon
# Race: coupon applied but balance not recalculated correctly
```

## Tooling

If a race-runner tool is available, use it. Otherwise use shell background jobs or Python `asyncio`/`threading` with precise timing.

Always compare against a sequential baseline: run the same N requests one at a time. If the sequential run is correct but the concurrent run is not, you have a race condition.

## Depth floor

For each state-changing endpoint, try at least **25 distinct race variants**:
- Different concurrency levels (2, 5, 10, 50).
- Different payloads (identical vs slightly different).
- Different timing (simultaneous vs staggered 1ms/10ms).
- Different auth contexts (same user vs different users).

Record negative results as `record_fact`.

## Evidence gate

Before `report_finding`:
- Show the concurrent requests and the sequential baseline.
- Show the side effect: balance, inventory, coupon count, or state log proving the race succeeded.
- Include timestamps to prove simultaneity.
- If possible, reproduce a second time with a clean account/state.
- Tag `vulnclass: race condition` and bind `traffic_refs`.

## Safety

Race testing can cause financial loss or inventory corruption. Only run on:
- Test accounts with fake balances.
- Staging environments.
- Endpoints explicitly in scope for destructive testing.

If unsure, record an `inferred` fact and ask for approval before exploitation.

## Stop-authority

This skill decides *what race patterns to try* and *how to verify*. It never decides when the race-condition class is exhausted — that stays with `hunting-methodology`.
