// Package cookie implements HMAC-signed session cookies for the admin panel.
//
// Why HMAC instead of just storing the admin token in a cookie?
//   - A raw token cookie would let any XSS steal it verbatim → attacker hits the API
//   - An HMAC cookie is bound to a short "session id" + timestamp. Stealing
//     the cookie cookie-value lets the attacker impersonate until expiry,
//     but the cookie never contains the admin token itself.
//
// Format: base64url(payload) + "." + base64url(hmac-sha256(payload, key))
// Payload: JSON {"exp": <unix_ts>, "iat": <unix_ts>}
//
// The cookie is set with HttpOnly + Secure (when API_EXTERNAL_URL is https) +
// SameSite=Strict. It's read back on every API call as an alternative to
// the Authorization header (which EventSource/WebSocket can't send).
package cookie

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// Name is the cookie name. Prefixed with sbl_ to avoid collision.
	Name = "sbl_auth"

	// Default session TTL. User must reauthenticate after this.
	DefaultTTL = 24 * time.Hour
)

type payload struct {
	Exp int64 `json:"exp"`
	Iat int64 `json:"iat"`
}

// Signer issues and verifies auth cookies using HMAC-SHA256.
type Signer struct {
	key []byte
	ttl time.Duration
}

// New creates a Signer. key must be at least 32 bytes (enforced).
func New(key string, ttl time.Duration) (*Signer, error) {
	if len(key) < 32 {
		return nil, fmt.Errorf("cookie signing key must be at least 32 bytes, got %d", len(key))
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Signer{key: []byte(key), ttl: ttl}, nil
}

// Issue returns a signed cookie value valid for s.ttl from now.
func (s *Signer) Issue() (string, error) {
	now := time.Now().UTC()
	p := payload{
		Exp: now.Add(s.ttl).Unix(),
		Iat: now.Unix(),
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	mac := s.compute(encoded)
	return encoded + "." + mac, nil
}

// Verify returns nil if the cookie value is valid and not expired.
func (s *Signer) Verify(value string) error {
	idx := strings.LastIndexByte(value, '.')
	if idx <= 0 || idx == len(value)-1 {
		return errors.New("malformed cookie")
	}
	encoded := value[:idx]
	providedMAC := value[idx+1:]

	expectedMAC := s.compute(encoded)
	if subtle.ConstantTimeCompare([]byte(providedMAC), []byte(expectedMAC)) != 1 {
		return errors.New("invalid signature")
	}

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	now := time.Now().UTC().Unix()
	if p.Exp <= now {
		return errors.New("cookie expired")
	}
	// Sanity: issued-at in the future beyond small clock skew
	if p.Iat > now+60 {
		return errors.New("cookie issued in the future")
	}
	return nil
}

func (s *Signer) compute(data string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Set writes the signed cookie to the response.
// secure=true sets the Secure flag (HTTPS only).
func (s *Signer) Set(w http.ResponseWriter, secure bool) error {
	value, err := s.Issue()
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     Name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(s.ttl.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
	return nil
}

// Clear instructs the browser to delete the cookie.
func Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     Name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}
