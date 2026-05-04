// Package envfile reads/writes .env files with Docker-Compose-compatible
// quoting semantics. Values with special characters are single-quoted on
// write, and quotes are stripped on read.
package envfile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// ValidateValue rejects values that would break .env parsing or shell
// interpretation. It does not do field-specific validation — callers should
// layer their own regex check on top (see handler/config.go).
func ValidateValue(v string) error {
	if strings.ContainsAny(v, "\n\r") {
		return fmt.Errorf("value contains newline")
	}
	if strings.ContainsRune(v, 0) {
		return fmt.Errorf("value contains null byte")
	}
	return nil
}

var (
	lineRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	mu     sync.Mutex
)

// Read returns a map of KEY → unquoted value.
func Read(path string) (map[string]string, error) {
	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := lineRe.FindStringSubmatch(line)
		if m != nil {
			values[m[1]] = unquoteEnvValue(m[2])
		}
	}
	return values, nil
}

// Write updates existing keys in place, appends new keys at the end, and
// atomically renames into position.
func Write(path string, updates map[string]string) error {
	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	seen := make(map[string]bool)
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		m := lineRe.FindStringSubmatch(trimmed)
		if m != nil {
			if val, ok := updates[m[1]]; ok {
				lines[i] = m[1] + "=" + quoteEnvValue(val)
				seen[m[1]] = true
			}
		}
	}

	// Append keys not yet present
	for k, v := range updates {
		if !seen[k] {
			lines = append(lines, k+"="+quoteEnvValue(v))
		}
	}

	content := []byte(strings.Join(lines, "\n"))

	// Atomic write: temp file + rename
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".env.tmp.*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// envSpecialChars characters that trigger quoting on write.
// Docker Compose v2 handles these but without quotes the parse is ambiguous.
const envSpecialChars = " \t#\"'`$\\"

// quoteEnvValue wraps the value in single quotes if it contains special chars.
// Single quotes don't allow escaping — if the value itself contains a
// single quote, we fall back to double quotes with \" escapes.
func quoteEnvValue(v string) string {
	if v == "" {
		return v
	}
	if !strings.ContainsAny(v, envSpecialChars) {
		return v
	}
	if !strings.ContainsRune(v, '\'') {
		return "'" + v + "'"
	}
	// Has single quote — use double quotes with escapes
	escaped := strings.ReplaceAll(v, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "$", `\$`)
	escaped = strings.ReplaceAll(escaped, "`", "\\`")
	return `"` + escaped + `"`
}

// unquoteEnvValue strips balanced surrounding quotes. Mirrors Docker Compose
// behavior: unquoted values are returned verbatim, single-quoted are literal,
// double-quoted support \n \t \" \\ \$ escapes.
func unquoteEnvValue(v string) string {
	if len(v) < 2 {
		return v
	}
	first, last := v[0], v[len(v)-1]
	if first == '\'' && last == '\'' {
		return v[1 : len(v)-1] // single quote = literal
	}
	if first == '"' && last == '"' {
		inner := v[1 : len(v)-1]
		// Process escapes
		var sb strings.Builder
		sb.Grow(len(inner))
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\\' && i+1 < len(inner) {
				switch inner[i+1] {
				case 'n':
					sb.WriteByte('\n')
				case 't':
					sb.WriteByte('\t')
				case 'r':
					sb.WriteByte('\r')
				case '"', '\\', '$', '`':
					sb.WriteByte(inner[i+1])
				default:
					sb.WriteByte(inner[i])
					sb.WriteByte(inner[i+1])
				}
				i++
				continue
			}
			sb.WriteByte(inner[i])
		}
		return sb.String()
	}
	return v
}
