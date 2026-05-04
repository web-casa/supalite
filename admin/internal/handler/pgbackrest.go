package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/supalite/admin/internal/docker"
)

// stanzaCreateRunning serializes stanza-create so spam-clicking
// "Initialize" doesn't launch parallel execs. pgBackRest's own
// repo lock would mostly handle it, but we want predictable UX.
var stanzaCreateRunning atomic.Bool

// Backup execution runs asynchronously because pgBackRest backups
// can take hours on large databases, and we don't want to pin an
// HTTP connection for that long. Clients kick off via POST and poll
// GET /status.
//
// One pgBackRest long-op at a time per admin process. `backupMu`
// guards BOTH backup and restore state; trying to start either while
// the other is running returns 409. Restore orchestration also stops
// the db container, which would abort any in-flight backup anyway.
var (
	backupMu       sync.Mutex
	backupRunning  bool
	backupState    backupRunStatus
	pgbrRestoreRunning bool
	pgbrRestoreState   restoreRunStatus
)

// restoreLabelRe matches pgBackRest backup labels. Full backups are
// YYYYMMDD-HHMMSSF, incremental/differential chains have the base
// label + `_YYYYMMDD-HHMMSSD` (diff) or `I` (incr) appended.
var restoreLabelRe = regexp.MustCompile(`^\d{8}-\d{6}[FDI](_\d{8}-\d{6}[DI])*$`)

type backupRunStatus struct {
	Type     string    `json:"type,omitempty"`
	Started  time.Time `json:"started,omitempty"`
	Finished time.Time `json:"finished,omitempty"`
	OK       bool      `json:"ok"`
	ExitCode int       `json:"exit_code"`
	Stderr   string    `json:"stderr,omitempty"`
	Duration string    `json:"duration,omitempty"`
}

// restoreRunStatus tracks the orchestrated restore state machine.
// Phase progresses: idle → stopping → restoring → starting → done|error.
type restoreRunStatus struct {
	Phase    string    `json:"phase"`           // see above
	Set      string    `json:"set,omitempty"`   // backup label requested
	Started  time.Time `json:"started,omitempty"`
	Finished time.Time `json:"finished,omitempty"`
	OK       bool      `json:"ok"`
	Error    string    `json:"error,omitempty"`
	Stderr   string    `json:"stderr,omitempty"`
	Duration string    `json:"duration,omitempty"`
}

// restoreTimeout caps the full stop→restore→start orchestration.
const restoreTimeout = 4 * time.Hour

// restoreOneShotLabelKey / Value label our restore containers so
// orphans left behind by an admin crash/restart can be detected.
const (
	restoreOneShotLabelKey   = "com.supalite.pgbackrest.op"
	restoreOneShotLabelValue = "restore"
)

// backupTimeout caps a single backup at 4 hours. Enough for a few
// hundred GB on typical hardware; larger installs should switch to
// running pgbackrest directly from a scheduled job.
const backupTimeout = 4 * time.Hour

var validBackupTypes = map[string]bool{
	"full": true, "diff": true, "incr": true,
}

// Phase 6.2a scope: two read/init endpoints only. Backup execution
// (full/diff/incr) and restore are deferred to 6.2b/c so the operator
// gets visibility + initialization this round without a destructive
// surface yet.

// HandlePgbackrestInfo runs `pgbackrest info --stanza=main --output=json`
// inside the db container and returns the parsed JSON. If the stanza
// hasn't been created yet, pgBackRest prints `[]` and exits 0 — we
// pass that through so the UI can detect "not initialized" state.
func (d *Deps) HandlePgbackrestInfo(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	res, err := d.Docker.Exec(ctx, "db",
		[]string{"pgbackrest", "--stanza=main", "--output=json", "info"},
		256*1024,
	)
	if err != nil {
		writeError(w, 500, "exec: "+err.Error())
		return
	}
	if res.ExitCode != 0 {
		writeJSON(w, 500, map[string]any{
			"ok":        false,
			"exit_code": res.ExitCode,
			"stderr":    tailString(res.Stderr, 4096),
		})
		return
	}

	// pgbackrest emits a JSON array. Parse into a generic structure so
	// the frontend can treat it as data, and so a stray trailing newline
	// doesn't confuse the UI.
	var parsed any
	if err := json.Unmarshal([]byte(strings.TrimSpace(res.Stdout)), &parsed); err != nil {
		writeError(w, 500, "parse info: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"ok":   true,
		"info": parsed,
	})
}

