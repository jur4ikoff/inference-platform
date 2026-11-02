#!/usr/bin/env sh
set -eu

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
compose_file="$root_dir/infra/docker-compose.yaml"
migrations_dir="$root_dir/infra/postgres/migrations"
env_file="$root_dir/infra/.env"
compose_project="inference-platform"

log() {
    printf '[%s] %s\n' "$(date '+%Y-%m-%dT%H:%M:%S%z')" "$*"
}

psql() {
    docker compose -p "$compose_project" --project-directory "$root_dir" --env-file "$env_file" -f "$compose_file" exec -T postgres \
        sh -c 'exec psql -X -v ON_ERROR_STOP=1 -v VERBOSITY=verbose -U "$POSTGRES_USER" -d "$POSTGRES_DB" "$@"' psql "$@"
}

log "Checking PostgreSQL connection"
psql -c 'SELECT current_database(), current_user, now();'

log "Creating migration history table"
psql -c '
    CREATE TABLE IF NOT EXISTS schema_migrations (
        version TEXT PRIMARY KEY,
        applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
    );
'

for migration in "$migrations_dir"/*.sql; do
    [ -f "$migration" ] || continue

    version=$(basename "$migration")
    log "Checking migration: $version"
    applied=$(psql -tAc "SELECT 1 FROM schema_migrations WHERE version = '$version'")
    if [ "$applied" = "1" ]; then
        log "Skipped (already applied): $version"
        continue
    fi

    started_at=$(date +%s)
    log "Applying: $version"
    psql -a -e < "$migration"
    finished_at=$(date +%s)
    log "SQL completed: $version ($((finished_at - started_at))s)"

    log "Saving migration version: $version"
    psql -c "INSERT INTO schema_migrations (version) VALUES ('$version')"
    log "Migration applied: $version"
done

log "All migrations completed"
