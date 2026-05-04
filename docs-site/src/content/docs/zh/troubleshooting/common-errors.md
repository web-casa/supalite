---
title: 常见问题
description: 现象、可能原因、修法。
---

## 安装

### `setup.sh` 失败：`port is already allocated`

8000（或 5432）端口被占。找出来：

```bash
sudo lsof -i :8000
```

要么停掉占用进程，要么改 `GATEWAY_HTTP_BIND` 和 `docker-compose.yml` 端口映射。

### `setup.sh` 依赖检查失败

需要 `docker`、`docker compose`（或 `docker-compose`）、`openssl`、`curl`。包管理器装。Linux 上要么把用户加 `docker` 组，要么命令前加 `sudo`。

### `/var/run/docker.sock` 权限拒绝

要么把用户加 docker 组：

```bash
sudo usermod -aG docker $USER
# 注销重登
```

……要么 compose 命令永远 `sudo`。管理面板需要这个 socket；以你用户身份跑更地道。

## 容器健康

### `db` 显示 `unhealthy`

常见原因：

1. **卷权限**（rootless docker / OrbStack 尤其）：
   ```
   chmod: changing permissions of '/var/lib/postgresql/data': Operation not permitted
   ```
   容器内用户改不了数据目录权限。重启 Docker / OrbStack 重试。持续就重建卷：`docker compose down -v && docker compose up -d`。

2. **健康检查找不到 pg_isready** —— 自定义 db 镜像构建时，`apt install pgbackrest` 带了 `postgresql-common` shim，覆盖了 `/usr/bin/pg_isready`，新 shim 不认识 Nix 装的 Postgres。验证容器内 `/nix/var/nix/profiles/default/bin` 是 PATH 第一项：
   ```bash
   docker exec supalite-db-1 sh -c 'echo $PATH'
   ```
   应该是 `/nix/var/...` 在 `/usr/bin` 之前。不是则 Dockerfile 需要 `ENV PATH=` 修复。

3. **Postgres 日志直接说**：
   ```bash
   docker logs supalite-db-1 --tail 50
   ```

### `admin` 反复重启

```bash
docker logs supalite-admin-1 --tail 20
```

常见：缺必需 env（`ADMIN_TOKEN`、`JWT_SECRET`、`DATABASE_URL`、`COOKIE_SIGNING_KEY`）。报错精确说哪个。重新跑 `setup.sh` 重生 `.env`。

日志里 `regexp: Compile(... ) error` —— admin 代码 bug，开 issue 附完整报错。

### `gateway` 在 `/admin/` 上 502

`admin` 没起或没就绪。`docker compose ps` 看。等 10s 再试。

## CORS / API

### 浏览器控制台 "blocked by CORS policy"

前端 origin 不在 `CORS_ALLOWED_ORIGINS_REGEX`。检查：

```bash
grep CORS /path/to/supalite/.env
```

默认回落 `SITE_URL`。要允许更多，设置正则（详见 [多前端](/supalite/zh/configuration/multi-frontend/)）。改完重启 gateway。

### `/rest/v1/...` 返 `401 Missing or invalid API key`

前端没带 `apikey` 头。原始 `fetch` 最常见——`supabase-js` 自动带。手动修：

```js
fetch('http://localhost:8000/rest/v1/your_table', {
  headers: {
    'apikey': '<ANON_KEY>',
    'Authorization': 'Bearer <ANON_KEY>'   // 或用户 JWT
  }
});
```

### `/rest/v1/...` 返了行，但不是预期的

RLS 在过滤。在 Studio SQL 编辑器以管理员身份跑：

```sql
set role authenticated;
set request.jwt.claims = '{"sub":"<user-uuid>","role":"authenticated"}';
select * from your_table;
```

……模拟用户视角。和 `set role service_role` 比，看表里实际有什么。差额就是 RLS 在隐藏的。

### OAuth 重定向 404 / `redirect_uri_mismatch`

provider 拒绝你注册的重定向 URI。检查：

1. provider 控制台上注册的是不是**精确** `${API_EXTERNAL_URL}/auth/v1/callback`（区分大小写、有没有尾斜杠？）
2. Settings → GitHub/Google → **Test credentials** —— 验证器能区分"凭证错"和"凭证对但 redirect_uri 没注册"。

## SMTP

### "Send Test" 成功但真注册不发邮件

检查 `GOTRUE_MAILER_AUTOCONFIRM` —— `true` 的话 GoTrue 自动确认新用户不发邮件。配好 SMTP 后改 `false`。

### "Send Test" 返 "STARTTLS required"

587 端口必须广告 STARTTLS。一些配错的 relay 没有。改 465（隐式 TLS）试试。

### "Send Test" 返 "auth failed"

用户名/密码错。再核对 `GOTRUE_SMTP_USER` 和 `GOTRUE_SMTP_PASS`。Gmail 需要应用专用密码，不是账户密码。

## 备份

### 备份永远不结束 / 卡住

30 分钟超时触发；admin 返 500。慢 S3 是常见原因；换离 DB 主机近的 S3，或换 pgBackRest（增量、传输量小）。

### "backup not configured"

`BACKUP_S3_BUCKET` 和凭证没设。Settings → Backup tab。

### 恢复半途失败

DB 处于不一致状态。要么：
- 重试同样的恢复（pg_restore 在 `--clean` 模式通常幂等）
- 从更早的备份恢复
- 抹卷重来（`docker compose down -v && docker compose up -d`，然后恢复）

## HTTPS

### 证书一直没签

Caddy 找不到 Let's Encrypt 或 LE 找不到你的 :80。检查：

1. `docker logs supalite-gateway-1 -f` —— Caddy 在说什么？
2. `dig <你的域名>` —— DNS 解析到这台主机吗？
3. 从另一台主机 `curl http://<你的域名>` —— 能到 Caddy 吗？
4. `CADDY_HTTPS_BIND` 是空（= 0.0.0.0）吗？loopback 公网到不了。

### 启用后浏览器还显示老证书

硬刷新（Ctrl/Cmd+Shift+R）。还缓存就重启 gateway：`docker compose restart gateway`。

## 哪里求助

- GitHub issue：<https://github.com/web-casa/supalite/issues>
- 包含：SupaLite 版本（`git rev-parse --short HEAD`）、`docker compose ps` 输出、相关日志片段。
