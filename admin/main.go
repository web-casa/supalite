package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"os"

	"github.com/supalite/admin/internal/cookie"
	"github.com/supalite/admin/internal/db"
	"github.com/supalite/admin/internal/docker"
	"github.com/supalite/admin/internal/handler"
	"github.com/supalite/admin/internal/scheduler"
	"github.com/supalite/admin/internal/server"
)

//go:embed all:web/out
var embeddedFS embed.FS

func main() {
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN is required")
	}

	cookieKey := os.Getenv("COOKIE_SIGNING_KEY")
	if cookieKey == "" {
		log.Fatal("COOKIE_SIGNING_KEY is required (at least 32 bytes)")
	}

	envFile := os.Getenv("ENV_FILE")
	if envFile == "" {
		envFile = "/app/.env"
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":9100"
	}

	ctx := context.Background()

	pools, err := db.NewPools(ctx, databaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pools.Close()

	// Compose project name — matches docker label `com.docker.compose.project`.
	// Defaults to "supalite" but honors COMPOSE_PROJECT_NAME if set (which
	// is always injected by docker-compose.yml for consistency across
	// renames / multi-instance deployments).
	project := os.Getenv("COMPOSE_PROJECT_NAME")
	if project == "" {
		project = "supalite"
	}
	dockerClient := docker.NewClient(project)

	signer, err := cookie.New(cookieKey, 0) // default TTL
	if err != nil {
		log.Fatalf("cookie signer: %v", err)
	}

	// Pull API_EXTERNAL_URL from env file so cookie Secure flag tracks deployment.
	// (We read it fresh on each request in the handler — this is just startup.)
	apiExternalURL := os.Getenv("API_EXTERNAL_URL")

	deps := &handler.Deps{
		EnvFile:        envFile,
		DB:             pools,
		Docker:         dockerClient,
		AdminToken:     adminToken,
		CookieSigner:   signer,
		APIExternalURL: apiExternalURL,
	}

	staticFS, err := fs.Sub(embeddedFS, "web/out")
	if err != nil {
		log.Fatalf("failed to get static fs: %v", err)
	}

	// Scheduled backups — opt-in via BACKUP_SCHEDULE_HOURS. Disabled
	// schedulers return nil with no error.
	if sched, err := scheduler.FromEnv(); err != nil {
		log.Printf("[scheduler] config invalid (%v) — scheduling DISABLED", err)
	} else if sched != nil {
		go sched.Run(context.Background())
	}

	h := server.New(deps, staticFS)
	server.ListenAndServe(listenAddr, h)
}
