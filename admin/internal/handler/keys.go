package handler

import (
	"net/http"

	"github.com/supalite/admin/internal/envfile"
)

func (d *Deps) HandleKeys(w http.ResponseWriter, r *http.Request) {
	env, err := envfile.Read(d.EnvFile)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	// service_role_key is NOT included here — it must be fetched via
	// /api/keys/service_role to minimize exposure on page loads.
	writeJSON(w, 200, map[string]string{
		"anon_key": env["ANON_KEY"],
		"api_url":  env["API_EXTERNAL_URL"],
	})
}

func (d *Deps) HandleServiceRoleKey(w http.ResponseWriter, r *http.Request) {
	env, err := envfile.Read(d.EnvFile)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{
		"service_role_key": env["SERVICE_ROLE_KEY"],
	})
}
