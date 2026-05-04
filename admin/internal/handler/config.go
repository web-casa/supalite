package handler

import (
	"net/http"
	"regexp"
	"strconv"

	"github.com/supalite/admin/internal/envfile"
)

// fieldValidators maps editable field names to per-field regex.
// Empty string is always accepted (means "clear the field").
// Fields not in this map fall back to envfile.ValidateValue alone.
var fieldValidators = map[string]*regexp.Regexp{
	"SITE_URL":                            regexp.MustCompile(`^(https?://[a-zA-Z0-9.\-_:/]+)?$`),
	"API_EXTERNAL_URL":                    regexp.MustCompile(`^(https?://[a-zA-Z0-9.\-_:/]+)?$`),
	// Regex alternation of allowed CORS origins. Regex metachars allowed.
	// Length-capped to bound Caddyfile parsing.
	"CORS_ALLOWED_ORIGINS_REGEX": regexp.MustCompile(`^[a-zA-Z0-9.\-_:/?=&|\\()^$*+\[\]]{0,1000}$`),
	// Comma-separated list of URLs / URL prefixes for GoTrue redirect allow list.
	"GOTRUE_URI_ALLOW_LIST": regexp.MustCompile(`^[a-zA-Z0-9.\-_:/?=&,+]{0,1000}$`),
	"GOTRUE_SMTP_HOST":                    regexp.MustCompile(`^[a-zA-Z0-9._\-]*$`),
	// Regex ensures digits-only; validateField does an extra range check (B4).
	"GOTRUE_SMTP_PORT": regexp.MustCompile(`^(\d{1,5})?$`),
	"GOTRUE_SMTP_ADMIN_EMAIL":             regexp.MustCompile(`^([a-zA-Z0-9._+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,})?$`),
	"GOTRUE_EXTERNAL_GITHUB_REDIRECT_URI": regexp.MustCompile(`^(https?://[a-zA-Z0-9.\-_:/?=&]+)?$`),
	"GOTRUE_EXTERNAL_GOOGLE_REDIRECT_URI": regexp.MustCompile(`^(https?://[a-zA-Z0-9.\-_:/?=&]+)?$`),
	"GOTRUE_EXTERNAL_APPLE_REDIRECT_URI":  regexp.MustCompile(`^(https?://[a-zA-Z0-9.\-_:/?=&]+)?$`),
	"GOTRUE_EXTERNAL_GITHUB_CLIENT_ID":    regexp.MustCompile(`^[a-zA-Z0-9._\-]*$`),
	"GOTRUE_EXTERNAL_GOOGLE_CLIENT_ID":    regexp.MustCompile(`^[a-zA-Z0-9._\-]*$`),
	"GOTRUE_EXTERNAL_APPLE_CLIENT_ID":     regexp.MustCompile(`^[a-zA-Z0-9._\-]*$`),
	"GOTRUE_DISABLE_SIGNUP":               regexp.MustCompile(`^(true|false)?$`),
	"GOTRUE_EXTERNAL_ANONYMOUS_USERS_ENABLED": regexp.MustCompile(`^(true|false)?$`),
	"GOTRUE_MAILER_AUTOCONFIRM":           regexp.MustCompile(`^(true|false)?$`),
	"GOTRUE_EXTERNAL_GITHUB_ENABLED":      regexp.MustCompile(`^(true|false)?$`),
	"GOTRUE_EXTERNAL_GOOGLE_ENABLED":      regexp.MustCompile(`^(true|false)?$`),
	"GOTRUE_EXTERNAL_APPLE_ENABLED":       regexp.MustCompile(`^(true|false)?$`),
	"SETUP_COMPLETE":                      regexp.MustCompile(`^(true|false)?$`),
	// Backup — S3 settings
	"BACKUP_S3_ENDPOINT":   regexp.MustCompile(`^(https?://[a-zA-Z0-9.\-_:/]+)?$`),
	"BACKUP_S3_BUCKET":     regexp.MustCompile(`^[a-zA-Z0-9.\-_]{0,63}$`),
	"BACKUP_S3_REGION":     regexp.MustCompile(`^[a-zA-Z0-9\-]{0,32}$`),
	"BACKUP_S3_ACCESS_KEY": regexp.MustCompile(`^[A-Za-z0-9/+=]{0,128}$`),
	"BACKUP_S3_PATH_STYLE": regexp.MustCompile(`^(true|false)?$`),
	"BACKUP_S3_PREFIX":     regexp.MustCompile(`^[a-zA-Z0-9.\-_/]{0,64}$`),
	// Secret fields (password, client secrets): only enforce ValidateValue.
	// We can't restrict allowed chars because users pick their own.
}

