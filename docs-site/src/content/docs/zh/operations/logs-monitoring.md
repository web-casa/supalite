---
title: 日志与监控
description: 任意容器实时日志；一眼看服务状态。
---

## 实时日志流

`/admin/logs/` 用 Server-Sent Events (SSE) 实时流任意容器的 stdout + stderr。

- 下拉里挑服务 —— `db`、`rest`、`gotrue`、`gateway`、`admin`。
- 选初始回填行数（50 / 100 / 500）。
- "live" 指示灯连上变绿。
- 点 **Pause** 冻结；**Resume** 重连（关闭再打开 SSE；暂停期间产生的行会丢）。

SSE 连接经 Caddy 不缓冲传输，再经 admin 的 docker-API client（自己开 `follow=true` log tail 到 Docker daemon）。关 tab 或点 Pause 时，上游 Docker 连接也关——不留孤儿 reader。

客户端最多 2000 行；老的从顶部掉以保 DOM 响应。

## 服务状态

两处显示服务状态：

1. **侧栏圆点** —— 每页侧栏底部都有 "Services" 区，每个运行容器一个彩色圆点。SSE 通过 `/api/status/stream` 实时更新（不轮询）。
2. **Dashboard** —— `/admin/` 上的完整表格。

颜色：

- **绿** —— `running`
- **红** —— `exited`
- **黄** —— 中间态（`restarting`、`created`、`dead`）

服务变红，下一步最有用的是去 Logs 页看它的日志。

## 管理面板里**没有**的

- **Postgres 指标**（TPS、复制延迟、query 耗时）—— Studio 的 Reports 页，或直接 SQL 查 `pg_stat_*` 视图。
- **Caddy 访问日志** —— `docker logs supalite-gateway-1`。Logs 页也能 tail gateway。
- **告警** —— 内置无。生产部署接外部 uptime check（如 UptimeRobot 的 `curl /health`）。

## 排查不健康的栈

1. 侧栏圆点 —— 哪个不是绿？
2. 打开 Logs → 那个服务 → 倒滚 ~100 行看 panic / error。
3. db 不健康时，`docker inspect supalite-db-1 --format '{{.State.Health.Log}}'` 看最近 healthcheck 输出。
4. 怀疑资源耗尽时，宿主上 `docker stats` 看每容器 CPU/RAM。

## 重启服务

Settings → **Restart Services** 重启 `gotrue` + `gateway`（改配置后最常重启的组合）。其它服务，宿主上 `docker compose restart <service>` 是明确的杠杆。

admin 不会因 .env 改动自动重启——什么时候宕机由你掌握。
