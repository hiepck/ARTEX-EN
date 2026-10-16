---
name: hunt-graphql-authz
description: Hunting skill for GraphQL authorization flaws — introspection recovery, alias batching, fragment abuse, nested resolver IDOR, field-level authorization gaps, and mutation access control. Use when the target exposes `/graphql`, `/graphql/v1`, `/api/graphql`, or a GraphQL Playground endpoint.
---

# GraphQL Authorization Hunting Methodology

GraphQL centralizes many operations behind one endpoint. Authorization mistakes often appear at the resolver level, not the HTTP route level, making them invisible to route-based scanners.

## Deeper reference (load only if this specific situation applies)

- **`references/clairvoyance.md`** — introspection is disabled but you still need the schema to hunt field-level authz. Covers the field-suggestion oracle and the Clairvoyance recovery loop.

## When to invoke

- Endpoint path contains `/graphql`, `/gql`, `/api/graphql`, `/graphql/v1`.
- Response contains `"data"` and `"errors"` JSON keys.
- `Content-Type: application/graphql` is accepted.
- The target uses Apollo, Relay, Strawberry, Graphene, Hot Chocolate, graphql-java, or similar.

## Recon

1. Check introspection:
   ```graphql
   { __schema { types { name fields { name args { name } } } } }
   ```
   If disabled, try `references/clairvoyance.md` (introspection recovery) or guess common schema patterns.

2. Look for GraphiQL/Playground on GET `/graphql` or `/graphql/playground`.

3. Check for schema suggestions / did-you-mean leaks in error messages.

## Query-level authz tests

### 1. Read forbidden objects by ID

If `user(id: 1)` returns your own profile, try:

```graphql
{ user(id: 2) { id email role } }
{ users { id email role } }
{ me { friends { id email } } }
```

If IDs are predictable (auto-increment, UUIDv1), iterate.

### 2. Alias batching to bypass rate limits

Aliases let you send many queries in one request:

```graphql
{
  u1: user(id: 1) { email }
  u2: user(id: 2) { email }
  u3: user(id: 3) { email }
  # ... 100 aliases
}
```

If the backend checks rate limits per HTTP request but not per alias, you can enumerate at scale.

### 3. Fragment abuse

Reuse fragments to access fields the frontend never asks for:

```graphql
fragment All on User { id email passwordResetToken role ssn }
{ me { ...All } }
```

If the resolver authorizes per-query-shape based on frontend fragments, a manually crafted fragment may bypass it.

### 4. Nested resolver pivot

```graphql
{
  project(id: 1) {
    owner { id email password }
    members { id email role }
    billing { cardLastFour }
  }
}
```

Authorization may be checked on `project` but not on nested `owner`, `members`, or `billing` resolvers.

### 5. Introspection-powered field discovery

Use introspection to find sensitive fields (`isAdmin`, `internalNotes`, `deletedAt`, `passwordHash`, `apiKey`), then query them through aliases or fragments.

## Mutation-level authz tests

### 1. Mutate objects you do not own

```graphql
mutation {
  updateUser(id: 2, input: { role: "admin" }) { id role }
}
```

### 2. Delete/restore operations

```graphql
mutation {
  deleteUser(id: 2) { id }
  restoreProject(id: 5) { id }
}
```

### 3. Admin mutations without admin role

Use introspection to find mutations prefixed `admin`, `internal`, `staff`, `super`. Try them with a normal user token.

### 4. Bypass input validation via arrays

```graphql
mutation {
  updateUser(id: 1, input: [{ role: "admin" }, { role: "user" }]) { id role }
}
```

Some resolvers validate the first array element but apply the last.

## Type confusion and suggestions

1. Send a valid query with an unexpected type:
   ```graphql
   { user(id: "1' OR '1'='1") { id } }
   ```
2. Look for SQL/NoSQL injection in resolver argument handling.
3. Check error messages for internal field names or stack traces.

## Batching / query cost bypass

If the API limits query cost or depth:

- Split one deep query into multiple aliases.
- Use circular fragments with `directive @skip`/`@include`.
- Send batched JSON array: `[{query: "..."}, {query: "..."}]`.

## Depth floor

For each GraphQL endpoint, try at least **25 distinct query/mutation shapes** covering introspection, alias batching, fragments, nested pivots, and admin mutations. Record negative results as `record_fact`.

## Evidence gate

Before `report_finding`:
- Include the exact GraphQL query and the full response.
- Show the unauthorized data or state change.
- For IDOR, include proof of accessing another user's/tenant's data.
- Tag `vulnclass: GraphQL authorization` or `GraphQL IDOR` and bind `traffic_refs`.

## Stop-authority

This skill decides *what GraphQL authz vectors to try*. It never decides when the GraphQL class is exhausted — that stays with `hunting-methodology`.
