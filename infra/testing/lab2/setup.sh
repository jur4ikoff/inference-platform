#!/usr/bin/env bash
set -euo pipefail
export PGHOST=postgres PGUSER=test PGPASSWORD=test PGDATABASE=postgres
if [[ "${1:-reset}" != verify ]]; then
# This address exists only on the per-run Compose network. No host DB is used.
dropdb --if-exists --force lab2
createdb lab2
psql -v ON_ERROR_STOP=1 -c "DO \$\$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='model_service_rw') THEN CREATE ROLE model_service_rw; END IF; END \$\$;"
export PGDATABASE=lab2
for migration in /src/infra/postgres/migrations/*.sql; do
  psql -v ON_ERROR_STOP=1 -f "$migration"
done
printf '%s\n' 'lab2-password' | bootstrap-admin --login labadmin --email labadmin@example.test --firstname Lab --lastname Admin --password-stdin
fi
export PGDATABASE=lab2
psql -v ON_ERROR_STOP=1 <<'SQL'
DO $$ BEGIN
  IF (SELECT count(*) FROM users) <> 1
     OR (SELECT count(*) FROM users WHERE login='labadmin' AND platform_role='admin' AND password_hash LIKE '$argon2id$%') <> 1
     OR (SELECT count(*) FROM roles) <> 3
     OR (SELECT count(*) FROM roles WHERE system_key IN ('teamlead','developer','viewer')) <> 3
     OR (SELECT count(*) FROM projects) <> 0
     OR (SELECT count(*) FROM members) <> 0
     OR (SELECT count(*) FROM models) <> 0
     OR (SELECT count(*) FROM deployments) <> 0
     OR (SELECT count(*) FROM deployment_idempotency) <> 0
     OR (SELECT count(*) FROM audit_events) <> 1
     OR (SELECT count(*) FROM audit_events WHERE action='user.bootstrap_admin_created') <> 1
  THEN RAISE EXCEPTION 'database differs from S0'; END IF;
END $$;
SELECT 'S0 verified: one admin, three roles, no test data' AS restoration;
SQL
mkdir -p /results/keys
if [[ ! -f /results/keys/jwt.pem ]]; then
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out /results/keys/jwt.pem
fi
