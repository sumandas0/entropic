CREATE EXTENSION IF NOT EXISTS vector;

DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'test') THEN
        CREATE USER test WITH PASSWORD 'test';
    END IF;
END
$$;

GRANT ALL PRIVILEGES ON DATABASE entropic_test TO test;
GRANT ALL ON SCHEMA public TO test;

ALTER DATABASE entropic_test SET search_path TO public;

ALTER SYSTEM SET shared_preload_libraries = 'pg_stat_statements';
