---
title: ANON vs SERVICE_ROLE
description: When to use which key.
---

| | `ANON_KEY` | `SERVICE_ROLE_KEY` |
|---|---|---|
| Postgres role | `anon` | `service_role` |
| Bypasses RLS | **No** | **Yes** |
| Safe for frontend | **Yes** | **No, never** |
| Typical use | Browser, mobile, public scripts | Server-side admin tasks, cron jobs, migrations |

## How they're different

Both are HS256 JWTs signed with `JWT_SECRET`. They differ only in their `role` claim.

When PostgREST sees a request, it sets the Postgres session's role to whatever the JWT says:

- `role: anon` → Postgres `SET ROLE anon` → RLS policies apply
- `role: authenticated` → Postgres `SET ROLE authenticated` → RLS policies apply
- `role: service_role` → Postgres `SET ROLE service_role` → `service_role` has `BYPASSRLS` so policies are skipped

## When to use ANON_KEY

The default for everything client-side:

```js
const supabase = createClient(URL, ANON_KEY);
// User browses public data: SELECT works subject to anon policies.
// User signs in: subsequent calls carry their JWT, RLS sees them as `authenticated`.
```

Common pattern:

```sql
-- Public products everyone can read
create policy "anyone reads products"
  on products for select to anon, authenticated using (true);

-- Only logged-in users can write reviews
create policy "auth user inserts review"
  on reviews for insert to authenticated
  with check (user_id = auth.uid());
```

## When to use SERVICE_ROLE_KEY

Server-side only. Common cases:

- **Background jobs** — a scheduled script that has to update rows across users
- **Admin tools** — your internal staff dashboard that can see anyone's data
- **Migrations** — programmatic schema changes via SQL
- **Webhooks** — Stripe/etc. callbacks that need to mark orders paid for any user

```js
// On a Node.js / serverless backend, NEVER in browser code:
const admin = createClient(URL, SERVICE_ROLE_KEY);
await admin.from('orders').update({ paid: true }).eq('id', orderId);
```

## Common mistakes

### Putting SERVICE_ROLE_KEY in `NEXT_PUBLIC_*` env vars

`NEXT_PUBLIC_*` env vars are bundled into the client JS. Anyone viewing source can read them. **Always** use a server-only env var name (no `NEXT_PUBLIC_` prefix) for `SERVICE_ROLE_KEY`.

### Using SERVICE_ROLE_KEY because RLS feels hard

RLS policies are how Supabase-style apps stay safe. Reaching for `SERVICE_ROLE_KEY` because you can't get a policy right means you've moved the security logic into your application code — which is generally **less** auditable than a SQL policy.

If you find yourself fighting RLS, use Studio's SQL editor with `set role authenticated; set request.jwt.claims = '{"sub":"...","role":"authenticated"}';` to debug your policies directly.

### Confusing access JWT with ANON_KEY

User session JWTs (issued by GoTrue per sign-in) and ANON_KEY are both JWTs but have different `role` claims. The `Authorization: Bearer ...` header on a `supabase-js` request is the **user JWT**, not the anon key. The `apikey` header is the **anon key** (or service role key for server-side).

## How rotation affects each

| Rotation | Effect on ANON_KEY | Effect on SERVICE_ROLE_KEY |
|---|---|---|
| `JWT_SECRET` | Re-minted automatically; client apps need new value | Re-minted automatically; server scripts need new value |
| `ANON_KEY` directly | (not supported by wizard — happens via JWT_SECRET) | unchanged |
| `SERVICE_ROLE_KEY` directly | unchanged | (not supported by wizard — happens via JWT_SECRET) |

Both keys cascade together. To rotate just one without the other, you'd need a custom script that signs a single JWT manually — uncommon enough that the wizard doesn't expose it.
