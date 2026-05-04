package secrets

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supalite/admin/internal/envfile"
)

func TestGenSecretLength(t *testing.T) {
	s, err := genSecret(32)
	if err != nil {
		t.Fatal(err)
	}
	// Raw base64 (no padding) of 32 bytes = ceil(32*4/3) = 43 chars.
	if len(s) != 43 {
		t.Errorf("32-byte gen produced %d chars, want 43", len(s))
	}
	// Subsequent calls should produce different output (sanity).
	s2, _ := genSecret(32)
	if s == s2 {
		t.Error("two calls produced identical output — RNG broken?")
	}
}

func TestSignJWTVerifiable(t *testing.T) {
	secret := "test-secret-at-least-32-bytes-long-please"
	tok, err := signJWT([]byte(anonPayload), secret)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("expected header.payload.signature, got %d parts", len(parts))
	}

	// Verify signature: HMAC-SHA256(header.payload, secret) == sig.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	wantSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if parts[2] != wantSig {
		t.Errorf("signature mismatch: got %s want %s", parts[2], wantSig)
	}

	// Decode payload and check role.
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(raw, &got)
	if got["role"] != "anon" || got["iss"] != "SupaLite" {
		t.Errorf("payload mismatch: %+v", got)
	}
}

func writeTmpEnv(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	os.WriteFile(p, []byte(body), 0600)
	return p
}

func TestRotateAdminToken(t *testing.T) {
	p := writeTmpEnv(t, "ADMIN_TOKEN=old\n")
	res, err := Rotate("ADMIN_TOKEN", p)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != "ADMIN_TOKEN" {
		t.Errorf("updated = %v", res.Updated)
	}
	if len(res.Restart) != 0 {
		t.Errorf("ADMIN_TOKEN should require no restart, got %v", res.Restart)
	}
	got, _ := envfile.Read(p)
	if got["ADMIN_TOKEN"] == "old" || got["ADMIN_TOKEN"] == "" {
		t.Errorf("token not rotated: %q", got["ADMIN_TOKEN"])
	}
}

func TestRotateJWTCascades(t *testing.T) {
	p := writeTmpEnv(t,
		"JWT_SECRET=old-secret\nANON_KEY=old-anon\nSERVICE_ROLE_KEY=old-service\n")
	res, err := Rotate("JWT_SECRET", p)
	if err != nil {
		t.Fatal(err)
	}
	wantUpdated := map[string]bool{
		"JWT_SECRET": true, "ANON_KEY": true, "SERVICE_ROLE_KEY": true,
	}
	for _, k := range res.Updated {
		if !wantUpdated[k] {
			t.Errorf("unexpected update: %s", k)
		}
		delete(wantUpdated, k)
	}
	if len(wantUpdated) > 0 {
		t.Errorf("missing updates: %v", wantUpdated)
	}

	got, _ := envfile.Read(p)
	if got["JWT_SECRET"] == "old-secret" {
		t.Error("JWT_SECRET not rotated")
	}
	// New ANON_KEY/SERVICE_ROLE_KEY should be JWTs (3 parts, dot-separated).
	for _, k := range []string{"ANON_KEY", "SERVICE_ROLE_KEY"} {
		v := got[k]
		if strings.Count(v, ".") != 2 {
			t.Errorf("%s does not look like a JWT: %q", k, v)
		}
	}
	// The minted keys must verify against the NEW secret.
	parts := strings.Split(got["ANON_KEY"], ".")
	mac := hmac.New(sha256.New, []byte(got["JWT_SECRET"]))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	wantSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if parts[2] != wantSig {
		t.Error("ANON_KEY signature does not verify against new JWT_SECRET")
	}

	// Expected restart list.
	wantRestart := []string{"rest", "gotrue", "admin", "gateway"}
	if !slicesSetEqual(res.Restart, wantRestart) {
		t.Errorf("restart = %v, want %v", res.Restart, wantRestart)
	}
}

func TestRotateUnknownKey(t *testing.T) {
	p := writeTmpEnv(t, "X=1\n")
	if _, err := Rotate("not_a_real_key", p); err == nil {
		t.Error("unknown key should error")
	}
}

func TestRotatePostgresPasswordRefused(t *testing.T) {
	p := writeTmpEnv(t, "X=1\n")
	_, err := Rotate("POSTGRES_PASSWORD", p)
	if err == nil {
		t.Fatal("POSTGRES_PASSWORD must NOT be rotatable")
	}
	// Specific message — distinguishes from generic "unknown secret".
	if !strings.Contains(err.Error(), "ALTER USER") {
		t.Errorf("error should mention ALTER USER context, got: %v", err)
	}
}

// JWT bytes must be byte-identical to setup.sh's gen_jwt output.
// The signature differs (different secret) but the header.payload
// portion must match — header is fixed, payload uses a literal JSON
// string with deterministic key order (NOT a Go map which sorts).
func TestJWTPayloadByteOrderMatchesSetupSh(t *testing.T) {
	// setup.sh writes literal `{"role":"anon","iss":"SupaLite",...}`.
	tok, err := signJWT([]byte(anonPayload), "x")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	gotPayload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	if string(gotPayload) != anonPayload {
		t.Errorf("payload bytes drift from setup.sh literal:\n got %q\nwant %q",
			gotPayload, anonPayload)
	}
}

func TestCatalogStable(t *testing.T) {
	c := Catalog()
	if len(c) != 4 {
		t.Errorf("expected 4 entries, got %d", len(c))
	}
	wantOrder := []string{
		"ADMIN_TOKEN", "COOKIE_SIGNING_KEY", "PG_META_CRYPTO_KEY", "JWT_SECRET",
	}
	for i, e := range c {
		if e["key"] != wantOrder[i] {
			t.Errorf("position %d: got %v, want %s", i, e["key"], wantOrder[i])
		}
	}
}

func slicesSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		m[s]--
	}
	for _, v := range m {
		if v != 0 {
			return false
		}
	}
	return true
}
