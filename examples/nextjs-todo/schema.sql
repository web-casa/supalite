-- Minimal todos table with row-level security.
-- Apply once against a running SupaLite instance.

create table if not exists public.todos (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references auth.users(id) on delete cascade,
    title text not null check (length(title) between 1 and 200),
    done boolean not null default false,
    created_at timestamptz not null default now()
);

create index if not exists todos_user_id_idx on public.todos (user_id);

-- RLS: every row is scoped to its owner. supabase-js attaches a JWT
-- so PostgREST sees auth.uid() = the signed-in user's UUID.
alter table public.todos enable row level security;

drop policy if exists todos_owner_select on public.todos;
create policy todos_owner_select on public.todos
    for select using (user_id = auth.uid());

drop policy if exists todos_owner_insert on public.todos;
create policy todos_owner_insert on public.todos
    for insert with check (user_id = auth.uid());

drop policy if exists todos_owner_update on public.todos;
create policy todos_owner_update on public.todos
    for update using (user_id = auth.uid());

drop policy if exists todos_owner_delete on public.todos;
create policy todos_owner_delete on public.todos
    for delete using (user_id = auth.uid());

-- PostgREST needs the anon and authenticated roles to see the table.
-- RLS still blocks unauthorized rows; this just exposes the relation.
grant select, insert, update, delete on public.todos to authenticated;
grant usage on schema public to anon, authenticated;
