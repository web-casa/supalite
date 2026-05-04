package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/supalite/admin/internal/envfile"
)

// Phase 7.2: lightweight "are these credentials real?" check for the
// OAuth providers SupaLite surfaces in Settings. We don't verify
// the full auth flow (that requires a redirect + user consent); we
// just hit a provider endpoint that authenticates client_id+secret
// without a user present. Saves admins from discovering typos during
// their users' first sign-in.
//
// Apple is intentionally not covered — its "client_secret" is a
// short-lived JWT signed with a private key, not a static string, so
// any static-secret validation would be misleading.

type oauthTestRequest struct {
	Provider string `json:"provider"` // "github" | "google"
}

// HandleOAuthTest dispatches to the provider-specific validator and
// normalizes the response shape.
func (d *Deps) HandleOAuthTest(w http.ResponseWriter, r *http.Request) {
	var req oauthTestRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	env, err := envfile.Read(d.EnvFile)
	if err != nil {
		writeError(w, 500, "read env: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var (
		msg     string
		testErr error
	)
	switch req.Provider {
	case "github":
		id := strings.TrimSpace(env["GOTRUE_EXTERNAL_GITHUB_CLIENT_ID"])
		secret := env["GOTRUE_EXTERNAL_GITHUB_SECRET"]
		if id == "" || secret == "" {
			writeError(w, 400, "github client_id and secret not saved — save first, then test")
			return
		}
		msg, testErr = testGitHubOAuth(ctx, id, secret)
	case "google":
		id := strings.TrimSpace(env["GOTRUE_EXTERNAL_GOOGLE_CLIENT_ID"])
		secret := env["GOTRUE_EXTERNAL_GOOGLE_SECRET"]
		redirect := strings.TrimSpace(env["GOTRUE_EXTERNAL_GOOGLE_REDIRECT_URI"])
		if id == "" || secret == "" {
			writeError(w, 400, "google client_id and secret not saved — save first, then test")
			return
		}
		msg, testErr = testGoogleOAuth(ctx, id, secret, redirect)
	default:
		writeError(w, 400, "unsupported provider (github|google; apple uses JWT secrets and is not covered)")
		return
	}

	if testErr != nil {
		writeJSON(w, 400, map[string]any{
			"provider": req.Provider,
			"ok":       false,
			"message":  testErr.Error(),
		})
		return
	}
	writeJSON(w, 200, map[string]any{
		"provider": req.Provider,
		"ok":       true,
		"message":  msg,
	})
}

// testGitHubOAuth checks GitHub OAuth App credentials via the
// "check a token" endpoint:
//   POST /applications/{client_id}/token
//   Basic auth = client_id:client_secret
//   body = {"access_token":"..."}
//
// We deliberately supply a probe token that can never be a real
// GitHub access token. The response codes GitHub returns:
//   - 401: Basic auth failed (wrong client_id OR wrong secret —
//     indistinguishable per GitHub's auth layer).
//   - 404: auth passed, token not found (= the fake token, as expected).
//     This is our success signal for a credentials probe.
//   - 200: auth passed, token actually valid (impossible for our fake).
//   - 422: request body validation failure (shouldn't happen here).
func testGitHubOAuth(ctx context.Context, clientID, clientSecret string) (string, error) {
	u := "https://api.github.com/applications/" + url.PathEscape(clientID) + "/token"
	req, err := http.NewRequestWithContext(ctx, "POST", u,
		strings.NewReader(`{"access_token":"supalite-validation-probe"}`))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "supalite-admin/oauth-test")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound: // 404 — auth passed, probe token rejected (expected)
		return "credentials valid (GitHub authenticated client_id + secret)", nil
	case http.StatusOK: // 200 — unexpected: our probe token is not a real token
		return "credentials valid (but the probe token resolved — unlikely; verify manually)", nil
	case http.StatusUnauthorized: // 401
		return "", fmt.Errorf("invalid client credentials (GitHub rejected client_id or secret)")
	default:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("unexpected GitHub response %d: %s", resp.StatusCode, tailString(string(b), 256))
	}
}

// testGoogleOAuth validates by sending a deliberately-invalid
// authorization-code grant to Google's token endpoint. Response
// shape distinguishes credential quality:
//   - invalid_grant         → credentials OK; our test code is fake (expected)
//   - invalid_client        → client_id/secret wrong
//   - redirect_uri_mismatch → credentials OK but the saved redirect URI
//                             isn't registered on the Google OAuth client
func testGoogleOAuth(ctx context.Context, clientID, clientSecret, redirectURI string) (string, error) {
	if redirectURI == "" {
		// Fallback — matches the default in .env.example. Google requires
		// *some* redirect_uri in the request even for this probe.
		redirectURI = "http://localhost:8000/auth/v1/callback"
	}
	form := url.Values{}
	form.Set("code", "supalite-validation-probe")
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var parsed struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	_ = json.Unmarshal(body, &parsed)

	switch parsed.Error {
	case "invalid_grant":
		return "credentials valid (Google accepted client_id + secret)", nil
	case "invalid_client":
		return "", fmt.Errorf("invalid client credentials (Google rejected client_id or secret)")
	case "redirect_uri_mismatch":
		return "", fmt.Errorf("credentials OK but redirect_uri %q is not registered on this Google OAuth client", redirectURI)
	case "":
		// Either 2xx (unexpected — a fake code shouldn't succeed) or a
		// non-JSON error body.
		return "", fmt.Errorf("unexpected Google response %d: %s", resp.StatusCode, tailString(string(body), 256))
	default:
		return "", fmt.Errorf("%s: %s", parsed.Error, parsed.ErrorDescription)
	}
}