// HandlePgbackrestStanzaCreate runs `pgbackrest stanza-create` against
// the `main` stanza. Idempotent per pgBackRest — running twice on an
// already-created stanza returns success with a notice.
//
// Preconditions (enforced only by pgBackRest itself, surfaced in stderr):
//   - BACKUP_S3_* env vars set in .env
//   - PGBACKREST_ARCHIVE_MODE=on and archive_command set to pgbackrest
//   - Postgres has been restarted since those env changes
func (d *Deps) HandlePgbackrestStanzaCreate(w http.ResponseWriter, r *http.Request) {
	if !stanzaCreateRunning.CompareAndSwap(false, true) {
		writeError(w, 409, "stanza-create already in progress")
		return
	}
	defer stanzaCreateRunning.Store(false)

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	res, err := d.Docker.Exec(ctx, "db",
		[]string{"pgbackrest", "--stanza=main", "stanza-create"},
		64*1024,
	)
	if err != nil {
		writeError(w, 500, "exec: "+err.Error())
		return
	}
	if res.ExitCode != 0 {
		writeJSON(w, 500, map[string]any{
			"ok":        false,
			"exit_code": res.ExitCode,
			"stdout":    tailString(res.Stdout, 4096),
			"stderr":    tailString(res.Stderr, 4096),
		})
		return
	}
	writeJSON(w, 200, map[string]any{
		"ok":     true,
		"stdout": tailString(res.Stdout, 4096),
		"stderr": tailString(res.Stderr, 4096),
	})
}

type pgbackrestBackupRequest struct {
	Type string `json:"type"` // full | diff | incr
}

// HandlePgbackrestBackup kicks off a backup asynchronously. Returns
// 202 immediately so the client can poll GET /status. Only one
// backup runs per process at a time (guarded by backupMu).
//
// Detached ctx: we use context.Background() for the subprocess so
// an admin container restart doesn't abort a running backup (the
// exec is happening inside the db container regardless). If admin
// restarts during the run, backupRunning resets to false and we lose
// visibility — but pgBackRest's own repo lock still prevents a new
// backup from racing the one still in progress.
func (d *Deps) HandlePgbackrestBackup(w http.ResponseWriter, r *http.Request) {
	var req pgbackrestBackupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validBackupTypes[req.Type] {
		writeError(w, 400, "invalid type: expected full|diff|incr")
		return
	}

	backupMu.Lock()
	if backupRunning {
		backupMu.Unlock()
		writeError(w, 409, "a backup is already running")
		return
	}
	if pgbrRestoreRunning {
		backupMu.Unlock()
		writeError(w, 409, "a restore is in progress — cannot start a backup")
		return
	}
	backupRunning = true
	backupState = backupRunStatus{
		Type:    req.Type,
		Started: time.Now().UTC(),
	}
	backupMu.Unlock()

	backupType := req.Type
	docker := d.Docker
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), backupTimeout)
		defer cancel()

		start := time.Now()
		res, err := docker.Exec(ctx, "db",
			[]string{"pgbackrest", "--stanza=main", "--type=" + backupType, "backup"},
			256*1024,
		)
		elapsed := time.Since(start)

		backupMu.Lock()
		defer backupMu.Unlock()
		backupRunning = false
		backupState.Finished = time.Now().UTC()
		backupState.Duration = elapsed.String()
		if err != nil {
			backupState.OK = false
			msg := err.Error()
			// Clarify the common timeout case: admin gave up waiting,
			// but pgBackRest may still be running inside the db
			// container. A later info call will reveal the truth.
			if ctx.Err() == context.DeadlineExceeded {
				msg = "admin stopped waiting after " + backupTimeout.String() +
					"; backup may still be running in db container — check info in a few minutes: " + msg
			}
			backupState.Stderr = tailString(msg, 4096)
			log.Printf("[pgbackrest] %s backup exec error: %v", backupType, err)
			return
		}
		backupState.ExitCode = res.ExitCode
		backupState.OK = res.ExitCode == 0
		// Keep stderr tail regardless of exit code — pgBackRest logs
		// useful progress + warnings even on success.
		backupState.Stderr = tailString(res.Stderr, 4096)
		if res.ExitCode != 0 {
			log.Printf("[pgbackrest] %s backup failed (exit %d): %s",
				backupType, res.ExitCode, backupState.Stderr)
		} else {
			log.Printf("[pgbackrest] %s backup ok in %s", backupType, elapsed)
		}
	}()

	writeJSON(w, 202, map[string]any{
		"accepted": true,
		"type":     req.Type,
	})
}

