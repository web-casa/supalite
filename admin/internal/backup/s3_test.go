package backup

import "testing"

// ScopedKey is the second line of defense (after handler-level regex)
// against bucket traversal. These tests pin the contract.
func TestScopedKeyRejectsTraversal(t *testing.T) {
	c := &Client{cfg: Config{Prefix: "backup/"}}
	bad := []string{
		"",
		"../../etc/passwd",
		"foo/../../bar",
		"with/slash",
		"with\\backslash",
		"..",
		"a..b", // dot-dot anywhere is rejected by ContainsString check
	}
	for _, n := range bad {
		if _, err := c.ScopedKey(n); err == nil {
			t.Errorf("expected ScopedKey(%q) to be rejected", n)
		}
	}
}

func TestScopedKeyOK(t *testing.T) {
	c := &Client{cfg: Config{Prefix: "backup/"}}
	got, err := c.ScopedKey("pgdump-20260101-120000.dump")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "backup/pgdump-20260101-120000.dump" {
		t.Errorf("unexpected key: %q", got)
	}
}

func TestPrefixNormalized(t *testing.T) {
	// FromEnv ensures trailing slash on prefix; here we just test the
	// resulting key shape with a prefix that already has the slash.
	c := &Client{cfg: Config{Prefix: "backup/sub/"}}
	got, _ := c.ScopedKey("a.dump")
	if got != "backup/sub/a.dump" {
		t.Errorf("got %q", got)
	}
}
