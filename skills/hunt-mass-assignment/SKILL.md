---
name: hunt-mass-assignment
description: Hunting skill for mass assignment / over-posting (CWE-915) — sending extra JSON fields (role, isAdmin, price, balance, verified, userId, discountPercent) that a backend binds wholesale to a model/struct with no allowlist. Covers Rails strong-params gaps, Django ModelForm over-exposure, ASP.NET and Spring model binding, and ORM auto-serialization (Mongoose, Sequelize, TypeORM). Use on any create/update JSON endpoint backed by a model.
---

# Mass Assignment Hunting Methodology

Mass assignment (over-posting) happens when a create/update endpoint binds the *entire* attacker-supplied JSON body onto a model or struct instead of copying only the fields the API is documented to accept. Any model attribute that isn't explicitly excluded becomes a write-primitive the client can reach directly — privilege escalation via `role`/`isAdmin`, financial manipulation via `price`/`balance`/`discountPercent`, and horizontal takeover via `userId`/`ownerId` on someone else's resource. This skill covers black-box hunting.

## When to invoke

- Any `POST`/`PUT`/`PATCH` endpoint that accepts a JSON object and persists it to a database-backed model.
- Endpoints backed by a framework with implicit whole-object binding: Rails (`ActiveRecord#update`/`create` fed from `params`), Django (`ModelForm`/DRF `ModelSerializer` with a broad `fields`), ASP.NET (MVC/Web API model binding onto an EF entity), Spring (`@ModelAttribute` or `@RequestBody` deserialized straight onto a JPA entity).
- Any Node.js/JS backend handler that passes `req.body` straight into an ORM call: `Model.create(req.body)` / `Model.update(req.body, ...)` (Mongoose, Sequelize), or a TypeORM `repository.save({...req.body, id})`.
- User self-service endpoints (profile update, settings, cart/checkout) where the underlying model also carries admin- or system-managed fields (role, verification status, balance, pricing, ownership).
- Signup/registration endpoints — a prime target since the resulting record often inherits privilege fields before any role-based check would normally apply.

## Technique: diff the documented surface against the real model

1. **Establish the documented/expected field set.** Pull it from whichever of these exists: published API docs, an OpenAPI/Swagger spec (`/swagger.json`, `/openapi.yaml`, `/v2/api-docs`), the client-side JS bundle (the object literal or TypeScript interface the frontend actually sends), or GraphQL introspection (`__type` on the corresponding input type) if the same backend model is exposed both ways.
2. **Establish the full underlying model/schema.** Look for a second, wider field set the backend trusts internally: a GraphQL type's full field list vs. its mutation input type, a response body that echoes more fields than the request accepted (GET on the same resource after creation, or a verbose/debug response), an admin-panel or internal-API variant of the same endpoint, error messages that leak column/field names (ORM validation errors, stack traces), or a leaked schema/migration file.
3. **Send the full documented request, then re-send it with each undocumented field appended one at a time** (and in combination), using realistic values for its apparent type. Keep every other field identical across requests so a behavior diff is attributable to the one added field.
4. **Confirm by reading the result back**, not by the write response alone — many frameworks return `200`/`201` with the submitted body echoed verbatim regardless of what was actually persisted. Fetch the resource via a separate `GET` (or as a different, lower-privileged session) and check whether the extra field's value actually took effect.

## Framework-specific exposure patterns

