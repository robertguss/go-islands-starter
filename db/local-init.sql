-- Synthetic local credentials only. Production provisions a separate runtime login.
CREATE ROLE islands_app LOGIN PASSWORD 'local-runtime-only' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOINHERIT;
