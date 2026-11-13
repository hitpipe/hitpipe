-- Run once as the administrator of a NEW dedicated Processing cluster/database.
-- This is role provisioning, deliberately separate from the immutable migration.
-- No credentials here. Test runner uses only an isolated private Unix socket.
BEGIN;
CREATE ROLE hp_processing_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
CREATE ROLE hp_processing_migrator LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
CREATE ROLE hp_processing_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
CREATE ROLE hp_processing_worker LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
CREATE ROLE hp_processing_purge LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
GRANT hp_processing_owner TO hp_processing_migrator;
COMMIT;
