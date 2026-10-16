---
name: clairvoyance
description: GraphQL introspection recovery when introspection is disabled — field/type name reconstruction via error suggestions and wordlists.
---

# GraphQL — Introspection Recovery (Clairvoyance)

> **Part of `skills/hunt-graphql-authz/SKILL.md`.** Read this ONLY when introspection is disabled but you need the schema to hunt field-level authz. If introspection works, skip this file.

---

When a server disables `__schema` introspection but keeps
**field-suggestion errors** (the "Did you mean …" hints) enabled, you can
reconstruct the schema field-by-field. This is the technique the
[Clairvoyance](https://github.com/nikitastupin/clairvoyance) tool automates.

## The oracle

Send a query naming a field that does not exist:

```graphql
{ user { id emailAdress } }
```

A server with suggestions responds:

```json
{ "errors": [ { "message": "Cannot query field \"emailAdress\" on type \"User\". Did you mean \"emailAddress\"?" } ] }
```

Every suggestion is a real field name. Repeat per type to map the schema.

## Manual recovery loop

1. **Seed types** — guess common root fields (`user`, `users`, `me`,
   `account`, `project`, `organization`, `admin`) and collect the type names
   from the did-you-mean and error text.
2. **Enumerate fields** — for each discovered type, submit a query with a
   near-miss field name and harvest suggestions. Feed a wordlist of common
   field names (`email`, `password`, `role`, `isAdmin`, `token`, `ssn`,
   `billing`, `internalNotes`, `deletedAt`) and keep the prefixes that
   produce suggestions.
3. **Enumerate arguments** — name an argument near-miss to learn parameter
   names (`id`, `userId`, `where`, `filter`, `input`).
4. **Enumerate mutations** — try `mutation { <nearMiss> { id } }` and read the
   suggestions for mutation names, then repeat for their input types.

## Automated

```bash
# Requires a captured set of request headers (auth cookie/token)
clairvoyance https://target.com/graphql -o schema.json -H 'Authorization: Bearer <token>' -w wordlist.txt
```

The tool drives the suggestion oracle, auto-resolves nested type/argument
names, and emits a partial `schema.json` usable with GraphQL Voyager or
`graphql-client` codegen.

## When suggestions are also disabled

Fall back to **field-probing from response shape**: query `me`/`user` with
increasingly nested selects and observe which fields return data vs
`Cannot query field`. Combine with client-side source maps (SPA bundle) that
often contain the full query strings the frontend sends — those queries name
the real fields.

## Evidence

Record the recovered `type.field` / `mutation` names and the raw
did-you-mean responses that revealed them; they justify the authz probes that
follow.
