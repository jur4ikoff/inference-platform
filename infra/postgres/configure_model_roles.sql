-- Executed once by PostgreSQL's entrypoint as POSTGRES_USER in POSTGRES_DB.
\getenv rw_password MODEL_SERVICE_RW_PASSWORD
\getenv ro_password MODEL_SERVICE_RO_PASSWORD

SELECT format('CREATE ROLE model_service_rw LOGIN PASSWORD %L', :'rw_password') \gexec
SELECT format('CREATE ROLE model_service_ro LOGIN PASSWORD %L', :'ro_password') \gexec
ALTER ROLE model_service_ro SET default_transaction_read_only = on;
