// Package secrets implements secret rotation for SupaLite.
//
// Each secret has a different blast radius:
//
//   ADMIN_TOKEN          — admin login. Cookie sessions survive; only
//                          Authorization-header callers (scripts) need
//                          the new value. No service restart.
//   COOKIE_SIGNING_KEY   — admin session HMAC. Restart admin.
//                          All admin browser sessions get logged out.
//   PG_META_CRYPTO_KEY   — Studio's stored DB credentials encryption.
//                          Restart meta + studio. Studio loses any
//                          saved project connection metadata.
//   JWT_SECRET           — signs ANON_KEY, SERVICE_ROLE_KEY, and every
//                          user-session JWT. Cascades: ANON_KEY and
//                          SERVICE_ROLE_KEY are re-minted with the new
//                          secret. Restart rest + gotrue + admin +
//                          gateway. Every client app must update its
//                          ANON_KEY. Every signed-in user is logged out.
//
// POSTGRES_PASSWORD rotation is intentionally NOT supported here — it
// requires running ALTER USER for 12 supabase roles inside the live
// database, which has too many partial-failure modes for a one-click
// wizard. Operator who needs to rotate the DB password should follow
// the manual procedure documented in the OSS README.
package secrets

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/supalite/admin/internal/envfile"
)

// genSecret returns n bytes of cryptographically random data,
// base64-url-encoded (no padding). 32 bytes → 43 chars.
func genSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand.Read: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// signJWT mints an HS256 JWT from a RAW payload byte sequence using
// secret. We take raw bytes (not a map) because setup.sh uses literal
// JSON strings with deterministic key order — `json.Marshal` on a
// map sorts keys alphabetically, producing different bytes (and
// therefore a different signature) than setup.sh would. To preserve
// byte-for-byte equivalence with setup.sh-generated keys, callers
// pass the same exact JSON literals setup.sh uses.
func signJWT(payloadJSON []byte, secret string) (string, error) {
	headerJSON := []byte(`{"alg":"HS256","typ":"JWT"}`)
	h := base64.RawURLEncoding.EncodeToString(headerJSON)
	p := base64.RawURLEncoding.EncodeToString(payloadJSON)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(h + "." + p))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return h + "." + p + "." + sig, nil
}

// These are the exact literals setup.sh writes into JWTs. Hard-coded
// here so we round-trip identically with operator-generated keys.
const (
	anonPayload    = `{"role":"anon","iss":"SupaLite","iat":1700000000,"exp":4102444800}`
	servicePayload = `{"role":"service_role","iss":"SupaLite","iat":1700000000,"exp":4102444800}`
)

// Result is what Rotate returns and the handler echoes back to the UI.
type Result struct {
	Updated []string `json:"updated"`     // env keys whose values changed
	Restart []string `json:"restart"`     // compose service names to restart
	Notes   string   `json:"notes,omitempty"`
}

// Catalog returns the list of rotatable keys with human-readable
// metadata. Stable order so the UI can render it deterministically.
func Catalog() []map[string]any {
	return []map[string]any{
		{
			"key":         "ADMIN_TOKEN",
			"label":       "Admin login token",
			"description": "Bearer token used by scripts and the login form.",
			"consequence": "Existing browser sessions keep working until they expire. Any script using the old token starts getting 401.",
			"restart":     []string{},
		},
		{
			"key":         "COOKIE_SIGNING_KEY",
			"label":       "Admin session HMAC key",
			"description": "Signs the admin session cookie. Rotating it invalidates every signed cookie immediately.",
			"consequence": "All open admin browser sessions get logged out. Operator must log in again with the (still-current) ADMIN_TOKEN.",
			"restart":     []string{"admin"},
		},
		{
			"key":         "PG_META_CRYPTO_KEY",
			"label":       "Studio metadata encryption key",
			"description": "Encrypts the DB credentials Studio stores for each project.",
			"consequence": "Studio's saved project list becomes unreadable; you'll re-enter the connection in Studio's UI.",
			"restart":     []string{"meta", "studio"},
		},
		{
			"key":         "JWT_SECRET",
			"label":       "JWT signing secret (cascading)",
			"description": "Signs ANON_KEY, SERVICE_ROLE_KEY, and every user-session JWT. ANON_KEY and SERVICE_ROLE_KEY will be re-minted automatically.",
			"consequence": "EVERY signed-in user is logged out. Every client app needs the new ANON_KEY (visible on the Dashboard after rotation). REST/GoTrue start rejecting old user JWTs immediately on restart.",
			"restart":     []string{"rest", "gotrue", "admin", "gateway"},
		},
	}
}

