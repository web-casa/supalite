---
title: ANON vs SERVICE_ROLE
description: 什么时候用哪个 key。
---

| | `ANON_KEY` | `SERVICE_ROLE_KEY` |
|---|---|---|
| Postgres 角色 | `anon` | `service_role` |
| 绕过 RLS | **不** | **是** |
| 前端安全 | **安全** | **绝不** |
| 典型用法 | 浏览器、移动端、公开脚本 | 服务端管理任务、cron、迁移 |

## 区别在哪

两者都是 HS256 JWT，用 `JWT_SECRET` 签。**唯一**区别在 `role` claim。

PostgREST 收到请求后，按 JWT 说的设 Postgres session 角色：

- `role: anon` → `SET ROLE anon` → RLS 策略生效
- `role: authenticated` → `SET ROLE authenticated` → RLS 策略生效
- `role: service_role` → `SET ROLE service_role` → 该角色有 `BYPASSRLS`，策略跳过

## 什么时候用 ANON_KEY

所有客户端默认：

```js
const supabase = createClient(URL, ANON_KEY);
// 用户浏览公开数据：SELECT 受 anon 策略约束。
// 用户登录后：后续调用带其 JWT，RLS 把 ta 当作 `authenticated`。
```

常见模式：

```sql
-- 公开商品任何人可读
create policy "anyone reads products"
  on products for select to anon, authenticated using (true);

-- 只有登录用户能写评论
create policy "auth user inserts review"
  on reviews for insert to authenticated
  with check (user_id = auth.uid());
```

## 什么时候用 SERVICE_ROLE_KEY

只在服务端。常见场景：

- **后台任务** —— 定时脚本要跨用户更新行
- **管理工具** —— 你内部员工面板要看任何人数据
- **迁移** —— 用 SQL 程序化改 schema
- **Webhook** —— Stripe 之类回调要给任意用户的订单标已付款

```js
// Node.js / serverless 后端，永远不在浏览器代码里：
const admin = createClient(URL, SERVICE_ROLE_KEY);
await admin.from('orders').update({ paid: true }).eq('id', orderId);
```

## 常见错误

### 把 SERVICE_ROLE_KEY 放在 `NEXT_PUBLIC_*` env

`NEXT_PUBLIC_*` env 被打包进 client JS。任何人查看源码就能读。**永远**用服务端 only 的 env 名（不带 `NEXT_PUBLIC_` 前缀）装 `SERVICE_ROLE_KEY`。

### 因为 RLS 难就用 SERVICE_ROLE_KEY

RLS 策略是 Supabase 风格 app 安全的根基。RLS 写不对就摸 `SERVICE_ROLE_KEY`，等于把安全逻辑挪到了应用代码——通常**更**难审计。

跟 RLS 较劲时，在 Studio SQL 编辑器里 `set role authenticated; set request.jwt.claims = '{"sub":"...","role":"authenticated"}';` 直接调试策略。

### 混淆 access JWT 和 ANON_KEY

GoTrue 每次登录签发的用户 JWT 和 ANON_KEY 都是 JWT 但 `role` claim 不同。`supabase-js` 请求里 `Authorization: Bearer ...` 是**用户 JWT**，不是 anon key。`apikey` 头才是 **anon key**（或服务端用的 service role key）。

## 轮换时各自怎样

| 轮换 | 对 ANON_KEY 的影响 | 对 SERVICE_ROLE_KEY 的影响 |
|---|---|---|
| `JWT_SECRET` | 自动重签；客户端 app 要换新值 | 自动重签；服务端脚本要换新值 |
| 直接轮换 ANON_KEY | （向导不支持——只能通过 JWT_SECRET） | 不变 |
| 直接轮换 SERVICE_ROLE_KEY | 不变 | （向导不支持——只能通过 JWT_SECRET） |

两 key 一起级联。想只轮一个不轮另一个，得自己写脚本手动签——少见到不值得给个按钮。
