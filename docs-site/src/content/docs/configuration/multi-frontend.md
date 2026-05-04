---
title: Multi-frontend Setup
description: One SupaLite instance serving web + mobile + marketing site.
---

A typical indie project has multiple frontends pointing at the same auth backend: a Next.js web app, a React Native or mobile app, maybe a marketing site. SupaLite expects this and configures it via two env vars.

## Two knobs

In `.env` (or via the admin panel's Settings → General tab):

```bash
# Primary frontend — used in email templates, OAuth defaults
SITE_URL=https://app.example.com

# Regex of ALL allowed CORS origins. Empty = fall back to SITE_URL only.
# Escape dots; alternation with `|`.
CORS_ALLOWED_ORIGINS_REGEX=https://app\.example\.com|https://admin\.example\.com|https://marketing\.example\.com

# Comma-separated extra OAuth redirect URIs (SITE_URL is implicitly allowed).
# Use for mobile deep links and any non-primary web frontend.
GOTRUE_URI_ALLOW_LIST=https://admin.example.com/auth/callback,myapp://auth
```

Restart the affected services so they pick up new env:

```bash
docker compose up -d gateway gotrue
```

## How CORS matching works

The Caddyfile uses `header_regexp Origin "^({$CORS_ALLOWED_ORIGINS_REGEX})$"`. On match, the request's actual `Origin` is echoed back as `Access-Control-Allow-Origin` (you can't combine `*` with cookies, so per-origin echo is required).

Unmatched origins get **no CORS headers** — the preflight returns 204 with no headers, and the browser blocks the actual request.

## How OAuth redirects work

When your frontend calls `signInWithOAuth({ redirectTo: "https://admin.example.com/auth/callback" })`, GoTrue checks `redirectTo` against:

1. `SITE_URL` (always allowed)
2. Each entry in `GOTRUE_URI_ALLOW_LIST` (substring match — so `myapp://` matches any URL starting with that scheme)

If neither matches, the OAuth request is rejected with a 400.

## Why not just `*`?

CORS with credentials (cookies, `Authorization` header) explicitly forbids `Access-Control-Allow-Origin: *`. Browsers reject it. So we MUST echo the actual origin, which means we need an allowlist.

The regex form was chosen over a CSV because it's natural for the common cases (one-off origin, group of subdomains via `https://([a-z]+)\.example\.com`) and remains simple to extend.

## What `SITE_URL` controls beyond CORS

- **Email confirmation links** — only `SITE_URL`/path. Pick your primary frontend; mobile deep links don't appear in emails.
- **Default OAuth `redirectTo`** — when frontend doesn't specify one
- **`GOTRUE_SITE_URL`** in the GoTrue container env

## Multi-instance vs multi-frontend

Multi-**frontend** (this page): one SupaLite, several frontends sharing the same user accounts.

Multi-**instance** (separate concept): want totally independent projects on one host? Run multiple compose projects. See [Compose Profiles](/supalite/configuration/compose-profiles/) for the `docker compose -p` pattern.
