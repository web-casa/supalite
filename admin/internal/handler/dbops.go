package handler

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// identPattern matches a single unquoted Postgres identifier or a
// schema-qualified one: `foo` or `schema.foo`. Deliberately strict —
// no embedded quotes, no whitespace, no operators. Even though we
// additionally pgx-quote the parts before interpolation, validating
// the shape first means we fail fast on malformed input without ever
// touching the SQL layer.
var identPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$`)

type dbOpRequest struct {
	Op     string `json:"op"`     // vacuum | analyze | reindex
	Target string `json:"target"` // "" = whole database, "schema.table" otherwise
	Full   bool   `json:"full"`   // VACUUM only: rewrite table (acquires AccessExclusiveLock)
}

// HandleDBOpsMaintenance runs one of VACUUM / ANALYZE / REINDEX.
//
// Why a single endpoint instead of three? The shape of each is the same
// (verb + optional target), and grouping them simplifies both routing
// and the frontend form. If the operations diverge later (e.g. vacuum
// gets many flags), we can split then.
//
// Timeout: 10 minutes. Longer ops should probably be scheduled via a
// proper job runner (future work).
func (d *Deps) HandleDBOpsMaintenance(w http.ResponseWriter, r *http.Request) {
	var req dbOpRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	verb, err := buildMaintenanceSQL(req)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	start := time.Now()
	_, err = d.DB.RW.Exec(ctx, verb)
	elapsed := time.Since(start)
	if err != nil {
		writeJSON(w, 500, map[string]any{
			"ok":       false,
			"error":    err.Error(),
			"duration": elapsed.String(),
			"sql":      verb,
		})
		return
	}
	writeJSON(w, 200, map[string]any{
		"ok":       true,
		"duration": elapsed.String(),
		"sql":      verb,
	})
}

// buildMaintenanceSQL validates the request and returns a safe SQL string.
// We use pgx.Identifier for proper quoting of schema/table names; the
// regex up front is belt-and-suspenders against malformed input.
func buildMaintenanceSQL(req dbOpRequest) (string, error) {
	target := strings.TrimSpace(req.Target)
	var quoted string
	if target != "" {
		if !identPattern.MatchString(target) {
			return "", fmt.Errorf("invalid target identifier")
		}
		parts := strings.Split(target, ".")
		quoted = pgx.Identifier(parts).Sanitize()
	}

	switch req.Op {
	case "vacuum":
		if req.Full {
			if quoted == "" {
				return "VACUUM (FULL)", nil
			}
			return "VACUUM (FULL) " + quoted, nil
		}
		if quoted == "" {
			return "VACUUM", nil
		}
		return "VACUUM " + quoted, nil
	case "analyze":
		if quoted == "" {
			return "ANALYZE", nil
		}
		return "ANALYZE " + quoted, nil
	case "reindex":
		if quoted == "" {
			// REINDEX DATABASE requires the current DB name; hardcoded to
			// `postgres` since that's the only DB in our deployment.
			return "REINDEX DATABASE postgres", nil
		}
		// Heuristic: if the target includes a dot, treat as TABLE.
		// Schema-level REINDEX without a dot needs REINDEX SCHEMA; we
		// disallow that to keep the semantics unambiguous for now.
		if !strings.Contains(target, ".") {
			return "", fmt.Errorf("reindex requires schema.table; schema-level reindex not supported")
		}
		return "REINDEX TABLE " + quoted, nil
	default:
		return "", fmt.Errorf("unknown op: %s (expected vacuum|analyze|reindex)", req.Op)
	}
}
