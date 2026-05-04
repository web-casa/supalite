---
title: 行级安全 RLS
description: Postgres 原生的逐行授权。让"前端拿 anon key"变安全的关键。
---

RLS 是 Postgres 的能力：每一行被 SELECT / INSERT / UPDATE / DELETE 之前先过 SQL **policy** 过滤。**同一条** `SELECT * FROM todos` 在不同调用者那里返回不同的行——依据是**谁**在调。

在 SupaLite 里，RLS 是让"前端带 ANON_KEY 上线"安全的根本——JWT 标识调用者，RLS 策略管它能看见什么。

## 30 秒例子

```sql
-- 表
create table todos (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references auth.users(id),
  title text not null
);

-- 启用 RLS——不开则没有策略生效，表完全开放。
alter table todos enable row level security;

-- 策略：用户只能 SELECT 自己的行。
create policy "owner reads own"
  on todos for select
  using (user_id = auth.uid());

-- 策略：用户只能 INSERT 自己 user_id 的行。
create policy "owner inserts own"
  on todos for insert
  with check (user_id = auth.uid());

-- update / delete 同理：
create policy "owner updates own"
  on todos for update
  using (user_id = auth.uid());
create policy "owner deletes own"
  on todos for delete
  using (user_id = auth.uid());
```

前端：

```js
const { data } = await supabase.from('todos').select('*');
// → 只返回 user_id = 当前登录用户的行
```

PostgREST 把用户 JWT 传给 Postgres。SupaLite 的 `auth.uid()` 从 `request.jwt.claims` 抽出 `sub`。策略自动生效。

## `using` vs `with check`

- **`using`** —— **读**行时应用（SELECT、UPDATE、DELETE 只看见匹配的行）。
- **`with check`** —— **写**行时应用（INSERT、UPDATE 拒绝不匹配的行）。

UPDATE 通常两者都要：`using` 找行，`with check` 防止把别人的 `user_id` 改成自己：

```sql
create policy "owner updates own"
  on todos for update
  using (user_id = auth.uid())
  with check (user_id = auth.uid());
```

## auth 辅助函数

SupaLite 在 `auth` schema 里提供：

| 函数 | 返回 |
|---|---|
| `auth.uid()` | 当前用户 UUID，匿名为 NULL |
| `auth.role()` | `'authenticated'`、`'anon'`、`'service_role'` |
| `auth.email()` | 当前用户邮箱 |
| `auth.jwt()` | 完整解码的 JWT payload (JSON) |

它们从 `request.jwt.claims` 读 —— 一个 Postgres GUC，PostgREST 每次请求根据传入 JWT 设置。

## 绕过 RLS（有意）

`SERVICE_ROLE_KEY` 是 `role: 'service_role'` 的 JWT。`service_role` Postgres 角色有 `BYPASSRLS`，策略不生效——服务端管理操作有用。

**绝对不要把 `SERVICE_ROLE_KEY` 暴露给前端**。否则任何用户都能看 / 改任何行。

## 多前端租户

一个 SupaLite 服务多个用户池不同的 app，加 `tenant_id` 列闸门：

```sql
create policy "tenant isolation"
  on documents for select
  using (
    user_id = auth.uid()
    AND tenant_id = (auth.jwt() ->> 'tenant_id')::uuid
  );
```

签 token 时在自定义 JWT claim 里塞 `tenant_id`（GoTrue 支持通过 admin API 写 `app_metadata` claim）。

## 常见坑

- **忘了 `enable row level security`** —— 表完全开放。Studio 表编辑器会警告；`pg_class.relrowsecurity` 能查状态。
- **INSERT 忘了 `with check`** —— 用户能 insert 别人 owned 的行。
- **不小心用了 SERVICE_ROLE_KEY** —— 全部策略被绕过。审计哪个 env var 装的是哪个 key。
- **递归策略** —— 一个策略不带 `BYPASSRLS` 引用同一张表会死循环或慢。跨表 join 用 `security definer` 函数。

## 延伸阅读

- [Supabase RLS docs](https://supabase.com/docs/guides/auth/row-level-security) —— 上游 Supabase 指南；那里讲的全部适用 SupaLite。
- 示例项目 [`examples/nextjs-todo/schema.sql`](https://github.com/web-casa/supalite/blob/main/examples/nextjs-todo/schema.sql) 是可直接运行的起点。