| Framework | Where the gap lives |
|---|---|
| Rails | `strong_params` only enumerates permitted keys *where the developer remembered to*; a controller still calling `Model.new(params[:model])` or `.permit!` (wildcard) accepts everything. Also watch `accepts_nested_attributes_for` — it opens a nested-object path that often has its own, separately-forgotten allowlist. |
| Django | A `ModelForm`/DRF `ModelSerializer` with `fields = '__all__'` (or a `Meta.fields` list that's broader than the documented request) exposes every listed field to client input, including ones the frontend never sends but the backend will still accept and process. |
| ASP.NET (MVC / Web API) | Default model binding populates every public property on the action's parameter type from the body. If that type *is* the EF entity (not a dedicated DTO/ViewModel), every entity property — including ones with database-only intent — is bindable unless `[Bind(Include=...)]`/`[BindNever]` or a separate view-model is used. |
| Spring (`@ModelAttribute` / `@RequestBody`) | Binding straight onto a JPA `@Entity` instead of a dedicated request DTO exposes every entity field Jackson can deserialize. `@JsonIgnore` on the getter only blocks *serialization* out — it does not block deserialization in; check whether the gap is write-only. |
| Generic ORM auto-serialization (Mongoose, Sequelize, TypeORM) | Passing the raw request body straight into `create()`/`update()`/`save()` persists every key present in the body that matches a schema field, including ones a `select: false` or `@Exclude()` decorator only hides on the *read* path. Sequelize/TypeORM save calls that merge `req.body` onto an existing entity (`Object.assign(entity, req.body)`) are an even more direct over-write path since no field list is consulted at all. |

## Nested-object and array-of-objects variants

Some frameworks allowlist only the *top level* of a request body and pass nested objects through unchecked — this is where a surface-level fix (e.g. a DTO wrapping the top-level fields) still leaves a gap:

- Wrap the privileged field one level deeper inside a legitimately-accepted nested object: `{"profile": {"name": "x", "role": "admin"}}` when `profile` is itself a permitted top-level key.
- If the endpoint accepts an array of sub-resources (line items, addresses, team members), try the privileged field inside **each array element**: `{"items": [{"sku": "x", "price": 0.01}]}` — array-element binding is frequently handled by a different code path than the parent object and may skip the allowlist entirely.
- For `accepts_nested_attributes_for` (Rails) or a similar nested-create pattern, try smuggling the privileged field into the nested-attributes hash rather than the top level.
- Try the same undocumented field at every nesting depth the schema plausibly supports — a field blocked at depth 0 may be unchecked at depth 1 or 2.

## Example payload set

| Documented field | Added field | Type / example value | What it targets |
|---|---|---|---|
| `{"name": "Alice"}` on `PATCH /users/me` | `"role": "admin"` | string | Privilege escalation via self-service profile update |
| `{"username": "bob"}` on `POST /signup` | `"isAdmin": true` | boolean | Privilege escalation at account creation, before any role check applies |
| `{"title": "Widget"}` on `POST /products` | `"price": 0.01` | number | Price manipulation on a resource the client should only describe, not price |
| `{"amount": 10}` on a wallet top-up endpoint | `"balance": 999999` | number | Direct balance overwrite instead of the intended increment logic |
| `{"comment": "..."}` on `POST /orders/{id}/notes` | `"userId": "<victim-id>"` | string/UUID | Re-parenting a resource to another user (ownership takeover) |
| `{"code": "SAVE10"}` on a coupon-application endpoint | `"discountPercent": 100` | number | Business-logic bypass via attacker-chosen discount |
| `{"email": "a@b.com"}` on a KYC/profile update | `"verified": true` | boolean | Skipping a verification workflow entirely |
| `{"name": "Team"}` on `POST /teams` | `"members": [{"userId": "me", "role": "owner"}]` | array of objects | Nested/array-element bypass of a top-level-only allowlist |

## Depth floor

Do not report this surface exhausted until you have tried **at least 25 distinct field × endpoint combinations** — vary the injected field name, its guessed internal name (camelCase/snake_case, aliasing like `is_admin` vs `isAdmin` vs `admin`), its placement (top level vs. nested vs. array element), and its target endpoint across every create/update surface found. Record every negative result as a `record_fact` so the planner knows what was covered.

## Evidence gate

Before `report_finding`:
- The exact request that includes the undocumented field, **and** a subsequent, independent read (a separate `GET`, or the same resource viewed from a different/lower-privileged session) proving the field's value was actually persisted and takes effect — not just that the write endpoint returned `200`/`201` or echoed the field back in its own response body.
- Where the field drives a privilege or financial outcome, demonstrate the outcome itself (the now-escalated session performing an admin-only action, the altered price actually charged at checkout) rather than stopping at "the field was stored."
- For nested/array-element bypasses, include the request showing the field at its working depth alongside a negative control showing the same field rejected or ignored at the top level — establishing that the nesting, specifically, is what got it through.
- Tag the finding `vulnclass: mass-assignment` and bind `traffic_refs`.

## Stop-authority

This skill decides *what mass-assignment vectors to try* and *how to verify*. It never decides when the entire vulnerability class is exhausted — that stays with `hunting-methodology`.
