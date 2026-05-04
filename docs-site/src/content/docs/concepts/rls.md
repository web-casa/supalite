---
title: Row-Level Security (RLS)
description: Postgres-native per-row authorization. The thing that makes "anon key in the frontend" safe.
---

RLS is a Postgres feature where each row is filtered by a SQL **policy** before it can be selected, inserted, updated, or deleted. The same `SELECT * FROM todos` query returns different rows to different callers based on **who** is making the call.

In SupaLite, RLS is what makes it safe to ship the `ANON_KEY` to the browser — the JWT identifies the caller, RLS policies enforce what they can see.

## A 30-second example

```sql
-- Table
create table todos (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references auth.users(id),
  title text not null
);

-- Enable RLS — without this, no policies apply and the table is wide open.
alter table todos enable row level security;

-- Policy: a user can only SELECT their own rows.
create policy "owner reads own"
  on todos for select
  using (user_id = auth.uid());

-- Policy: a user can only INSERT rows where user_id = themselves.
create policy "owner inserts own"
  on todos for insert
  with check (user_id = auth.uid());

-- Same for update / delete:
create policy "owner updates own"
  on todos for update
  using (user_id = auth.uid());
create policy "owner deletes own"
  on todos for delete
  using (user_id = auth.uid());
```

Now from the frontend:

```js
const { data } = await supabase.from('todos').select('*');
// → returns ONLY rows where user_id = current logged-in user
```

PostgREST passes the user's JWT to Postgres. SupaLite's `auth.uid()` extracts the `sub` claim from `request.jwt.claims`. The policies fire automatically.

## `using` vs `with check`

- **`using`** — filter applied when **reading** rows (SELECT, UPDATE, DELETE see only matching rows).
- **`with check`** — filter applied when **writing** rows (INSERT, UPDATE refuse rows that don't match).

For UPDATE you typically want both: `using` to find the row, `with check` to ensure you can't update someone else's row by changing `user_id`:

```sql
create policy "owner updates own"
  on todos for update
  using (user_id = auth.uid())
  with check (user_id = auth.uid());
```

## Auth helper functions

SupaLite ships these in the `auth` schema:

| Function | Returns |
|---|---|
| `auth.uid()` | UUID of the current user, or NULL if anonymous |
| `auth.role()` | `'authenticated'`, `'anon'`, or `'service_role'` |
| `auth.email()` | Email of the current user |
| `auth.jwt()` | Full decoded JWT payload as JSON (advanced) |

These read from `request.jwt.claims` — a Postgres GUC that PostgREST sets per request from the incoming JWT.

## Bypassing RLS (intentionally)

`SERVICE_ROLE_KEY` is a JWT with `role: 'service_role'`. The `service_role` Postgres role has `BYPASSRLS`, so policies don't apply — useful for server-side admin operations.

**Never expose `SERVICE_ROLE_KEY` to the frontend.** It would let any user see/modify any row.

## Multi-frontend tenancy

If you're running one SupaLite for several apps with separate user pools, add a `tenant_id` column and gate by it:

```sql
create policy "tenant isolation"
  on documents for select
  using (
    user_id = auth.uid()
    AND tenant_id = (auth.jwt() ->> 'tenant_id')::uuid
  );
```

Set `tenant_id` in custom JWT claims when minting tokens (GoTrue supports `app_metadata` claims via the admin API).

## Common pitfalls

- **Forgetting `enable row level security`** — table is wide open without it. Studio's table editor warns; `pg_class.relrowsecurity` reveals state.
- **Forgetting `with check` on INSERT** — without it, a user could insert rows owned by someone else.
- **Using SERVICE_ROLE_KEY accidentally** — you're now bypassing all your policies. Audit which keys are in which env var.
- **Recursive policies** — a policy that references the same table without `BYPASSRLS` can infinite-loop or be slow. Use `security definer` functions for cross-table joins.

## Further reading

- [Supabase RLS docs](https://supabase.com/docs/guides/auth/row-level-security) — the upstream Supabase guide; everything there applies to SupaLite.
- The example project [`examples/nextjs-todo/schema.sql`](https://github.com/web-casa/supalite/blob/main/examples/nextjs-todo/schema.sql) is a runnable starting point.
