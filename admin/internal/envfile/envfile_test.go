package envfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateValue(t *testing.T) {
	cases := []struct {
		v      string
		wantOK bool
	}{
		{"normal", true},
		{"with spaces", true},
		{"https://example.com/path?q=1", true},
		{"", true},
		{"with\nnewline", false},
		{"with\rcarriage", false},
		{"with\x00null", false},
	}
	for _, c := range cases {
		err := ValidateValue(c.v)
		if (err == nil) != c.wantOK {
			t.Errorf("ValidateValue(%q): wantOK=%v, err=%v", c.v, c.wantOK, err)
		}
	}
}

func writeTmp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadIgnoresCommentsAndBlanks(t *testing.T) {
	p := writeTmp(t, `
# top comment
FOO=bar

  # indented comment
BAZ=qux
`)
	got, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if got["FOO"] != "bar" || got["BAZ"] != "qux" {
		t.Fatalf("unexpected: %#v", got)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %#v", len(got), got)
	}
}

func TestReadStripsSingleQuotes(t *testing.T) {
	p := writeTmp(t, "URL='https://x.example.com'\n")
	got, _ := Read(p)
	if got["URL"] != "https://x.example.com" {
		t.Fatalf("got %q", got["URL"])
	}
}

func TestReadDoubleQuoteEscapes(t *testing.T) {
	p := writeTmp(t, `MSG="line1\nline2\twith\\back"
`)
	got, _ := Read(p)
	if got["MSG"] != "line1\nline2\twith\\back" {
		t.Fatalf("escapes wrong: %q", got["MSG"])
	}
}

func TestWriteUpdatesAndAppends(t *testing.T) {
	p := writeTmp(t, "EXISTING=old\n")
	err := Write(p, map[string]string{
		"EXISTING": "new",
		"BRAND_NEW": "value",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := Read(p)
	if got["EXISTING"] != "new" {
		t.Errorf("EXISTING not updated: %q", got["EXISTING"])
	}
	if got["BRAND_NEW"] != "value" {
		t.Errorf("BRAND_NEW not appended: %q", got["BRAND_NEW"])
	}
}

func TestWriteRoundtripsSpecialChars(t *testing.T) {
	p := writeTmp(t, "")
	values := map[string]string{
		"PLAIN":       "simple",
		"WITH_SPACE":  "two words",
		"WITH_HASH":   "value#hash",
		"WITH_EQUALS": "key=value",
		"WITH_DOLLAR": "$VAR_REF",
		"WITH_QUOTE":  "it's tricky",
		"REGEX":       `^(https://app\.com|https://other\.com)$`,
	}
	if err := Write(p, values); err != nil {
		t.Fatal(err)
	}
	got, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range values {
		if got[k] != v {
			t.Errorf("%s: want %q, got %q", k, v, got[k])
		}
	}
}

func TestWritePreservesComments(t *testing.T) {
	p := writeTmp(t, `# Section header
KEY=oldvalue
# trailing comment
`)
	if err := Write(p, map[string]string{"KEY": "newvalue"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	body := string(data)
	if !strings.Contains(body, "# Section header") {
		t.Errorf("section comment lost: %s", body)
	}
	if !strings.Contains(body, "# trailing comment") {
		t.Errorf("trailing comment lost: %s", body)
	}
	if !strings.Contains(body, "KEY=newvalue") {
		t.Errorf("update missing: %s", body)
	}
}

func TestWriteAtomicNoLeftover(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	os.WriteFile(p, []byte("X=1\n"), 0600)
	if err := Write(p, map[string]string{"X": "2"}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".env.tmp.") {
			t.Errorf("temp file leaked: %s", e.Name())
		}
	}
}

func TestWriteFilePermissions(t *testing.T) {
	p := writeTmp(t, "X=1\n")
	if err := Write(p, map[string]string{"X": "2"}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(p)
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file mode not 0600 after Write: %v", perm)
	}
}
