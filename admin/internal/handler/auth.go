package handler

import (
	"crypto/subtle"
	"net/http"

	"github.com/supalite/admin/internal/cookie"
)

// HandleAuthVerify is the login endpoint. It receives `Authorization: Bearer
// <admin_token>`, validates it (constant-time compare), and on success sets
// an HMAC session cookie.
//
// This handler intentionally does NOT go through the global auth middleware —
// it IS the authentication point. Rate limiting (server.rateLimitAuth wrapper)
// prevents brute force.
func (d *Deps) HandleAuthVerify(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	expected := "Bearer " + d.AdminToken
	if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
		writeError(w, 401, "unauthorized")
		return
	}
	if d.CookieSigner != nil {
		if err := d.CookieSigner.Set(w, d.CookieSecure()); err != nil {
			writeError(w, 500, "failed to set cookie: "+err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// HandleAuthLogout clears the session cookie.
func (d *Deps) HandleAuthLogout(w http.ResponseWriter, r *http.Request) {
	cookie.Clear(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
