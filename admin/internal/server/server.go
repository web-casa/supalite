package server

import (
	"crypto/subtle"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/supalite/admin/internal/cookie"
	"github.com/supalite/admin/internal/handler"
	"github.com/supalite/admin/internal/ratelimit"
)

// authLimiter gates /api/auth/verify. 10 attempts per IP per minute is
// enough for humans (including fat-finger retries) but stops brute force.
var authLimiter = ratelimit.New(10, 1*time.Minute)

func New(deps *handler.Deps, staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()

	// Go 1.22 method-prefix patterns: most wrong-method requests get a
	// 404 JSON from spaHandler's `/api/` fallback branch, not a 405 —
	// the "/" catch-all matches every method, which suppresses mux's
	// built-in 405. That's fine for us (admin UI never sends wrong
	// methods) and keeps error bodies in JSON. Handlers dispatched by
	// method pattern also accept HEAD for GET by HTTP convention —
	// HandleConfig explicitly maps HEAD → getConfig for that reason.

	// Auth endpoints — rate limited, special bearer-only semantics.
	mux.HandleFunc("POST /api/auth/verify", rateLimitAuth(deps.HandleAuthVerify))
	mux.HandleFunc("POST /api/auth/logout", deps.HandleAuthLogout)

	// SMTP diagnostic (Phase 7.1)
	mux.HandleFunc("POST /api/auth/smtp-test", deps.HandleSMTPTest)

	// OAuth credential probes (Phase 7.2) — github / google only.
	mux.HandleFunc("POST /api/auth/oauth-test", deps.HandleOAuthTest)

	// Config: single handler covers both verbs, dispatched internally.
	mux.HandleFunc("GET /api/config", deps.HandleConfig)
	mux.HandleFunc("POST /api/config", deps.HandleConfig)

	mux.HandleFunc("GET /api/keys", deps.HandleKeys)
	mux.HandleFunc("POST /api/keys/service_role", deps.HandleServiceRoleKey)
	mux.HandleFunc("GET /api/status", deps.HandleStatus)
	mux.HandleFunc("POST /api/restart", deps.HandleRestart)
	mux.HandleFunc("GET /api/logs", deps.HandleLogs)
	mux.HandleFunc("GET /api/logs/stream", deps.HandleLogsStream)
	mux.HandleFunc("GET /api/status/stream", deps.HandleStatusStream)

	// DBA ops (Phase 5)
	mux.HandleFunc("POST /api/dbops/maintenance", deps.HandleDBOpsMaintenance)
	mux.HandleFunc("GET /api/sessions", deps.HandleSessions)
	mux.HandleFunc("POST /api/sessions/terminate", deps.HandleSessionsTerminate)

	// Secret rotation
	mux.HandleFunc("GET /api/secrets", deps.HandleSecretsCatalog)
	mux.HandleFunc("POST /api/secrets/rotate", deps.HandleSecretRotate)

	// Backups (Phase 6.1 — pg_dump to S3)
	mux.HandleFunc("POST /api/backup/run", deps.HandleBackupRun)
	mux.HandleFunc("GET /api/backup/list", deps.HandleBackupList)
	mux.HandleFunc("POST /api/backup/delete", deps.HandleBackupDelete)
	mux.HandleFunc("GET /api/backup/download", deps.HandleBackupDownload)
	mux.HandleFunc("POST /api/backup/restore", deps.HandleBackupRestore)

	// pgBackRest (Phase 6.2a/b — info + stanza init + backup execution)
	mux.HandleFunc("GET /api/pgbackrest/info", deps.HandlePgbackrestInfo)
	mux.HandleFunc("GET /api/pgbackrest/status", deps.HandlePgbackrestStatus)
	mux.HandleFunc("POST /api/pgbackrest/stanza-create", deps.HandlePgbackrestStanzaCreate)
	mux.HandleFunc("POST /api/pgbackrest/backup", deps.HandlePgbackrestBackup)
	// Restore orchestration (Phase 6.2c) — destructive.
	mux.HandleFunc("POST /api/pgbackrest/restore", deps.HandlePgbackrestRestore)
	mux.HandleFunc("GET /api/pgbackrest/restore/status", deps.HandlePgbackrestRestoreStatus)

	mux.Handle("/", spaHandler(staticFS))

	return authMiddleware(deps.AdminToken, deps.CookieSigner, mux)
}

