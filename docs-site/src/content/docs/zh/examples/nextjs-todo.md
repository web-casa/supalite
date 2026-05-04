---
title: Next.js Todo
description: 端到端最小示例——登录 + RLS 保护的 CRUD，约 200 行。
---

示例在主仓库的 [`examples/nextjs-todo/`](https://github.com/web-casa/supalite/tree/main/examples/nextjs-todo)。约 200 行 TypeScript 演示：

- `supabase-js` 邮箱密码注册 / 登录
- `todos` 表 RLS 保护（每个用户只看到自己的行）
- CRUD：list / add / toggle / delete

## 准备

1. 在另一个终端跑 SupaLite：
   ```bash
   cd path/to/supalite
   ./setup.sh
   ```

2. 把示例的 schema 应用到运行中的栈：
   ```bash
   docker exec -i supalite-db-1 psql -U supabase_admin -d postgres < examples/nextjs-todo/schema.sql
   ```
   （或粘到 `http://localhost:8000/studio/` 的 SQL 编辑器里。）

3. 在 `examples/nextjs-todo/`：
   ```bash
   cp .env.local.example .env.local
   $EDITOR .env.local   # 从 /admin/ Dashboard 复制 ANON_KEY
   npm install
   npm run dev
   ```

4. 打开 `http://localhost:3000`。注册、加 todo、登出、再登录——todo 还在。换个用户注册，看不到上个用户的 todo（RLS 在工作）。

## 该看什么

| 文件 | 看什么 |
|---|---|
| `schema.sql` | 4 条 RLS 策略，数据隔离的关键 |
| `lib/supabase.ts` | 单个 `createClient`——只用 anon key，绝不用 service role |
| `app/page.tsx` | 单文件 SPA：登出时显示 auth form，登录后显示 todo 列表 |

## 进一步

- **OAuth**：在 SupaLite Settings 启用 GitHub/Google → 把 `signInWithPassword` 换成 `signInWithOAuth({ provider: 'github' })`。
- **类型生成**：`npx supabase gen types typescript --db-url postgres://...` 从 schema 生成 TS 类型。
- **Realtime**：SupaLite 故意不带 Supabase realtime 服务。现在可用 `setInterval` 轮询；要真正的实时订阅请上游 Supabase。
