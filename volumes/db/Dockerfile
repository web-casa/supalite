# Custom Postgres image: supabase/postgres + pgBackRest.
#
# Why a custom image: pgBackRest's standard architecture pushes WAL
# segments from postgres via archive_command, which means the binary
# must live in the same container as postgres. Stock supabase/postgres
# doesn't ship it. Adding ~5 MB for everyone is cheaper than inventing
# a sidecar-only WAL pipeline.
#
# The binary is present regardless of whether the operator has
# enabled WAL archiving (PGBACKREST_* env vars in .env). When disabled
# nothing runs — just extra disk.
FROM supabase/postgres:15.8.1.085

USER root
RUN apt-get update \
    && apt-get install -y --no-install-recommends pgbackrest \
    && rm -rf /var/lib/apt/lists/*

# apt's postgresql-common (pulled in transitively by pgbackrest) installs
# a pg-wrapper stub at /usr/bin/pg_isready that expects an apt-managed
# postgresql-client. But supabase/postgres ships its Postgres binaries
# through Nix under /nix/var/nix/profiles/default/bin — so the shim
# always errors with "You must install at least one postgresql-client".
# Put the Nix profile first so the real binaries win.
ENV PATH="/nix/var/nix/profiles/default/bin:${PATH}"

# Entrypoint wrapper: substitutes BACKUP_S3_* env values into
# /etc/pgbackrest/pgbackrest.conf at startup, then chains to the
# original supabase/postgres entrypoint.
COPY pgbackrest-entrypoint.sh /usr/local/bin/pgbackrest-entrypoint.sh
RUN chmod +x /usr/local/bin/pgbackrest-entrypoint.sh \
    && mkdir -p /etc/pgbackrest /var/log/pgbackrest /var/lib/pgbackrest \
    && chown -R postgres:postgres /etc/pgbackrest /var/log/pgbackrest /var/lib/pgbackrest

COPY pgbackrest.conf.template /etc/pgbackrest/pgbackrest.conf.template

# Stay as root so the upstream docker-entrypoint.sh can chown
# /var/lib/postgresql/data on first start, then gosu-drop to the
# postgres user before exec'ing postgres. Setting USER postgres here
# skips that fix-up and initdb fails with "could not change
# permissions of directory" on a fresh named volume.
ENTRYPOINT ["/usr/local/bin/pgbackrest-entrypoint.sh"]
# Default CMD inherited from supabase/postgres (postgres with its args).
