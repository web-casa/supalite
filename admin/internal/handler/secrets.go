package handler

import (
	"net/http"

	"github.com/supalite/admin/internal/secrets"
)

// HandleSecretsCatalog returns the static metadata for each rotatable
// secret — keys, descriptions, consequences, services that need restart
// after rotation. The frontend renders a card per entry.
func (d *Deps) HandleSecretsCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"secrets": secrets.Catalog()})
}

type secretRotateRequest struct {
	Key string `json:"key"`
}

// HandleSecretRotate generates new value(s) for the requested secret,
// writes them atomically into .env, and returns the side-effect
// summary. The handler does NOT auto-restart services — operator
// confirms via Settings → Restart Services after seeing what changed.
func (d *Deps) HandleSecretRotate(w http.ResponseWriter, r *http.Request) {
	var req secretRotateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := secrets.Rotate(req.Key, d.EnvFile)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"ok":      true,
		"updated": res.Updated,
		"restart": res.Restart,
		"notes":   res.Notes,
	})
}
