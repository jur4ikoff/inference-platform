#!/bin/sh
set -eu

: "${PGDATA:?PGDATA is required}"
: "${REPLICATION_PASSWORD:?REPLICATION_PASSWORD is required}"

if [ ! -s "$PGDATA/PG_VERSION" ]; then
    if [ -n "$(ls -A "$PGDATA" 2>/dev/null)" ]; then
        echo 'Replica data directory is not empty; refusing to overwrite it.' >&2
        exit 1
    fi
    mkdir -p "$PGDATA"
    chown postgres:postgres "$PGDATA"
    chmod 700 "$PGDATA"
    echo 'Creating standby from primary PostgreSQL.'
    export PGPASSWORD="$REPLICATION_PASSWORD"
    gosu postgres pg_basebackup -h postgres -U mirror_replicator -D "$PGDATA" -R -X stream
    unset PGPASSWORD
fi

if [ ! -f "$PGDATA/standby.signal" ]; then
    echo 'Missing standby.signal; refusing to start the replica as a primary.' >&2
    exit 1
fi

echo 'Starting PostgreSQL standby.'
exec docker-entrypoint.sh postgres
