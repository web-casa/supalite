package handler

import (
	"context"
	"net/http"
	"time"
)

// Session mirrors the columns we expose from pg_stat_activity.
// State/query may be null/empty for idle/system backends.
type Session struct {
	PID             int     `json:"pid"`
	Username        string  `json:"username"`
	ApplicationName string  `json:"application_name"`
	ClientAddr      *string `json:"client_addr"`
	DatabaseName    string  `json:"database_name"`
	State           string  `json:"state"`
	WaitEventType   *string `json:"wait_event_type"`
	WaitEvent       *string `json:"wait_event"`
	BackendStart    string  `json:"backend_start"`
	QueryStart      *string `json:"query_start"`
	Query           string  `json:"query"`
	BackendType     string  `json:"backend_type"`
}

// HandleSessions lists currently connected backends.
//
// We hide background workers (autovacuum launcher, walwriter etc.)
// by default — they're noise for a DBA trying to spot client activity.
// Pass ?system=true to include them.
func (d *Deps) HandleSessions(w http.ResponseWriter, r *http.Request) {
	includeSystem := r.URL.Query().Get("system") == "true"

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	query := `
		SELECT
			pid,
			COALESCE(usename, ''),
			COALESCE(application_name, ''),
			client_addr::text,
			COALESCE(datname, ''),
			COALESCE(state, ''),
			wait_event_type,
			wait_event,
			backend_start::text,
			query_start::text,
			COALESCE(query, ''),
			COALESCE(backend_type, '')
		FROM pg_stat_activity
		WHERE ($1 OR backend_type = 'client backend')
		  AND pid <> pg_backend_pid()
		ORDER BY backend_start DESC
	`
	rows, err := d.DB.RW.Query(ctx, query, includeSystem)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer rows.Close()

	sessions := make([]Session, 0, 32)
	for rows.Next() {
		var s Session
		if err := rows.Scan(
			&s.PID, &s.Username, &s.ApplicationName, &s.ClientAddr,
			&s.DatabaseName, &s.State, &s.WaitEventType, &s.WaitEvent,
			&s.BackendStart, &s.QueryStart, &s.Query, &s.BackendType,
		); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"sessions": sessions})
}

type terminateRequest struct {
	PID int `json:"pid"`
}

// HandleSessionsTerminate sends pg_terminate_backend to the given PID.
// Guards:
//   - pid > 0
//   - must not be our own backend (pg_terminate_backend(pg_backend_pid())
//     would kill the session we're using to run this very query)
func (d *Deps) HandleSessionsTerminate(w http.ResponseWriter, r *http.Request) {
	var req terminateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.PID <= 0 {
		writeError(w, 400, "pid must be > 0")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var terminated bool
	err := d.DB.RW.QueryRow(ctx, `
		SELECT CASE
			WHEN $1 = pg_backend_pid() THEN false
			ELSE pg_terminate_backend($1)
		END
	`, req.PID).Scan(&terminated)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"terminated": terminated, "pid": req.PID})
}
