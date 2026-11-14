#!/bin/sh
set -eu

rule='host replication mirror_replicator all scram-sha-256'
if ! grep -Fxq "$rule" "$PGDATA/pg_hba.conf"; then
    printf '\n%s\n' "$rule" >> "$PGDATA/pg_hba.conf"
fi
psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c 'SELECT pg_reload_conf();'
