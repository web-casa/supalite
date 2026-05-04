// API client.
//
// Auth model (Phase 2):
//   1. Login form POSTs /api/auth/verify with `Authorization: Bearer <admin_token>`.
//   2. On success, server sets an HttpOnly HMAC cookie (sbl_auth).
//   3. Subsequent requests rely on the cookie only — we no longer send Bearer.
//
// Why not keep Bearer? Because HttpOnly cookies can't be stolen by XSS,
// and EventSource / Studio (cross-origin-ish) can't set Authorization headers
// but do send cookies automatically. One auth mechanism for everything.
//
// Login token is still cached in sessionStorage only to enable auto-login
// on page refresh during a session. It is NEVER sent on regular API calls
// after the initial login handshake — only on /api/auth/verify.

const API_BASE = "/admin/api";

/**
 * ApiError carries both the human-readable message and the HTTP
 * status, so callers can branch on status without parsing text
 * (e.g. `if (err instanceof ApiError && err.status === 409) …`).
 */
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
    public readonly data?: unknown,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/**
 * Extract a user-facing message from any thrown value. Covers the
 * common catch-block pattern `e instanceof Error ? e.message : "…"`
 * plus the new ApiError path.
 */
export function errorMessage(e: unknown, fallback = "request failed"): string {
  if (e instanceof Error) return e.message;
  if (typeof e === "string") return e;
  return fallback;
}

function getLoginToken(): string {
  if (typeof window === "undefined") return "";
  return sessionStorage.getItem("admin_token") || "";
}

export function setToken(token: string) {
  sessionStorage.setItem("admin_token", token);
}

export function clearToken() {
  sessionStorage.removeItem("admin_token");
}

export function hasToken(): boolean {
  if (typeof window === "undefined") return false;
  return !!sessionStorage.getItem("admin_token");
}

type RequestOpts = {
  /**
   * Set to true ONLY on the login call to /api/auth/verify. This attaches
   * the Authorization header. On all other calls we rely on the HttpOnly
   * cookie that was set by the login response.
   */
  loginHandshake?: boolean;
};

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  opts: RequestOpts = {}
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (opts.loginHandshake) {
    headers["Authorization"] = `Bearer ${getLoginToken()}`;
  }
  const init: RequestInit = {
    method,
    headers,
    credentials: "same-origin", // send cookies
  };
  if (body !== undefined) {
    init.body = JSON.stringify(body);
  }
  const res = await fetch(`${API_BASE}${path}`, init);
  if (res.status === 401) {
    if (opts.loginHandshake) {
      // Bad token during login — let the login form show the error.
      // Don't reload, don't clear token (caller may want to keep the
      // attempted value in the input for retry).
      let msg = "Invalid admin token";
      try {
        const data = await res.json();
        msg = data.error || msg;
      } catch {
        /* ignore parse errors */
      }
      throw new Error(msg);
    }
    // Session expired on a regular call — force re-login.
    clearToken();
    window.location.reload();
    throw new Error("unauthorized");
  }
  const data = await res.json();
  if (!res.ok) {
    throw new ApiError(res.status, data?.error || `HTTP ${res.status}`, data);
  }
  return data as T;
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body?: unknown) => request<T>("POST", path, body),
  del: <T>(path: string) => request<T>("DELETE", path),
  /** Login handshake: sends Bearer + sets cookie on success. */
  login: <T>(path: string, body?: unknown) =>
    request<T>("POST", path, body, { loginHandshake: true }),
  /**
   * Open a Server-Sent Events stream against /admin/api/<path>.
   * Cookies are sent automatically (same-origin). Caller is responsible
   * for closing the returned EventSource.
   */
  sse: (path: string) => new EventSource(`${API_BASE}${path}`),
};