// HandlePgbackrestStatus returns current running state + last result.
// Frontend polls this while running.
//
// We snapshot under the lock and release BEFORE encoding the response,
// so slow clients don't hold backupMu during socket I/O (which would
// block incoming backup/status requests).
func (d *Deps) HandlePgbackrestStatus(w http.ResponseWriter, r *http.Request) {
	backupMu.Lock()
	running := backupRunning
	// Copy by value — backupRunStatus contains no pointers/slices/maps,
	// so this is a full independent snapshot safe to use after Unlock.
	last := backupState
	hasLast := !backupState.Started.IsZero()
	backupMu.Unlock()

	resp := map[string]any{"running": running}
	if hasLast {
		resp["last"] = last
	}
	writeJSON(w, 200, resp)
}

type pgbackrestRestoreRequest struct {
	Set string `json:"set"` // backup label to restore from (required)
}

// HandlePgbackrestRestore orchestrates a full destructive restore:
//
//	1. Inspect the running db container to capture Image, Mounts, Networks.
//	2. Stop db (SIGTERM, 60s grace).
//	3. Run a one-shot container with the SAME image + data volume + env,
//	   entrypoint overridden to: pgbackrest --stanza=main --set=<set> restore.
//	4. On success: start db back up.
//	5. On any failure: leave db stopped; operator must investigate.
//
// Asynchronous (returns 202 immediately); client polls GET /restore/status.
// DESTRUCTIVE: caller has already confirmed via typed-label in the UI.
func (d *Deps) HandlePgbackrestRestore(w http.ResponseWriter, r *http.Request) {
	var req pgbackrestRestoreRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Set = strings.TrimSpace(req.Set)
	if !restoreLabelRe.MatchString(req.Set) {
		writeError(w, 400, "invalid backup label")
		return
	}

	// Orphan check: if a previous admin crash left a labeled one-shot
	// running (still writing pg_data), starting another restore or
	// touching the db would corrupt the volume. Block until operator
	// resolves manually.
	orphanCtx, orphanCancel := context.WithTimeout(r.Context(), 10*time.Second)
	orphans, orphanErr := d.Docker.ListContainersByLabel(orphanCtx,
		restoreOneShotLabelKey, restoreOneShotLabelValue)
	orphanCancel()
	if orphanErr != nil {
		writeError(w, 500, "orphan check failed: "+orphanErr.Error())
		return
	}
	if len(orphans) > 0 {
		writeError(w, 409,
			"a prior restore container is still present ("+orphans[0][:12]+
				"); inspect and remove with `docker rm -f "+orphans[0][:12]+"` before retrying")
		return
	}

	backupMu.Lock()
	if pgbrRestoreRunning {
		backupMu.Unlock()
		writeError(w, 409, "a restore is already running")
		return
	}
	if backupRunning {
		backupMu.Unlock()
		writeError(w, 409, "a backup is running — cannot start restore")
		return
	}
	pgbrRestoreRunning = true
	pgbrRestoreState = restoreRunStatus{
		Phase:   "stopping",
		Set:     req.Set,
		Started: time.Now().UTC(),
	}
	backupMu.Unlock()

	dockerClient := d.Docker
	set := req.Set

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), restoreTimeout)
		defer cancel()

		start := time.Now()
		finish := func(ok bool, phase, errMsg, stderr string) {
			backupMu.Lock()
			defer backupMu.Unlock()
			pgbrRestoreRunning = false
			pgbrRestoreState.Phase = phase
			pgbrRestoreState.OK = ok
			pgbrRestoreState.Error = tailString(errMsg, 4096)
			pgbrRestoreState.Stderr = tailString(stderr, 4096)
			pgbrRestoreState.Finished = time.Now().UTC()
			pgbrRestoreState.Duration = time.Since(start).String()
			if ok {
				log.Printf("[pgbackrest] restore from %s ok in %s", set, pgbrRestoreState.Duration)
			} else {
				log.Printf("[pgbackrest] restore from %s FAILED at %s: %s",
					set, phase, errMsg)
			}
		}
		setPhase := func(p string) {
			backupMu.Lock()
			pgbrRestoreState.Phase = p
			backupMu.Unlock()
		}

		// 1. Inspect db container for Image / Binds / Network.
		dbID, err := dockerClient.FindContainerID("db")
		if err != nil {
			finish(false, "error", "find db: "+err.Error(), "")
			return
		}
		inspect, err := dockerClient.InspectContainer(ctx, dbID)
		if err != nil {
			finish(false, "error", "inspect db: "+err.Error(), "")
			return
		}
		image, binds, env, netMode, err := extractRestoreSpec(inspect)
		if err != nil {
			finish(false, "error", err.Error(), "")
			return
		}

		// 2. Stop db.
		log.Printf("[pgbackrest] restore: stopping db container %s", dbID[:12])
		if err := dockerClient.StopContainer(ctx, dbID, 60); err != nil {
			finish(false, "error", "stop db: "+err.Error(), "")
			return
		}

		// 3. One-shot pgbackrest restore.
		setPhase("restoring")
		log.Printf("[pgbackrest] restore: running pgbackrest --set=%s", set)
		res, err := dockerClient.RunOneShot(ctx, docker.OneShotConfig{
			Image:       image,
			Cmd:         []string{"pgbackrest", "--stanza=main", "--set=" + set, "--delta", "restore"},
			Env:         env,
			Binds:       binds,
			NetworkMode: netMode,
			MaxLogBytes: 256 * 1024,
			Labels: map[string]string{
				restoreOneShotLabelKey: restoreOneShotLabelValue,
			},
		})
		if err != nil {
			finish(false, "error", "restore exec: "+err.Error(), "")
			return
		}
		if res.ExitCode != 0 {
			finish(false, "error",
				fmt.Sprintf("pgbackrest restore exited %d", res.ExitCode),
				res.Stderr,
			)
			return
		}

		// Post-run orphan verification: if RunOneShot's deferred
		// force-remove failed (Docker daemon trouble mid-stop), a
		// container might still be holding the data volume. Starting
		// db now would mean two writers on the same files.
		orphanCheckCtx, orphanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		remaining, orphanErr := dockerClient.ListContainersByLabel(orphanCheckCtx,
			restoreOneShotLabelKey, restoreOneShotLabelValue)
		orphanCancel()
		if orphanErr != nil || len(remaining) > 0 {
			msg := "restore ran but one-shot container was not removed; refusing to start db"
			if orphanErr != nil {
				msg += " (orphan check errored: " + orphanErr.Error() + ")"
			}
			finish(false, "error", msg, res.Stderr)
			return
		}

		// 4. Start db back up.
		setPhase("starting")
		log.Printf("[pgbackrest] restore: starting db container")
		if err := dockerClient.StartContainer(ctx, dbID); err != nil {
			finish(false, "error",
				"restore succeeded but failed to start db: "+err.Error(),
				res.Stderr,
			)
			return
		}

		finish(true, "done", "", res.Stderr)
	}()

	writeJSON(w, 202, map[string]any{"accepted": true, "set": req.Set})
}

