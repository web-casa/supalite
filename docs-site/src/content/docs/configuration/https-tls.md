---
title: HTTPS / TLS
description: Enable Caddy auto-HTTPS via Let's Encrypt.
---

The default `docker compose up` is **HTTP-only on port 8000**. To put Caddy in front with auto-renewing TLS certs:

## 5-step enable

1. **DNS** — point a public A/AAAA record at this host.
2. **`.env`**:
   ```
   CADDY_SITE_ADDR=db.example.com
   API_EXTERNAL_URL=https://db.example.com
   ```
   For multiple domains, space-separate: `CADDY_SITE_ADDR="a.example.com b.example.com"`.
3. **Make sure ports 80 + 443 are reachable from the public internet** — Let's Encrypt's HTTP-01 challenge hits `:80`, and TLS serving uses `:443`.
4. **Bring up with the HTTPS override file**:
   ```bash
   docker compose -f docker-compose.yml -f docker-compose.https.yml up -d
   ```
5. **Watch the gateway logs**: `docker logs supalite-gateway-1 -f`. Caddy provisions the cert in 10–60 seconds; subsequent restarts reuse it from the `caddy-data` named volume.

## Why an override file?

`docker-compose.https.yml` adds the `:80` and `:443` host port bindings. Keeping them out of the default file means a plain `docker compose up` doesn't silently take over those ports — important when you're upgrading on a host that already has a webserver bound to 80/443.

## Cert persistence

`caddy-data` named volume holds:

- Issued certificates and private keys
- ACME account keys
- OCSP staples

`docker compose down` does NOT wipe the volume. `docker compose down -v` DOES — and you'll re-request certs on next start (Let's Encrypt rate-limits 5 duplicate certs per hostname per week).

## Custom certs (internal CA, wildcard)

Edit `volumes/api/Caddyfile` directly and use Caddy's `tls` directive:

```caddyfile
{$CADDY_SITE_ADDR} {
    tls /path/to/cert.pem /path/to/key.pem
    # ... rest of site block
}
```

Mount your cert/key into the gateway container via an additional volume in `docker-compose.https.yml`.

## Local testing without ACME

For dev with a real domain pointed at your laptop, use Caddy's local CA mode:

```caddyfile
{$CADDY_SITE_ADDR} {
    tls internal
    # ...
}
```

Browsers will warn (cert not in trust store); curl needs `-k`. Don't ship this to prod.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| `cert provisioning errored` in gateway logs | DNS hasn't propagated, or :80/:443 aren't reachable from the internet |
| Browser shows self-signed warning after enable | Cert not provisioned yet — wait 60s, check gateway logs |
| Old cert keeps showing | Browser cached. Hard reload (Ctrl/Cmd+Shift+R) |
| Let's Encrypt rate-limit error | You've requested >5 certs for this hostname this week. Wait or use a staging endpoint |
