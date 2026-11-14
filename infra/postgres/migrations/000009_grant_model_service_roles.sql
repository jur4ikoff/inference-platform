GRANT USAGE ON SCHEMA public TO model_service_rw, model_service_ro;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.models TO model_service_rw;
GRANT SELECT ON TABLE public.models TO model_service_ro;