// validateField combines envfile.ValidateValue + per-field regex + range checks.
func validateField(name, value string) error {
	if err := envfile.ValidateValue(value); err != nil {
		return err
	}
	if re, ok := fieldValidators[name]; ok {
		if !re.MatchString(value) {
			return &fieldFormatError{field: name}
		}
	}
	// Extra range checks beyond what regex can express.
	switch name {
	case "GOTRUE_SMTP_PORT":
		if value == "" {
			return nil
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 65535 {
			return &fieldFormatError{field: name}
		}
	}
	return nil
}

type fieldFormatError struct{ field string }

func (e *fieldFormatError) Error() string {
	return "invalid format for field " + e.field
}

var editableFields = map[string][]string{
	"networking":   {"SITE_URL", "API_EXTERNAL_URL", "CORS_ALLOWED_ORIGINS_REGEX", "GOTRUE_URI_ALLOW_LIST"},
	"general":      {"GOTRUE_DISABLE_SIGNUP", "GOTRUE_EXTERNAL_ANONYMOUS_USERS_ENABLED"},
	"smtp":         {"GOTRUE_SMTP_HOST", "GOTRUE_SMTP_PORT", "GOTRUE_SMTP_USER", "GOTRUE_SMTP_PASS", "GOTRUE_SMTP_ADMIN_EMAIL", "GOTRUE_MAILER_AUTOCONFIRM"},
	"oauth_github": {"GOTRUE_EXTERNAL_GITHUB_ENABLED", "GOTRUE_EXTERNAL_GITHUB_CLIENT_ID", "GOTRUE_EXTERNAL_GITHUB_SECRET", "GOTRUE_EXTERNAL_GITHUB_REDIRECT_URI"},
	"oauth_google": {"GOTRUE_EXTERNAL_GOOGLE_ENABLED", "GOTRUE_EXTERNAL_GOOGLE_CLIENT_ID", "GOTRUE_EXTERNAL_GOOGLE_SECRET", "GOTRUE_EXTERNAL_GOOGLE_REDIRECT_URI"},
	"oauth_apple":  {"GOTRUE_EXTERNAL_APPLE_ENABLED", "GOTRUE_EXTERNAL_APPLE_CLIENT_ID", "GOTRUE_EXTERNAL_APPLE_SECRET", "GOTRUE_EXTERNAL_APPLE_REDIRECT_URI"},
	"backup":       {"BACKUP_S3_ENDPOINT", "BACKUP_S3_BUCKET", "BACKUP_S3_REGION", "BACKUP_S3_ACCESS_KEY", "BACKUP_S3_SECRET_KEY", "BACKUP_S3_PATH_STYLE", "BACKUP_S3_PREFIX"},
}

var allEditable map[string]bool

// Fields whose current value is NEVER returned to the client in cleartext.
// The frontend gets a fixed sentinel string if the value is set, empty
// string otherwise. POSTing the sentinel back is ignored (keeps existing
// value). The sentinel is deliberately unlikely to collide with any
// real secret; if a user truly needs this exact string, they can edit
// the .env file directly.
const secretPlaceholder = "__SECRET_UNCHANGED__"

var secretFields = map[string]bool{
	"GOTRUE_SMTP_PASS":              true,
	"GOTRUE_EXTERNAL_GITHUB_SECRET": true,
	"GOTRUE_EXTERNAL_GOOGLE_SECRET": true,
	"GOTRUE_EXTERNAL_APPLE_SECRET":  true,
	"BACKUP_S3_SECRET_KEY":          true,
}

func init() {
	allEditable = make(map[string]bool)
	for _, fields := range editableFields {
		for _, f := range fields {
			allEditable[f] = true
		}
	}
	allEditable["SETUP_COMPLETE"] = true
}

// HandleConfig is registered for both GET and POST in server.go.
// Go 1.22 ServeMux automatically accepts HEAD for any GET pattern,
// so we must treat HEAD as GET here — otherwise a HEAD request would
// fall through to postConfig and 400 on the empty/missing body.
func (d *Deps) HandleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		d.getConfig(w)
	case http.MethodPost:
		d.postConfig(w, r)
	}
}

func (d *Deps) getConfig(w http.ResponseWriter) {
	env, err := envfile.Read(d.EnvFile)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	config := make(map[string]map[string]string)
	for section, fields := range editableFields {
		config[section] = make(map[string]string)
		for _, f := range fields {
			val := env[f]
			if secretFields[f] && val != "" {
				val = secretPlaceholder
			}
			config[section][f] = val
		}
	}
	config["meta"] = map[string]string{
		"SETUP_COMPLETE": env["SETUP_COMPLETE"],
	}
	writeJSON(w, 200, map[string]any{"config": config})
}

func (d *Deps) postConfig(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if !decodeJSON(w, r, &body) {
		return
	}
	updates := make(map[string]string)
	for k, v := range body {
		if !allEditable[k] {
			continue
		}
		// Ignore submissions of the placeholder value — user didn't
		// actually edit the secret field.
		if secretFields[k] && v == secretPlaceholder {
			continue
		}
		if err := validateField(k, v); err != nil {
			writeError(w, 400, "invalid value for "+k+": "+err.Error())
			return
		}
		updates[k] = v
	}
	if len(updates) == 0 {
		writeError(w, 400, "no valid fields")
		return
	}
	if err := envfile.Write(d.EnvFile, updates); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	keys := make([]string, 0, len(updates))
	for k := range updates {
		keys = append(keys, k)
	}
	writeJSON(w, 200, map[string]any{"updated": keys})
}