// authMiddleware gates regular /api/* requests. Accepts either:
//   1. Authorization: Bearer <ADMIN_TOKEN>  — used by scripts / CLI
//   2. Cookie: sbl_auth=<signed>            — used after login (SSE, Studio, UI)
//
// Notable bypasses:
//   /api/auth/verify  — login endpoint, has its own rate-limited bearer check
//   /api/auth/logout  — anyone can ask to clear their cookie
func authMiddleware(token string, signer *cookie.Signer, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Login / logout endpoints bypass this middleware.
		if r.URL.Path == "/api/auth/verify" || r.URL.Path == "/api/auth/logout" {
			next.ServeHTTP(w, r)
			return
		}

		// 1. Try Authorization: Bearer <token>
		if auth := r.Header.Get("Authorization"); auth != "" {
			expected := "Bearer " + token
			if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
			// Bad Bearer — don't fall through to cookie (attacker might have both)
			unauthorized(w)
			return
		}

		// 2. Try sbl_auth cookie
		if signer != nil {
			if c, err := r.Cookie(cookie.Name); err == nil && c.Value != "" {
				if signer.Verify(c.Value) == nil {
					next.ServeHTTP(w, r)
					return
				}
			}
		}

		unauthorized(w)
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(401)
	w.Write([]byte(`{"error":"unauthorized"}`))
}

// rateLimitAuth wraps the auth verify handler with per-IP rate limiting.
// The limiter is reset on successful auth (via RateLimitResetCookie header).
func rateLimitAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := ratelimit.ClientIP(r)
		if !authLimiter.Allow(ip) {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(429)
			w.Write([]byte(`{"error":"too many auth attempts, retry later"}`))
			return
		}
		// Intercept response to reset limiter on 200
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next(rec, r)
		if rec.status == 200 {
			authLimiter.Reset(ip)
		}
	}
}

// statusRecorder lets us observe the HTTP status after the handler runs.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(s int) {
	if !r.wrote {
		r.status = s
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(s)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.wrote = true // implicit 200
	}
	return r.ResponseWriter.Write(b)
}

// Flush passes through to the underlying ResponseWriter if it supports it,
// so SSE still works when wrapped.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// spaHandler serves static files from the embedded Next.js export with SPA
// fallback to index.html for client-side routes.
//
// We roll our own resolution instead of delegating to http.FileServer because
// that helper has a quirk: it 301-redirects any request whose path ends in
// "/index.html" to "./", which breaks our SPA fallback (we'd need to rewrite
// the URL to point at index.html, which then triggers the redirect).
func spaHandler(staticFS fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /api/* requests that fell through here (no registered handler)
		// should be 404, not fall back to SPA index.html.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			notFound(w)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/")

		// Normalize directory URLs ("/logs/" → look for "logs/index.html").
		switch {
		case path == "":
			path = "index.html"
		case strings.HasSuffix(path, "/"):
			path = path + "index.html"
		}

		// Try the exact file first.
		if data, err := fs.ReadFile(staticFS, path); err == nil {
			writeStatic(w, path, data)
			return
		}

		// If the path has no extension, try appending "/index.html"
		// (Next.js static export layout: /logs → logs/index.html).
		if !strings.Contains(path, ".") {
			indexPath := strings.TrimSuffix(path, "/") + "/index.html"
			if _, err := fs.Stat(staticFS, indexPath); err == nil {
				// Redirect to trailing-slash form so relative assets resolve.
				http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
				return
			}
		}

		// SPA fallback: serve root index.html for unknown client routes.
		if data, err := fs.ReadFile(staticFS, "index.html"); err == nil {
			writeStatic(w, "index.html", data)
			return
		}

		notFound(w)
	})
}

func writeStatic(w http.ResponseWriter, name string, data []byte) {
	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

func notFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(404)
	w.Write([]byte(`{"error":"not found"}`))
}

func ListenAndServe(addr string, h http.Handler) {
	log.Printf("[admin] listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatalf("[admin] server error: %v", err)
	}
}
