-- Executed once by PostgreSQL's entrypoint as POSTGRES_USER in POSTGRES_DB.
\getenv rw_password MODEL_SERVICE_RW_PASSWORD

SELECT format('CREATE ROLE model_service_rw LOGIN PASSWORD %L', :'rw_password') \gexec
