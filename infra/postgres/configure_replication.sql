\getenv replication_password REPLICATION_PASSWORD

SELECT 'CREATE ROLE mirror_replicator LOGIN REPLICATION'
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mirror_replicator')
\gexec
SELECT format('ALTER ROLE mirror_replicator LOGIN REPLICATION NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS PASSWORD %L', :'replication_password')
\gexec