// Rotate generates new value(s) for `key`, writes them to envPath
// atomically, and returns the side-effect summary. Atomic envfile.Write
// means a partial failure leaves .env unchanged.
func Rotate(key, envPath string) (*Result, error) {
	switch key {
	case "ADMIN_TOKEN":
		v, err := genSecret(36) // ~48 chars base64-url
		if err != nil {
			return nil, err
		}
		if err := envfile.Write(envPath, map[string]string{"ADMIN_TOKEN": v}); err != nil {
			return nil, fmt.Errorf("write env: %w", err)
		}
		return &Result{
			Updated: []string{"ADMIN_TOKEN"},
			Restart: nil,
			Notes:   "Admin process re-reads .env per request; no restart needed. Existing cookie sessions remain valid.",
		}, nil

	case "COOKIE_SIGNING_KEY":
		v, err := genSecret(32)
		if err != nil {
			return nil, err
		}
		if err := envfile.Write(envPath, map[string]string{"COOKIE_SIGNING_KEY": v}); err != nil {
			return nil, fmt.Errorf("write env: %w", err)
		}
		return &Result{
			Updated: []string{"COOKIE_SIGNING_KEY"},
			Restart: []string{"admin"},
			Notes:   "Restart the admin service to load the new key. All current admin browser sessions will be logged out.",
		}, nil

	case "PG_META_CRYPTO_KEY":
		v, err := genSecret(32)
		if err != nil {
			return nil, err
		}
		if err := envfile.Write(envPath, map[string]string{"PG_META_CRYPTO_KEY": v}); err != nil {
			return nil, fmt.Errorf("write env: %w", err)
		}
		return &Result{
			Updated: []string{"PG_META_CRYPTO_KEY"},
			Restart: []string{"meta", "studio"},
			Notes:   "Restart meta + studio to load the new key. Studio will need to be re-pointed at this database.",
		}, nil

	case "JWT_SECRET":
		newSecret, err := genSecret(48)
		if err != nil {
			return nil, err
		}
		// Re-mint with the exact literal payloads setup.sh uses.
		anonKey, err := signJWT([]byte(anonPayload), newSecret)
		if err != nil {
			return nil, err
		}
		serviceKey, err := signJWT([]byte(servicePayload), newSecret)
		if err != nil {
			return nil, err
		}
		updates := map[string]string{
			"JWT_SECRET":       newSecret,
			"ANON_KEY":         anonKey,
			"SERVICE_ROLE_KEY": serviceKey,
		}
		if err := envfile.Write(envPath, updates); err != nil {
			return nil, fmt.Errorf("write env: %w", err)
		}
		return &Result{
			Updated: []string{"JWT_SECRET", "ANON_KEY", "SERVICE_ROLE_KEY"},
			Restart: []string{"rest", "gotrue", "admin", "gateway"},
			Notes:   "All user sessions are now invalid. Update every client app with the new ANON_KEY (visible on the Dashboard).",
		}, nil

	case "POSTGRES_PASSWORD":
		// Explicit refusal so callers hitting the API directly get the
		// real reason, not a generic "unknown secret" error.
		return nil, fmt.Errorf(
			"POSTGRES_PASSWORD is not rotatable through this wizard: it requires " +
				"running ALTER USER for 12 supabase roles inside the live database, " +
				"which has too many partial-failure modes for one-click automation. " +
				"See the README for the manual procedure.",
		)
	}
	return nil, fmt.Errorf("unknown secret: %q (rotatable: ADMIN_TOKEN, COOKIE_SIGNING_KEY, PG_META_CRYPTO_KEY, JWT_SECRET)", key)
}