// HandlePgbackrestRestoreStatus returns the current restore state.
// Snapshot under lock, encode after release — same pattern as backup.
func (d *Deps) HandlePgbackrestRestoreStatus(w http.ResponseWriter, r *http.Request) {
	backupMu.Lock()
	running := pgbrRestoreRunning
	state := pgbrRestoreState
	hasState := !pgbrRestoreState.Started.IsZero()
	backupMu.Unlock()

	resp := map[string]any{"running": running}
	if hasState {
		resp["state"] = state
	}
	writeJSON(w, 200, resp)
}

// extractRestoreSpec pulls the image, binds, env, and network from a
// `GET /containers/{id}/json` response. Only the data-volume bind is
// reused — the init-script bind is irrelevant in tool mode.
func extractRestoreSpec(inspect map[string]any) (image string, binds []string, env []string, netMode string, err error) {
	// Config.Image (e.g. "supalite-db:latest")
	if cfg, ok := inspect["Config"].(map[string]any); ok {
		if s, ok := cfg["Image"].(string); ok {
			image = s
		}
		if list, ok := cfg["Env"].([]any); ok {
			for _, v := range list {
				if s, ok := v.(string); ok {
					// Carry only what the pgbackrest entrypoint needs to
					// render its config. Filters stale values (e.g. stale
					// POSTGRES_PASSWORD that might leak into logs).
					if strings.HasPrefix(s, "BACKUP_S3_") ||
						strings.HasPrefix(s, "PGBACKREST_") {
						env = append(env, s)
					}
				}
			}
		}
	}
	if image == "" {
		return "", nil, nil, "", fmt.Errorf("could not determine db image from inspect")
	}

	// Mounts — pick the one destined at the postgres data directory.
	if mounts, ok := inspect["Mounts"].([]any); ok {
		for _, m := range mounts {
			mm, ok := m.(map[string]any)
			if !ok {
				continue
			}
			dst, _ := mm["Destination"].(string)
			if dst != "/var/lib/postgresql/data" {
				continue
			}
			// Prefer Name (volume) over Source (bind path).
			name, _ := mm["Name"].(string)
			src, _ := mm["Source"].(string)
			ref := name
			if ref == "" {
				ref = src
			}
			if ref != "" {
				binds = append(binds, ref+":/var/lib/postgresql/data")
			}
		}
	}
	if len(binds) == 0 {
		return "", nil, nil, "", fmt.Errorf("could not find db-data mount on db container")
	}

	// NetworkMode — prefer HostConfig.NetworkMode (deterministic,
	// matches what compose requested) over NetworkSettings.Networks
	// (runtime, map-order nondeterministic).
	if hc, ok := inspect["HostConfig"].(map[string]any); ok {
		if s, ok := hc["NetworkMode"].(string); ok && s != "" && s != "default" {
			netMode = s
		}
	}
	if netMode == "" {
		if ns, ok := inspect["NetworkSettings"].(map[string]any); ok {
			if nets, ok := ns["Networks"].(map[string]any); ok {
				for key := range nets {
					netMode = key
					break
				}
			}
		}
	}
	// NetworkMode may legitimately be empty (host net, no-net) — don't
	// fail if so; Docker defaults to "default".

	return image, binds, env, netMode, nil
}
