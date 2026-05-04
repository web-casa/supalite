package scheduler

import (
	"os"
	"testing"
	"time"

	"github.com/supalite/admin/internal/backup"
)

func setEnv(t *testing.T, k, v string) {
	t.Helper()
	old, had := os.LookupEnv(k)
	if v == "" {
		os.Unsetenv(k)
	} else {
		os.Setenv(k, v)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv(k, old)
		} else {
			os.Unsetenv(k)
		}
	})
}

func TestFromEnvDisabled(t *testing.T) {
	setEnv(t, "BACKUP_SCHEDULE_HOURS", "")
	s, err := FromEnv()
	if err != nil || s != nil {
		t.Fatalf("empty schedule should return (nil, nil), got (%v, %v)", s, err)
	}
	setEnv(t, "BACKUP_SCHEDULE_HOURS", "0")
	s, err = FromEnv()
	if err != nil || s != nil {
		t.Fatalf("zero schedule should return (nil, nil), got (%v, %v)", s, err)
	}
}

func TestFromEnvValid(t *testing.T) {
	setEnv(t, "BACKUP_SCHEDULE_HOURS", "24")
	setEnv(t, "BACKUP_RETENTION_COUNT", "7")
	s, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.interval != 24*time.Hour {
		t.Errorf("interval = %v, want 24h", s.interval)
	}
	if s.retain != 7 {
		t.Errorf("retain = %d, want 7", s.retain)
	}
}

func TestFromEnvBadInputs(t *testing.T) {
	cases := []struct {
		name, hours, retain string
	}{
		{"non-numeric hours", "soon", ""},
		{"negative hours", "-1", ""},
		{"way too big hours", "9999", ""},
		{"non-numeric retain", "1", "many"},
		{"negative retain", "1", "-3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setEnv(t, "BACKUP_SCHEDULE_HOURS", c.hours)
			setEnv(t, "BACKUP_RETENTION_COUNT", c.retain)
			if _, err := FromEnv(); err == nil {
				t.Errorf("expected error for %+v", c)
			}
		})
	}
}

func TestPickRetentionVictimsEmpty(t *testing.T) {
	if v := pickRetentionVictims(nil, 5); len(v) != 0 {
		t.Errorf("empty input → %d victims", len(v))
	}
}

func TestPickRetentionVictimsRetainZero(t *testing.T) {
	objs := []backup.Object{
		{Name: "scheduled-a.dump", LastModified: time.Now()},
	}
	if v := pickRetentionVictims(objs, 0); len(v) != 0 {
		t.Errorf("retain=0 should disable pruning, got %d victims", len(v))
	}
}

func TestPickRetentionVictimsManualNeverDeleted(t *testing.T) {
	now := time.Now()
	objs := []backup.Object{
		// 5 manual + 5 scheduled, but retain only the newest 2 scheduled.
		{Name: "pgdump-1.dump", LastModified: now.Add(-1 * time.Hour)},
		{Name: "pgdump-2.dump", LastModified: now.Add(-2 * time.Hour)},
		{Name: "pgdump-3.dump", LastModified: now.Add(-3 * time.Hour)},
		{Name: "pgdump-4.dump", LastModified: now.Add(-4 * time.Hour)},
		{Name: "pgdump-5.dump", LastModified: now.Add(-5 * time.Hour)},
		{Name: "scheduled-1.dump", LastModified: now.Add(-1 * time.Minute)},
		{Name: "scheduled-2.dump", LastModified: now.Add(-2 * time.Minute)},
		{Name: "scheduled-3.dump", LastModified: now.Add(-3 * time.Minute)},
		{Name: "scheduled-4.dump", LastModified: now.Add(-4 * time.Minute)},
		{Name: "scheduled-5.dump", LastModified: now.Add(-5 * time.Minute)},
	}
	victims := pickRetentionVictims(objs, 2)
	if len(victims) != 3 {
		t.Fatalf("expected 3 victims (5 - 2 retained), got %d", len(victims))
	}
	for _, v := range victims {
		if v.Name[:9] != "scheduled" {
			t.Errorf("victim %q is not scheduled-prefix — manual backups must never be deleted", v.Name)
		}
	}
}

func TestPickRetentionVictimsNewestFirst(t *testing.T) {
	now := time.Now()
	objs := []backup.Object{
		{Name: "scheduled-old", LastModified: now.Add(-3 * time.Hour)},
		{Name: "scheduled-new", LastModified: now},
		{Name: "scheduled-mid", LastModified: now.Add(-1 * time.Hour)},
	}
	// retain=1 → keep only "scheduled-new"; both "old" and "mid" become victims.
	victims := pickRetentionVictims(objs, 1)
	if len(victims) != 2 {
		t.Fatalf("expected 2 victims, got %d", len(victims))
	}
	got := map[string]bool{}
	for _, v := range victims {
		got[v.Name] = true
	}
	if !got["scheduled-old"] || !got["scheduled-mid"] {
		t.Errorf("wrong victims: %v", got)
	}
	if got["scheduled-new"] {
		t.Errorf("newest backup must NOT be a victim")
	}
}

func TestPickRetentionVictimsExactlyRetain(t *testing.T) {
	now := time.Now()
	objs := []backup.Object{
		{Name: "scheduled-a", LastModified: now},
		{Name: "scheduled-b", LastModified: now.Add(-1 * time.Hour)},
		{Name: "scheduled-c", LastModified: now.Add(-2 * time.Hour)},
	}
	if v := pickRetentionVictims(objs, 3); len(v) != 0 {
		t.Errorf("count == retain → no victims, got %d", len(v))
	}
	if v := pickRetentionVictims(objs, 5); len(v) != 0 {
		t.Errorf("count < retain → no victims, got %d", len(v))
	}
}
