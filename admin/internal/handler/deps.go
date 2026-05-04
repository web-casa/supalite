package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/supalite/admin/internal/cookie"
	"github.com/supalite/admin/internal/db"
	"github.com/supalite/admin/internal/docker"
)

type Deps struct {
	EnvFile        string
	DB             *db.Pools
	Docker         *docker.Client
	AdminToken     string
	CookieSigner   *cookie.Signer
	APIExternalURL string // for deriving cookie Secure flag
}

// CookieSecure returns true if the external URL uses https scheme.
// Used to set Secure flag on cookies when deployed under TLS.
func (d *Deps) CookieSecure() bool {
	return strings.HasPrefix(strings.ToLower(d.APIExternalURL), "https://")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decodeJSON decodes r.Body into dst, writing a 400 response and
// returning false if the body is malformed. Callers validate field
// contents themselves after a successful decode. `dst` is `any`
// because json.Decoder.Decode takes `any`; a generic `*T` added no
// safety since we don't inspect T.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, 400, "invalid JSON")
		return false
	}
	return true
}
