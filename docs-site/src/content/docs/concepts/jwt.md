---
title: JWT in SupaLite
description: How JWTs flow from sign-in through PostgREST and into RLS policies.
---

JSON Web Tokens are the connective tissue between your frontend, GoTrue, PostgREST, and Postgres. Each token carries claims (`sub`, `email`, `role`, …) that identify the caller and tell Postgres what to allow.

## The three secret-bearing keys

| Token | Signed by | `role` claim | Lives where |
|---|---|---|---|
| `ANON_KEY` | `JWT_SECRET` | `anon` | Frontend code (safe to expose) |
| `SERVICE_ROLE_KEY` | `JWT_SECRET` | `service_role` | Server-only (never frontend) |
| User session JWTs | `JWT_SECRET` | `authenticated` (+ `sub`, `email`) | Issued by GoTrue per sign-in |

All three are HS256-signed with the **same** `JWT_SECRET`. PostgREST verifies any of them on incoming requests.

## The flow

```
  ┌──────────┐   email+password    ┌──────────┐
  │ frontend │ ───────────────────► │  GoTrue  │
  │          │ ◄─────────────────── │          │
  └──────────┘   user-session JWT   └──────────┘
       │                                  │
       │  fetch /rest/v1/todos            │
       │  apikey: ANON_KEY                │
       │  Authorization: Bearer <user JWT>│
       ▼                                  │
  ┌──────────┐                            │
  │  Caddy   │  require_apikey check      │
  └──────────┘                            │
       │                                  │
       ▼                                  │
  ┌──────────┐  set request.jwt.claims    │
  │PostgREST │ ─────────────────────────► │   Postgres
  └──────────┘                            │   auth.uid() reads claims
                                          ▼
                                       (RLS policies)
```

1. User signs in. GoTrue returns a JWT.
2. Frontend calls `/rest/v1/...` with **two** headers:
   - `apikey: <ANON_KEY>` (Caddy uses this to gate the path)
   - `Authorization: Bearer <user JWT>` (PostgREST uses this to identify the user)
3. PostgREST verifies the JWT using `JWT_SECRET`.
4. PostgREST sets the `request.jwt.claims` GUC inside the Postgres session.
5. RLS policies read `auth.uid()` (which itself reads `request.jwt.claims->>sub`).

## Why two headers?

`apikey` is **path authorization** — Caddy rejects any request to `/rest/*` or `/graphql/*` without a valid `apikey`. Prevents random internet scanners from hitting your DB.

`Authorization: Bearer` is **user identity** — PostgREST decodes it and gives Postgres the per-user claims for RLS.

A request from an anonymous (not-signed-in) user has only `apikey: <ANON_KEY>` and no `Authorization` header. PostgREST treats them as the `anon` role; RLS policies for `anon` (if any) apply.

## What's in a user JWT

GoTrue mints (HS256, 1-hour default expiry):

```json
{
  "sub": "uuid-of-the-user",
  "email": "user@example.com",
  "role": "authenticated",
  "aud": "authenticated",
  "exp": 1700003600,
  "iat": 1700000000,
  "app_metadata": { ... },
  "user_metadata": { ... }
}
```

Custom claims via `app_metadata` (admin-set, immutable from client) and `user_metadata` (user-editable). For tenant_id-style multi-tenancy, put it in `app_metadata`.

## Token refresh

GoTrue issues a refresh token alongside the access JWT. `supabase-js` handles refresh transparently — when the access token's about to expire, it POSTs the refresh token to `/auth/v1/token?grant_type=refresh_token` and gets a new pair.

If `JWT_SECRET` rotates (via the Secrets page), the refresh token from the OLD secret can't mint a new access JWT under the NEW secret. Users get logged out and have to sign in again.

## Why HS256 (not RS256)?

Symmetric signing with `JWT_SECRET` keeps the key management trivial — one secret, four services share it. RS256 would let PostgREST verify with only the public key, but in a self-hosted single-instance deployment there's no third-party verifier so the asymmetric guarantee buys nothing.

If you ever need RS256 (e.g., to share JWT verification with a SaaS service), GoTrue and PostgREST both support it — set `GOTRUE_JWT_ALGORITHM=RS256` and provide a key pair.

## Debugging JWT issues

1. **Decode the token** — `jwt.io` or `jq -R` on the middle segment. Check `exp` (expired?), `role` (right one?), `sub` (UUID?).
2. **Wrong signature**: PostgREST returns `401 invalid_token`. Means the JWT was signed with a different `JWT_SECRET` than PostgREST has.
3. **Missing `apikey`**: Caddy returns `401 Missing or invalid API key`. Add `apikey: <ANON_KEY>` header.
4. **RLS blocking valid request**: Run the same query as `service_role` (bypasses RLS) to confirm data exists; the policy is the issue.
