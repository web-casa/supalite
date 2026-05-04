---
title: HTTPS / TLS
description: 启用 Caddy 自动 HTTPS（Let's Encrypt）。
---

默认 `docker compose up` 是**纯 HTTP，端口 8000**。要让 Caddy 前置自动续期 TLS：

## 5 步启用

1. **DNS** — 公网 A/AAAA 记录指向本机。
2. **`.env`**：
   ```
   CADDY_SITE_ADDR=db.example.com
   API_EXTERNAL_URL=https://db.example.com
   ```
   多域名空格分隔：`CADDY_SITE_ADDR="a.example.com b.example.com"`。
3. **确保 80 + 443 端口公网可达** — Let's Encrypt 的 HTTP-01 challenge 打 `:80`，TLS 服务在 `:443`。
4. **用 HTTPS override 文件启动**：
   ```bash
   docker compose -f docker-compose.yml -f docker-compose.https.yml up -d
   ```
5. **看 gateway 日志**：`docker logs supalite-gateway-1 -f`。Caddy 10–60 秒拿到证书；后续重启从 `caddy-data` named volume 复用。

## 为什么用 override 文件？

`docker-compose.https.yml` 加 `:80` `:443` 宿主端口映射。默认文件不带这两个端口意味着普通的 `docker compose up` 不会偷偷占住这两个端口——升级到已有 web 服务监听 80/443 的宿主时尤其重要。

## 证书持久化

`caddy-data` named volume 存：

- 签发的证书和私钥
- ACME 账户密钥
- OCSP staple

`docker compose down` 不会清卷。`docker compose down -v` 会——下次启动会重新申请证书（Let's Encrypt 限速：每个 hostname 每周 5 张重复证书）。

## 自签 / 内部 CA / 通配符证书

直接编辑 `volumes/api/Caddyfile`，用 Caddy 的 `tls` 指令：

```caddyfile
{$CADDY_SITE_ADDR} {
    tls /path/to/cert.pem /path/to/key.pem
    # ... 站点块其余部分
}
```

通过 `docker-compose.https.yml` 多挂一个卷把证书/密钥送进 gateway 容器。

## 本地测试（不走 ACME）

开发时域名指向自己笔记本，用 Caddy local CA 模式：

```caddyfile
{$CADDY_SITE_ADDR} {
    tls internal
    # ...
}
```

浏览器会警告（证书不在信任库），curl 需要 `-k`。生产别这样部署。

## 故障排查

| 现象 | 常见原因 |
|---|---|
| gateway 日志报 `cert provisioning errored` | DNS 还没传播，或 :80/:443 公网不可达 |
| 启用后浏览器还是自签警告 | 证书还在签 — 等 60s，看 gateway 日志 |
| 一直显示老证书 | 浏览器缓存。硬刷新（Ctrl/Cmd+Shift+R） |
| Let's Encrypt 限速错误 | 本周给这个域名签过 >5 张证书。等等，或者用 staging endpoint |
