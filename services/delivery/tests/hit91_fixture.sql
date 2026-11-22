\set ON_ERROR_STOP on
CREATE ROLE hp_delivery_migrator NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
CREATE ROLE hp_delivery_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
CREATE ROLE hp_delivery_worker LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
CREATE ROLE hp_delivery_purge NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
ALTER DATABASE delivery OWNER TO hp_delivery_migrator;
SET ROLE hp_delivery_migrator;
\ir /tmp/001_delivery.sql
RESET ROLE;

CREATE TABLE public.hit91_fail_commit(message_id uuid PRIMARY KEY);
CREATE FUNCTION public.hit91_deferred_failure() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.hit91_fail_commit WHERE message_id=NEW.message_id) THEN
        RAISE EXCEPTION 'fixture deferred failure';
    END IF;
    RETURN NEW;
END;
$$;
CREATE CONSTRAINT TRIGGER hit91_deferred_failure
    AFTER INSERT ON hp_delivery.inbox DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION public.hit91_deferred_failure();
