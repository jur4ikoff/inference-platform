"""Restore the isolated PostgreSQL database and verify its seeded state."""
import os
from pathlib import Path
import subprocess

RESULTS = Path("/results")
MIGRATIONS = Path(__file__).resolve().parents[1] / "postgres" / "migrations"
ADMIN_LOGIN = "labadmin"
ADMIN_PASSWORD = "lab2-password"

VERIFY_STATE = """
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
"""


def prepare_database(*, verify_only=False, stdout=None):
    # This host resolves only inside the per-run Compose network.
    env = dict(os.environ, PGHOST="postgres", PGUSER="test", PGPASSWORD="test")

    def run(*args, database="lab2", input_text=None):
        subprocess.run(
            args, env=dict(env, PGDATABASE=database), text=True,
            input=input_text, stdout=stdout, stderr=subprocess.STDOUT, check=True,
        )

    if not verify_only:
        run("dropdb", "--if-exists", "--force", "lab2", database="postgres")
        run("createdb", "lab2", database="postgres")
        for migration in sorted(MIGRATIONS.glob("*.sql")):
            run("psql", "-v", "ON_ERROR_STOP=1", "-f", str(migration))
        run(
            "bootstrap-admin", "--login", ADMIN_LOGIN, "--email", "labadmin@example.test",
            "--firstname", "Lab", "--lastname", "Admin", "--password-stdin",
            input_text=ADMIN_PASSWORD + "\n",
        )

    run("psql", "-v", "ON_ERROR_STOP=1", input_text=VERIFY_STATE)
    if not verify_only:
        key = RESULTS / "keys" / "jwt.pem"
        key.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        if not key.is_file():
            run("openssl", "genpkey", "-algorithm", "RSA", "-pkeyopt", "rsa_keygen_bits:2048", "-out", str(key))
        # The production user-service runs unprivileged and mounts only this file.
        key.chmod(0o644)
