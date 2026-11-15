-- Separate administrator step, before the migrator connects to its own database.
-- No login credentials and no application membership in the owner/purge groups.
BEGIN;
DO $$
DECLARE role_name text; found_role record;
BEGIN
  FOREACH role_name IN ARRAY ARRAY['hp_iam_migrator','hp_iam_app','hp_iam_worker','hp_iam_purge'] LOOP
    SELECT * INTO found_role FROM pg_roles WHERE rolname=role_name;
    IF NOT FOUND THEN
      EXECUTE format('CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS',role_name);
    ELSE
      IF found_role.rolcanlogin OR found_role.rolsuper OR found_role.rolcreatedb
          OR found_role.rolcreaterole OR found_role.rolreplication OR found_role.rolbypassrls THEN
        RAISE EXCEPTION 'IAM_BOOTSTRAP_UNSAFE_EXISTING_ROLE';
      END IF;
    END IF;
    IF EXISTS(SELECT 1 FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.member WHERE r.rolname=role_name) THEN
      RAISE EXCEPTION 'IAM_BOOTSTRAP_UNSAFE_MEMBERSHIP';
    END IF;
  END LOOP;
  EXECUTE format('GRANT CREATE ON DATABASE %I TO hp_iam_migrator',current_database());
END $$;
COMMIT;
