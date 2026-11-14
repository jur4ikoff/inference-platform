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

# The initial, pre-release migration set was consolidated into six baseline
# migrations. Local databases that recorded the old filenames keep their
# schema; only their bookkeeping is translated before the normal loop.
log "Normalizing pre-release migration history"
psql -c "
    DELETE FROM schema_migrations
    WHERE version IN (
        '000006_unique_model_name_per_project.sql',
        '000007_align_users_with_user_service.sql',
        '000008_remove_user_role.sql',
        '000009_add_role_permissions.sql',
        '000010_remove_model_interaction_permission.sql',
        '000012_model_username.sql',
        '000013_audit_event_details.sql',
        '000014_models_created_by.sql'
    );
    UPDATE schema_migrations
    SET version = '000006_create_deployments_table.sql'
    WHERE version = '000011_create_deployments_table.sql';
"

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
    { cat "$migration"; printf "\nINSERT INTO schema_migrations (version) VALUES ('%s');\n" "$version"; } | psql -1
    finished_at=$(date +%s)
    log "SQL completed: $version ($((finished_at - started_at))s)"

    log "Saving migration version: $version"

    log "Migration applied: $version"
done

log "All migrations completed"
log "Configuring the replication role"
psql < "$root_dir/infra/postgres/configure_replication.sql"
docker compose -p "$compose_project" --project-directory "$root_dir" --env-file "$env_file" -f "$compose_file" \
    exec -T postgres sh /docker-entrypoint-initdb.d/30-replication-access.sh
