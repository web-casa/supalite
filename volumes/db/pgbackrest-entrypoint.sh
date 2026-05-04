#!/usr/bin/env bash
# pgBackRest entrypoint wrapper.
#
# Substitutes BACKUP_S3_* env vars into pgbackrest.conf, then hands
# off to the supabase/postgres original entrypoint (docker-entrypoint.sh).
#
# The substitution is deliberately verbose (one sed per placeholder)
# instead of envsubst — envsubst isn't in the base image, and we
# want to do exact-replacements on placeholders that contain no
# special regex chars.

set -eu -o pipefail

tmpl=/etc/pgbackrest/pgbackrest.conf.template
out=/etc/pgbackrest/pgbackrest.conf

if [[ -f "$tmpl" ]]; then
  # Strip scheme (https://host → host) for pgBackRest's repo1-s3-endpoint.
  endpoint_host="${BACKUP_S3_ENDPOINT:-}"
  endpoint_host="${endpoint_host#https://}"
  endpoint_host="${endpoint_host#http://}"

  # URI style: pgBackRest wants "host" (vhost-style) or "path".
  # Our BACKUP_S3_PATH_STYLE="true" means path-style.
  if [[ "${BACKUP_S3_PATH_STYLE:-false}" == "true" ]]; then
    uri_style="path"
  else
    uri_style="host"
  fi

  # Prefix for pgBackRest inside the S3 bucket — separate from pg_dump
  # prefix so they don't stomp each other.
  pgbackrest_path="${BACKUP_S3_PGBACKREST_PATH:-/pgbackrest}"

  # Escape special characters before feeding to sed's replacement side.
  # `&` means "entire match", `\` starts backreferences, `|` is our
  # delimiter. Standard S3 keys/paths don't contain these, but we
  # harden the substitution defensively for any future field.
  sed_escape() {
    printf '%s' "$1" | sed -e 's/[\\&|]/\\&/g'
  }

  sed \
    -e "s|__BACKUP_S3_ENDPOINT_HOST__|$(sed_escape "$endpoint_host")|g" \
    -e "s|__BACKUP_S3_BUCKET__|$(sed_escape "${BACKUP_S3_BUCKET:-}")|g" \
    -e "s|__BACKUP_S3_REGION__|$(sed_escape "${BACKUP_S3_REGION:-us-east-1}")|g" \
    -e "s|__BACKUP_S3_ACCESS_KEY__|$(sed_escape "${BACKUP_S3_ACCESS_KEY:-}")|g" \
    -e "s|__BACKUP_S3_SECRET_KEY__|$(sed_escape "${BACKUP_S3_SECRET_KEY:-}")|g" \
    -e "s|__BACKUP_S3_URI_STYLE__|$(sed_escape "$uri_style")|g" \
    -e "s|__BACKUP_S3_PGBACKREST_PATH__|$(sed_escape "$pgbackrest_path")|g" \
    "$tmpl" > "$out"
  chmod 640 "$out"
fi

# If the first arg isn't `postgres`, we're in tool mode — a one-shot
# container launched by the admin panel to run pgbackrest directly
# (typically `pgbackrest --stanza=main --set=... restore`). Execute
# the command as-is; skipping the upstream docker-entrypoint.sh which
# would try to interpret tool args as postgres options.
if [[ "${1:-}" != "postgres" ]]; then
  exec "$@"
fi

# Normal server mode: hand off to the original entrypoint that
# supabase/postgres uses.
exec docker-entrypoint.sh "$@"
