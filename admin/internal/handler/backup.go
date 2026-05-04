package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/supalite/admin/internal/backup"
)

// restoreRunning guards against concurrent destructive restores.
// A second request while one is in flight gets 409 Conflict instead
// of racing two pg_restore --clean processes against the live DB.
var restoreRunning atomic.Bool

// Backup names: letters, digits, dash, underscore, dot. No slashes
// (enforced in backup.ScopedKey too, but we fail earlier for a nicer
// error). Keep names short enough to fit in S3 key limits and tidy UI.
var backupNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// pgDumpTimeout caps one backup at 30 min. Very large databases may
// exceed this; for those the operator should switch to pgBackRest
// (Phase 6.2).
const pgDumpTimeout = 30 * time.Minute

// HandleBackupRun runs pg_dump and streams it into S3.
//
// Streaming design: we use an io.Pipe so pg_dump's stdout feeds the
// S3 multipart uploader directly — no temp file, no memory buffer,
// works for arbitrarily large databases.
//
// On pg_dump failure we still return a 500 and try to capture stderr
// for the error body. If the upload succeeded with partial data before
// pg_dump died, the partial object is left in the bucket but its size
// will look off; operator should delete it.
func (d *Deps) HandleBackupRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, 400, "invalid JSON")
			return
		}
	}
	name := body.Name
	if name == "" {
		name = fmt.Sprintf("pgdump-%s.dump", time.Now().UTC().Format("20060102-150405"))
	}
	if !backupNameRe.MatchString(name) {
		writeError(w, 400, "invalid backup name (allowed: [A-Za-z0-9._-], 1-128 chars)")
		return
	}

	cfg, err := backup.FromEnv()
	if err != nil {
		writeError(w, 400, "backup not configured: "+err.Error())
		return
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		writeError(w, 500, "DATABASE_URL not set")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), pgDumpTimeout)
	defer cancel()

	client, err := backup.NewClient(ctx, *cfg)
	if err != nil {
		writeError(w, 500, "s3 client: "+err.Error())
		return
	}

	elapsed, err := backup.RunPgDump(ctx, dbURL, client, name)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"name":     name,
		"duration": elapsed.String(),
	})
}

