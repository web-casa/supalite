package handler

import (
	"strings"
	"testing"
)

// buildMaintenanceSQL must reject any input that could let a caller
// inject arbitrary SQL through the target identifier. These tests
// pin the security contract in place.
func TestBuildMaintenanceSQLValid(t *testing.T) {
	cases := []struct {
		req     dbOpRequest
		wantSub string // expected substring in result
	}{
		{dbOpRequest{Op: "vacuum"}, "VACUUM"},
		{dbOpRequest{Op: "vacuum", Full: true}, "VACUUM (FULL)"},
		{dbOpRequest{Op: "vacuum", Target: "public.users"}, `"public"."users"`},
		{dbOpRequest{Op: "vacuum", Full: true, Target: "auth.refresh_tokens"}, `VACUUM (FULL) "auth"."refresh_tokens"`},
		{dbOpRequest{Op: "analyze", Target: "public.users"}, `ANALYZE "public"."users"`},
		{dbOpRequest{Op: "analyze"}, "ANALYZE"},
		{dbOpRequest{Op: "reindex", Target: "public.users"}, `REINDEX TABLE "public"."users"`},
		{dbOpRequest{Op: "reindex"}, "REINDEX DATABASE postgres"},
	}
	for _, c := range cases {
		got, err := buildMaintenanceSQL(c.req)
		if err != nil {
			t.Errorf("%+v: unexpected error: %v", c.req, err)
			continue
		}
		if !strings.Contains(got, c.wantSub) {
			t.Errorf("%+v: got %q, want substring %q", c.req, got, c.wantSub)
		}
	}
}

func TestBuildMaintenanceSQLInjectionAttempts(t *testing.T) {
	bad := []string{
		"public.users; DROP TABLE secrets",
		"public.users--",
		"public.users WHERE 1=1",
		"'; DROP TABLE users; --",
		"public.users\"escape\"",
		`public."weird"`,
		"public.users\x00null",
		"public.users with space",
		"a/b",
		"a..b",
	}
	for _, target := range bad {
		_, err := buildMaintenanceSQL(dbOpRequest{Op: "analyze", Target: target})
		if err == nil {
			t.Errorf("expected rejection of injection-like target %q", target)
		}
	}
}

func TestBuildMaintenanceSQLUnknownOp(t *testing.T) {
	_, err := buildMaintenanceSQL(dbOpRequest{Op: "DROP", Target: ""})
	if err == nil {
		t.Fatal("unknown op must error")
	}
}

func TestReindexSchemaLevelRefused(t *testing.T) {
	// Reindex without a `.` would mean "REINDEX SCHEMA <name>" — we
	// deliberately don't support it (target=public would be ambiguous
	// vs a table named public). Callers must qualify.
	_, err := buildMaintenanceSQL(dbOpRequest{Op: "reindex", Target: "public"})
	if err == nil {
		t.Fatal("reindex on schema-only identifier should fail")
	}
}

func TestRestoreLabelRe(t *testing.T) {
	good := []string{
		"20260101-120000F",
		"20260101-120000F_20260101-130000D",
		"20260101-120000F_20260101-130000I",
		"20260101-120000F_20260101-130000D_20260101-140000I",
	}
	for _, l := range good {
		if !restoreLabelRe.MatchString(l) {
			t.Errorf("expected %q to be accepted", l)
		}
	}
	bad := []string{
		"",
		"random",
		"20260101-120000",         // no F/D/I suffix
		"20260101_120000F",        // wrong separator
		"20260101-120000F; rm -rf",
		"../../etc/passwd",
		"20260101-120000F\nmalicious",
	}
	for _, l := range bad {
		if restoreLabelRe.MatchString(l) {
			t.Errorf("expected %q to be rejected", l)
		}
	}
}
