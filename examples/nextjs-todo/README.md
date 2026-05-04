# nextjs-todo — minimal SupaLite example

A 100-line Next.js 15 app showing the full SupaLite handshake: email
sign-up + sign-in, a `todos` table guarded by RLS, CRUD via
`supabase-js`. About as small as a real example can be.

## Setup

1. **Run SupaLite** in a sibling terminal:
   ```bash
   cd ../..       # repo root
   ./setup.sh
   ```

2. **Apply the schema** to the running stack:
   ```bash
   docker exec -i supalite-db-1 psql -U supabase_admin -d postgres \
     < schema.sql
   ```
   (Or paste `schema.sql` into Studio's SQL Editor at
   <http://localhost:8000/studio/>.)

3. **Configure this example**:
   ```bash
   cp .env.local.example .env.local
   ```
   Then open SupaLite's admin panel, copy the **anon key** from the
   dashboard, and paste it into `.env.local` as
   `NEXT_PUBLIC_SUPABASE_ANON_KEY`.

4. **Install + run**:
   ```bash
   npm install
   npm run dev
   ```

5. Open <http://localhost:3000>, sign up with any email/password,
   add a couple of todos, watch them stick. Sign out, sign in again,
   they're still there. Sign up as a *different* user — your todos
   are gone (RLS isolation in action).

## What this demonstrates

- **Auth via supabase-js** — email/password sign-up + sign-in,
  session stored in localStorage, automatic JWT refresh.
- **RLS** — `todos` table policies restrict every row to its
  `user_id = auth.uid()`. The same query `select * from todos`
  returns different rows per user.
- **Anon key is safe in the frontend** — it's a JWT with role
  `anon`, can't bypass RLS, can't access service-role-only tables.

## Files

- `schema.sql` — table + RLS policies
- `lib/supabase.ts` — single `createClient` call
- `app/page.tsx` — single-file app: auth form when logged out,
  todo CRUD when logged in
- `app/layout.tsx` + `app/globals.css` — minimal styling

## Going further

- Add OAuth: enable GitHub/Google in SupaLite's Settings, add
  `<button onClick={() => supabase.auth.signInWithOAuth({...})}>`
- Use realtime: subscribe with `supabase.channel(...).on('postgres_changes', ...)`
  (requires the realtime service which SupaLite intentionally omits;
  use a vanilla setInterval poll, or run upstream Supabase if you
  need it)
- Type generation: `npx supabase gen types typescript --db-url ...`
