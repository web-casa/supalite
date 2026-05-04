package handler

import (
	"strings"
	"testing"
)

// backup name validation guards an S3 key path. These tests prevent
// regressions that would let a caller traverse outside the prefix.
func TestBackupNameRe(t *testing.T) {
	good := []string{
		"pgdump-20260101-123000.dump",
		"a",
		"backup_name.tar.gz",
		"file-with-dashes",
		"file_with_underscores",
	}
	for _, n := range good {
		if !backupNameRe.MatchString(n) {
			t.Errorf("expected %q to be accepted", n)
		}
	}

	bad := []string{
		"",
		"with/slash",
		"with\\backslash",
		"with space",
		"with;semicolon",
		"with$dollar",
		"with`backtick",
		"with*star",
		strings.Repeat("a", 129), // exceeds 128
		// Note: ".." is permitted by this regex (dots are valid name
		// chars). Defense-in-depth against path traversal lives in
		// backup.Client.ScopedKey — covered by its own test.
	}
	for _, n := range bad {
		if backupNameRe.MatchString(n) {
			t.Errorf("expected %q to be rejected", n)
		}
	}
}

func TestTailString(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 100, "short"},
		{"", 10, ""},
		{"abcdefghij", 5, "…fghij"},
		{"a", 0, "…"},                    // n=0 → everything truncated
		{"abc", 3, "abc"},                // exactly fits
		{"abcd", 3, "…bcd"},              // 1 over
	}
	for _, c := range cases {
		got := tailString(c.in, c.n)
		if got != c.want {
			t.Errorf("tailString(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}
