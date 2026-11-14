#!/usr/bin/env sh
set -eu

root_dir=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
compose_file="$root_dir/infra/docker-compose.yaml"
env_file="$root_dir/infra/.env"

psql_on() {
    service=$1
    shift
    docker compose -p inference-platform --project-directory "$root_dir" \
        --env-file "$env_file" -f "$compose_file" exec -T "$service" \
        sh -c 'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" "$@"' psql "$@"
}

echo 'Primary replication status:'
psql_on postgres -c "SELECT application_name, state, sync_state, pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) AS lag_bytes FROM pg_stat_replication;"
if [ "$(psql_on postgres -tAc "SELECT count(*) FROM pg_stat_replication WHERE state = 'streaming';")" -eq 0 ]; then
    echo 'No standby is streaming from the primary.' >&2
    exit 1
fi

echo 'Standby status:'
psql_on postgres-2 -c 'SELECT pg_is_in_recovery() AS is_standby, status AS wal_receiver FROM pg_stat_wal_receiver;'
if [ "$(psql_on postgres-2 -tAc "SELECT pg_is_in_recovery() AND EXISTS (SELECT 1 FROM pg_stat_wal_receiver WHERE status = 'streaming');")" != 't' ]; then
    echo 'postgres-2 is not a streaming standby.' >&2
    exit 1
fi

if result=$(psql_on postgres-2 -v VERBOSITY=sqlstate -c 'UPDATE public.models SET name = name WHERE false;' 2>&1); then
    echo 'Unexpected success: the standby accepted a write.' >&2
    exit 1
fi
case "$result" in
    *25006*) echo 'Standby rejected the write: SQLSTATE 25006 (read-only transaction).' ;;
    *) printf 'Unexpected error: %s\n' "$result" >&2; exit 1 ;;
esac
