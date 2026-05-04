---
title: 端口与网络
description: 谁绑定哪里、为什么。
---

## 默认端口图

```
host:8000     ───►  gateway      :8000  (Caddy)
host:5432     ───►  db           :5432  (Postgres，默认只 loopback)
                      └─ 内网由 rest、gotrue、admin 访问

仅容器内：
  rest         :3000
  gotrue       :9999
  meta         :8080  (仅 COMPOSE_PROFILES=studio)
  studio       :3000  (同)
  admin        :9100
```

宿主只看到 `:8000`（和 loopback 的 `:5432`）。其它都在 Docker 内网桥 `supalite_default` 上。

## 为什么 Postgres 默认 loopback

Postgres 绑 `0.0.0.0:5432` 到公网是常见地雷——bot 扫端口暴力破解密码。SupaLite 默认 `POSTGRES_BIND_ADDR=127.0.0.1`，只有宿主能直连。

Docker 网络内的 app 用 `db:5432`（Docker DNS 解析），不走宿主端口。宿主端口给你 shell 里的 `psql`。

要外露 Postgres（除非你确定为什么）：

```bash
# .env
POSTGRES_BIND_ADDR=0.0.0.0
```

然后要么防火墙限 :5432 到 IP 白名单，要么仔细配 `pg_hba.conf`（supabase/postgres 镜像默认 hba 很宽松）。

## HTTPS 启用后多出的端口

`docker-compose.https.yml` override 加：

```
host:80       ───►  gateway      :80   (ACME challenge + HTTP→HTTPS 重定向)
host:443      ───►  gateway      :443  (TLS 终止流量)
```

`CADDY_HTTPS_BIND` 控制这两个的宿主 IP。空（默认）= `0.0.0.0`（Let's Encrypt 公网到 :80 必需）。

## 容器间 DNS

Compose 创建 `supalite_default` 网络。容器按服务名互相访问：

| 源 | 目标 |
|---|---|
| rest → `postgres://...@db:5432/...` | db |
| gotrue → `postgres://...@db:5432/...` | db |
| admin → `http://docker/...`（经 docker.sock）| docker daemon |
| admin → DB pool | db（经 DATABASE_URL）|
| meta → db | db |
| studio → meta | meta |
| Caddy `reverse_proxy rest:3000` | rest |

横向扩展（compose v2 不真支持，但可手动 `docker run` 多个）时，服务名 round-robin 解析。SupaLite 用不上（单副本架构）。

## 卷

| 卷 | 存什么 | 生命周期 |
|---|---|---|
| `supalite_db-data` | Postgres 数据目录 | **持久化。** `docker compose down` 保留；`down -v` 抹掉 |
| `caddy-data` | Let's Encrypt 证书 + ACME 状态 | 持久化（仅启用 HTTPS 时创建）|
| `caddy-config` | Caddy autosave 配置 | 持久化 |

Bind 挂载（只读）：

- `./volumes/db/init/zz-supalite.sh` → db 首启 init 脚本
- `./volumes/api/Caddyfile` → gateway 配置
- `./volumes/api/www` → `/` 上的静态着陆页
- `./` → `/project`（admin 的 `working_dir`，让它能用同一个 `.env` 跑 `docker compose`）

`/var/run/docker.sock` 挂进 admin 让它能：
- 流容器日志
- 检查兄弟容器
- 在 db 内 exec pgbackrest 命令
- 为恢复创建 one-shot 容器
- 调度重启时 start/stop 容器

这是**有意的强权限**——admin 需要兄弟容器管理才能干活。信任模型是"管理面板访问 ⇒ 宿主 root"。

## 出网

容器内向外：

- gotrue → SMTP 服务器（你配的 host）
- gotrue → OAuth provider（GitHub/Google/Apple API）
- admin → S3 兼容桶（备份 + presigned URL）
- db → S3（启用 pgBackRest 后，WAL 归档 + 备份）

如果防火墙限制出网，放行：
- 你的 SMTP host
- `api.github.com`、`oauth2.googleapis.com`、`appleid.apple.com`（你启用的 OAuth provider）
- 你的 S3 endpoint

Let's Encrypt：放行 `acme-v02.api.letsencrypt.org` 和 OCSP responder。
