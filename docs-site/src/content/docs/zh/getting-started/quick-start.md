---
title: 5 分钟启动
description: 从 git clone 到可用 Postgres 后端，三分钟搞定。
---

你需要一台装好 **Docker** + **Docker Compose** 的主机。SupaLite 面向 Linux 设计；macOS / Windows 通过 Docker Desktop 或 OrbStack 用于开发也行。

## 1. Clone 并运行 setup

```bash
git clone https://github.com/web-casa/supalite.git
cd supalite
./setup.sh
```

`setup.sh` 做三件事：

1. 生成所有密钥——`POSTGRES_PASSWORD`、`JWT_SECRET`、`ADMIN_TOKEN`、`COOKIE_SIGNING_KEY`、`PG_META_CRYPTO_KEY`——写入 `.env`（权限 0600）。
2. 用 `JWT_SECRET` 签发 `ANON_KEY` 和 `SERVICE_ROLE_KEY` JWT。
3. `docker compose pull` 然后 `docker compose up -d`，最后给每个公共端点做健康检查。

末尾会打印管理令牌。**保存好**——丢了也能在 `.env` 里找到。

:::tip[OS 提示]
原生 Linux 是首选目标。macOS / Windows 通过 Docker Desktop 或 OrbStack 用于开发；Apple Silicon 上 OrbStack 处理卷权限比 Docker Desktop 干净。
:::

## 2. 打开管理面板

```
http://localhost:8000/admin/
```

用 `setup.sh` 打印的 `ADMIN_TOKEN` 登录。首次运行向导会引导你：

- 设置 `SITE_URL` 和 `API_EXTERNAL_URL`
- 配置 SMTP（带实时 "Send Test" 按钮）
- 展示 `ANON_KEY` 让你复制到前端

## 3. 打开 Studio

```
http://localhost:8000/studio/
```

Studio 是上游 Supabase UI——表编辑器、SQL 编辑器、auth 用户、函数浏览器。应用开发用 Studio，运维用 `/admin/`。

## 4. 从前端调用 API

```js
import { createClient } from '@supabase/supabase-js';

const supabase = createClient(
  'http://localhost:8000',     // API_EXTERNAL_URL
  '<从 /admin/ 复制的 ANON_KEY>',
);

const { data } = await supabase.from('your_table').select('*');
```

完整可运行示例见 [Next.js Todo](/supalite/zh/examples/nextjs-todo/)。

## 跑起来都是什么

```
$ docker compose ps
NAME                STATUS              PORTS
supalite-db-1       Up (healthy)        127.0.0.1:5432->5432/tcp
supalite-rest-1     Up
supalite-gotrue-1   Up
supalite-meta-1     Up (healthy)
supalite-studio-1   Up (healthy)
supalite-admin-1    Up
supalite-gateway-1  Up                  0.0.0.0:8000->8000/tcp
```

只有 `gateway` 对外暴露（`:8000`）。Postgres 默认绑回环；只有真需要从外部直连数据库时才在 `.env` 里改 `POSTGRES_BIND_ADDR=0.0.0.0`。

## 下一步

- [架构总览](/supalite/zh/getting-started/architecture/) — 各部件如何协作
- [环境变量参考](/supalite/zh/configuration/environment-reference/) — 每个 `.env` 字段的解释
- [备份](/supalite/zh/operations/backups/) — 在需要前先把自动备份配好
- [HTTPS](/supalite/zh/configuration/https-tls/) — 生产环境启用 Let's Encrypt
