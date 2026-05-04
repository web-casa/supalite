package handler

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
)

// restartRunning serializes restart requests. Two concurrent
// `docker compose up` calls don't break the daemon but do spam the
// logs and compete for the same state transitions — refuse the
// second one with 409 until the first completes.
var restartRunning atomic.Bool

func (d *Deps) HandleRestart(w http.ResponseWriter, r *http.Request) {
	if !restartRunning.CompareAndSwap(false, true) {
		writeError(w, 409, "a restart is already in progress")
		return
	}

	hostDir := os.Getenv("HOST_PROJECT_DIR")
	if hostDir == "" {
		hostDir = "/project"
	}

	// Detach the restart to a background goroutine with its own context.
	// Restarting `gateway` tears down the reverse proxy handling this
	// request, which would otherwise make the caller see "failed" even
	// on a successful restart.
	go func() {
		defer restartRunning.Store(false)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx,
			"docker", "compose",
			"--project-directory", hostDir,
			"-f", "/project/docker-compose.yml",
			"up", "-d",
			"--no-deps",
			"gotrue", "gateway",
		)
		cmd.Env = append(os.Environ(), "COMPOSE_PROJECT_NAME=supalite")
		// Graceful cancel: SIGTERM lets `docker compose` tear down its
		// client connection cleanly; SIGKILL after 5s in case it hangs.
		cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
		cmd.WaitDelay = 5 * time.Second

		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("[restart] compose up failed: %v\n%s", err, out)
			return
		}
		log.Printf("[restart] compose up ok:\n%s", out)
	}()

	// Respond immediately. The gateway/gotrue will restart asynchronously;
	// the client should re-fetch /api/status a few seconds later.
	writeJSON(w, 202, map[string]any{
		"accepted": true,
		"message":  "restart initiated in background",
	})
}