func (d *Deps) HandleBackupList(w http.ResponseWriter, r *http.Request) {
	cfg, err := backup.FromEnv()
	if err != nil {
		writeError(w, 400, "backup not configured: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	client, err := backup.NewClient(ctx, *cfg)
	if err != nil {
		writeError(w, 500, "s3 client: "+err.Error())
		return
	}
	objs, err := client.List(ctx)
	if err != nil {
		writeError(w, 500, "s3 list: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"backups": objs})
}

func (d *Deps) HandleBackupDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if !backupNameRe.MatchString(body.Name) {
		writeError(w, 400, "invalid backup name")
		return
	}
	cfg, err := backup.FromEnv()
	if err != nil {
		writeError(w, 400, "backup not configured: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	client, err := backup.NewClient(ctx, *cfg)
	if err != nil {
		writeError(w, 500, "s3 client: "+err.Error())
		return
	}
	if err := client.Delete(ctx, body.Name); err != nil {
		writeError(w, 500, "s3 delete: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"deleted": body.Name})
}

type restoreRequest struct {
	Name  string `json:"name"`
	Clean bool   `json:"clean"` // include --clean --if-exists in pg_restore
}

// HandleBackupRestore streams an object from S3 into pg_restore stdin,
// restoring it into the live postgres database.
//
// DESTRUCTIVE. With clean=true (default in UI), existing objects are
// dropped and recreated. Caller must be prepared for downtime and
// data loss outside the backup's coverage.
//
// The UI gates this behind a typed-name confirmation dialog; this
// handler takes the request at face value.
func (d *Deps) HandleBackupRestore(w http.ResponseWriter, r *http.Request) {
	// Refuse overlapping restores — two concurrent pg_restore --clean
	// against the same DB would race destructive DDL.
	if !restoreRunning.CompareAndSwap(false, true) {
		writeError(w, 409, "another restore is already in progress")
		return
	}
	defer restoreRunning.Store(false)

	var req restoreRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !backupNameRe.MatchString(req.Name) {
		writeError(w, 400, "invalid backup name")
		return
	}

	cfg, err := backup.FromEnv()
	if err != nil {
		writeError(w, 400, "backup not configured: "+err.Error())
		return
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		writeError(w, 500, "DATABASE_URL not set")
		return
	}

	// Same 30-min ceiling as pg_dump.
	ctx, cancel := context.WithTimeout(r.Context(), pgDumpTimeout)
	defer cancel()

	client, err := backup.NewClient(ctx, *cfg)
	if err != nil {
		writeError(w, 500, "s3 client: "+err.Error())
		return
	}

	body, err := client.Get(ctx, req.Name)
	if err != nil {
		writeError(w, 500, "s3 get: "+err.Error())
		return
	}
	// Close happens after cmd.Wait so we can unblock a stuck copy
	// goroutine (blocked in S3 body.Read) after pg_restore exits.
	defer body.Close()

	args := []string{"--dbname", dbURL, "--no-owner", "--no-acl"}
	if req.Clean {
		args = append(args, "--clean", "--if-exists")
	}
	cmd := exec.CommandContext(ctx, "pg_restore", args...)
	// On ctx cancel send SIGTERM (pg_restore aborts its current
	// transaction cleanly), falling back to SIGKILL after 5s. Without
	// this override CommandContext defaults to SIGKILL immediately,
	// which would leave the DB in a half-dropped state.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second

	// We own the S3→stdin copy explicitly (instead of cmd.Stdin = body)
	// so we can close the body to unblock a stuck Read if pg_restore
	// exits early. See `body.Close()` after cmd.Wait below.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		writeError(w, 500, "stdin pipe: "+err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		writeError(w, 500, "stderr pipe: "+err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		writeError(w, 500, "pg_restore start: "+err.Error())
		return
	}

	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(stdin, body)
		_ = stdin.Close() // signal EOF to pg_restore
		close(copyDone)
	}()

	var stderrBuf bytes.Buffer
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&stderrBuf, io.LimitReader(stderr, 64*1024))
		_, _ = io.Copy(io.Discard, stderr)
		close(stderrDone)
	}()

	start := time.Now()
	waitErr := cmd.Wait()
	// pg_restore has exited. If the copy goroutine is stuck in S3
	// body.Read (slow/hanging S3), closing the body unblocks it.
	_ = body.Close()
	<-copyDone
	<-stderrDone
	elapsed := time.Since(start)

	if waitErr != nil {
		writeError(w, 500, fmt.Sprintf("pg_restore failed: %v\n%s", waitErr, tailString(stderrBuf.String(), 4096)))
		return
	}

	// pg_restore can "succeed" (exit 0) while emitting harmless notices
	// (e.g. "does not exist, skipping" with --if-exists). Surface stderr
	// tail so the UI can show it; operator can decide if anything is
	// actionable.
	writeJSON(w, 200, map[string]any{
		"ok":       true,
		"name":     req.Name,
		"duration": elapsed.String(),
		"notices":  tailString(stderrBuf.String(), 4096),
	})
}

// tailString returns the last n bytes of s, prefixed with "…" if truncated.
func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// HandleBackupDownload returns a presigned GET URL. Client redirects
// to S3 directly — backup bytes never route through admin.
func (d *Deps) HandleBackupDownload(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if !backupNameRe.MatchString(name) {
		writeError(w, 400, "invalid backup name")
		return
	}
	cfg, err := backup.FromEnv()
	if err != nil {
		writeError(w, 400, "backup not configured: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	client, err := backup.NewClient(ctx, *cfg)
	if err != nil {
		writeError(w, 500, "s3 client: "+err.Error())
		return
	}
	url, err := client.PresignGet(ctx, name, 10*time.Minute)
	if err != nil {
		writeError(w, 500, "presign: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"url": url})
}
