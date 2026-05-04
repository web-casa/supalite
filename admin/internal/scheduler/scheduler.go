// Package scheduler runs periodic pg_dump backups and prunes old ones
// according to a retention count. Configured purely through env vars
// at admin start; no admin-API knobs (a missed schedule isn't worth
// the on-the-fly reconfiguration complexity).
//
// Env contract:
//   BACKUP_SCHEDULE_HOURS  — interval between scheduled backups, e.g. 24.
//                            Empty or 0 disables the scheduler entirely.
//   BACKUP_RETENTION_COUNT — keep this many newest scheduled backups;
//                            older ones are deleted after each successful run.
//                            Empty or 0 disables retention pruning.
//
// Scheduled backups are named `scheduled-YYYYMMDD-HHMMSS.dump`. Manual
// backups (`pgdump-...`) are NEVER touched by retention.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/supalite/admin/internal/backup"
)

// scheduledNamePrefix tags backup objects so retention only ever
// touches the ones we own. Manual user backups are immune.
const scheduledNamePrefix = "scheduled-"

type Scheduler struct {
	interval time.Duration
	retain   int // 0 = disabled
}

// FromEnv reads BACKUP_SCHEDULE_HOURS and BACKUP_RETENTION_COUNT.
// Returns nil, nil if scheduling is disabled (the empty/zero case).
// Returns nil, err only on malformed config — the admin should still
// boot in that case (caller logs and proceeds).
func FromEnv() (*Scheduler, error) {
	hoursStr := strings.TrimSpace(os.Getenv("BACKUP_SCHEDULE_HOURS"))
	if hoursStr == "" || hoursStr == "0" {
		return nil, nil
	}
	hours, err := strconv.Atoi(hoursStr)
	if err != nil || hours < 1 || hours > 24*30 {
		return nil, fmt.Errorf("BACKUP_SCHEDULE_HOURS must be 1..720, got %q", hoursStr)
	}

	retain := 0
	if r := strings.TrimSpace(os.Getenv("BACKUP_RETENTION_COUNT")); r != "" {
		retain, err = strconv.Atoi(r)
		if err != nil || retain < 0 {
			return nil, fmt.Errorf("BACKUP_RETENTION_COUNT must be a non-negative int, got %q", r)
		}
	}

	return &Scheduler{
		interval: time.Duration(hours) * time.Hour,
		retain:   retain,
	}, nil
}

// Run blocks until ctx is canceled. Sleeps for `interval` between
// runs (no try-to-catch-up logic — if the host was off during a
// scheduled tick, we just resume the cadence on next boot).
func (s *Scheduler) Run(ctx context.Context) {
	log.Printf("[scheduler] starting; interval=%s retention=%d", s.interval, s.retain)
	t := time.NewTimer(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("[scheduler] stopping: %v", ctx.Err())
			return
		case <-t.C:
			s.tick(ctx)
			t.Reset(s.interval)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	cfg, err := backup.FromEnv()
	if err != nil {
		log.Printf("[scheduler] skipping run — backup not configured: %v", err)
		return
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Printf("[scheduler] skipping run — DATABASE_URL not set")
		return
	}

	// Fresh client per tick — cheap (no network), and lets the operator
	// rotate S3 credentials by restarting admin without recompiling.
	jobCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	client, err := backup.NewClient(jobCtx, *cfg)
	if err != nil {
		log.Printf("[scheduler] s3 client: %v", err)
		return
	}

	name := fmt.Sprintf("%s%s.dump", scheduledNamePrefix,
		time.Now().UTC().Format("20060102-150405"))
	dur, err := backup.RunPgDump(jobCtx, dbURL, client, name)
	if err != nil {
		log.Printf("[scheduler] backup %s failed after %s: %v", name, dur, err)
		return
	}
	log.Printf("[scheduler] backup %s ok in %s", name, dur)

	if s.retain > 0 {
		s.prune(jobCtx, client)
	}
}

// prune lists scheduled-prefix objects, sorts newest-first, deletes
// the tail beyond s.retain. Failure to list/delete is logged but
// doesn't propagate — next tick will retry.
func (s *Scheduler) prune(ctx context.Context, client *backup.Client) {
	objs, err := client.List(ctx)
	if err != nil {
		log.Printf("[scheduler] retention list failed: %v", err)
		return
	}
	for _, victim := range pickRetentionVictims(objs, s.retain) {
		if err := client.Delete(ctx, victim.Name); err != nil {
			log.Printf("[scheduler] retention delete %s failed: %v", victim.Name, err)
			continue
		}
		log.Printf("[scheduler] retention deleted %s (kept %d newest)",
			victim.Name, s.retain)
	}
}

// pickRetentionVictims is the pure decision: filter for the
// scheduled-prefix, sort newest-first by LastModified, then return
// the tail older than the `retain` newest. Manual user backups
// (anything not prefixed `scheduled-`) are NEVER returned.
//
// Extracted so unit tests can pin the contract without touching S3.
func pickRetentionVictims(objs []backup.Object, retain int) []backup.Object {
	if retain <= 0 {
		return nil
	}
	scheduled := make([]backup.Object, 0, len(objs))
	for _, o := range objs {
		if strings.HasPrefix(o.Name, scheduledNamePrefix) {
			scheduled = append(scheduled, o)
		}
	}
	if len(scheduled) <= retain {
		return nil
	}
	sort.Slice(scheduled, func(i, j int) bool {
		return scheduled[i].LastModified.After(scheduled[j].LastModified)
	})
	return scheduled[retain:]
}
