---
title: Next.js Todo
description: Minimal end-to-end example — login + RLS-guarded CRUD in ~200 lines.
---

The example lives in [`examples/nextjs-todo/`](https://github.com/web-casa/supalite/tree/main/examples/nextjs-todo) in the main repo. About 200 lines of TypeScript demonstrate:

- Email + password sign-up / sign-in via `supabase-js`
- A `todos` table guarded by RLS (each user sees only their own rows)
- CRUD: list / add / toggle / delete

## Setup

1. Run SupaLite in a sibling terminal:
   ```bash
   cd path/to/supalite
   ./setup.sh
   ```

2. Apply the example's schema to the running stack:
   ```bash
   docker exec -i supalite-db-1 psql -U supabase_admin -d postgres < examples/nextjs-todo/schema.sql
   ```
   (Or paste it into Studio's SQL Editor at `http://localhost:8000/studio/`.)

3. In `examples/nextjs-todo/`:
   ```bash
   cp .env.local.example .env.local
   $EDITOR .env.local   # paste ANON_KEY from /admin/ Dashboard
   npm install
   npm run dev
   ```

4. Open `http://localhost:3000`. Sign up, add todos, sign out, sign in again — your todos persist. Sign up as a different user and your old user's todos are invisible (RLS in action).

## What to look at

| File | Why |
|---|---|
| `schema.sql` | The 4 RLS policies that make the data isolation work |
| `lib/supabase.ts` | Single `createClient` — anon key only, never service role |
| `app/page.tsx` | Single-file SPA: auth form when logged out, todo list when logged in |

## Going further

- **OAuth**: enable GitHub/Google in SupaLite Settings → swap `signInWithPassword` for `signInWithOAuth({ provider: 'github' })`.
- **Type generation**: `npx supabase gen types typescript --db-url postgres://...` to derive TS types from your schema.
- **Realtime**: SupaLite intentionally omits Supabase's realtime service. Use `setInterval` polling for now, or run upstream Supabase if you need true realtime subscriptions.
