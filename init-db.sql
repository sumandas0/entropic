-- Database initialization script for Entropic
-- This script sets up the initial database configuration

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgvector";

-- Create a dedicated schema for Entropic (optional)
-- CREATE SCHEMA IF NOT EXISTS entropic;

-- Set default privileges for the entropic user
-- GRANT ALL PRIVILEGES ON DATABASE entropic TO postgres;

-- Create database users (if needed)
-- Note: In production, create dedicated users with minimal privileges

-- Example of creating a read-only user for reporting
-- CREATE USER entropic_readonly WITH PASSWORD 'readonly_password';
-- GRANT CONNECT ON DATABASE entropic TO entropic_readonly;
-- GRANT USAGE ON SCHEMA public TO entropic_readonly;
-- GRANT SELECT ON ALL TABLES IN SCHEMA public TO entropic_readonly;
-- ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO entropic_readonly;

-- Set timezone
SET timezone = 'UTC';

-- Configure PostgreSQL settings for better performance
ALTER SYSTEM SET shared_preload_libraries = 'pg_stat_statements';
ALTER SYSTEM SET max_connections = 200;
ALTER SYSTEM SET shared_buffers = '256MB';
ALTER SYSTEM SET effective_cache_size = '1GB';
ALTER SYSTEM SET maintenance_work_mem = '64MB';
ALTER SYSTEM SET checkpoint_completion_target = 0.9;
ALTER SYSTEM SET wal_buffers = '16MB';
ALTER SYSTEM SET default_statistics_target = 100;

-- Log slow queries (adjust as needed)
ALTER SYSTEM SET log_min_duration_statement = 1000; -- Log queries taking more than 1 second
ALTER SYSTEM SET log_statement = 'mod'; -- Log all DDL and DML statements

-- Set up logging for better monitoring
ALTER SYSTEM SET log_destination = 'stderr';
ALTER SYSTEM SET logging_collector = on;
ALTER SYSTEM SET log_directory = 'log';
ALTER SYSTEM SET log_filename = 'postgresql-%Y-%m-%d_%H%M%S.log';
ALTER SYSTEM SET log_truncate_on_rotation = on;
ALTER SYSTEM SET log_rotation_age = 1440; -- 1 day
ALTER SYSTEM SET log_rotation_size = 100MB;

-- Configure connection logging
ALTER SYSTEM SET log_connections = on;
ALTER SYSTEM SET log_disconnections = on;
ALTER SYSTEM SET log_line_prefix = '%t [%p]: [%l-1] user=%u,db=%d,app=%a,client=%h ';

-- Note: RELOAD configuration to apply system settings
-- SELECT pg_reload_conf();