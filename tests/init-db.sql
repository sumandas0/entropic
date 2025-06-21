-- Test database initialization script for PostgreSQL with pgvector
-- This script sets up the test environment for Entropic

-- Create pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Create test user (if not exists from container setup)
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'test') THEN
        CREATE USER test WITH PASSWORD 'test';
    END IF;
END
$$;

-- Grant necessary permissions
GRANT ALL PRIVILEGES ON DATABASE entropic_test TO test;
GRANT ALL ON SCHEMA public TO test;

-- Set search path
ALTER DATABASE entropic_test SET search_path TO public;

-- Optional: Create additional test-specific configurations
ALTER SYSTEM SET shared_preload_libraries = 'pg_stat_statements';