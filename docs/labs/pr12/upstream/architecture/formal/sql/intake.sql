-- HitPipe intake persistence contract v1. Applies ONLY to its dedicated database.
-- Migrator owns schema/tables; hp_intake_app and hp_intake_worker are NOT owners,
-- NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS. Bootstrap roles before migration.
-- hp_intake_purge is a separately authorized maintenance role, never inherited by app/worker.
-- Real event/context/history/proof/outbox payloads and encrypted secrets live ONLY in
-- service-owned fsync/atomic-rename content files under BP006, never SQL/WAL/SQL backups.
-- SQL retains opaque refs/hashes/size/expiry plus minimal safety/accepted-rule state.
-- App/scoped workers SET LOCAL hp.workspace_id after authoritative scope validation.
-- Global job_index exposes ONLY tenant/job IDs and due/claim metadata, never payload/PII.
BEGIN;
CREATE SCHEMA hp_intake;
REVOKE ALL ON SCHEMA hp_intake FROM PUBLIC;
GRANT USAGE ON SCHEMA hp_intake TO hp_intake_app,hp_intake_worker,hp_intake_purge;
SET LOCAL search_path=hp_intake,pg_catalog;
CREATE TABLE schema_version(version integer PRIMARY KEY CHECK(version=1),installed_at timestamptz NOT NULL DEFAULT clock_timestamp());
INSERT INTO schema_version(version) VALUES(1);

CREATE TABLE source_connections(workspace_id uuid NOT NULL,connection_id uuid NOT NULL,account_id text NOT NULL CHECK(length(account_id)<=255),livemode boolean NOT NULL,security_epoch bigint NOT NULL CHECK(security_epoch>0),health text NOT NULL CHECK(health IN('CHECKING','READY','DEGRADED','REVOKED','UNSUPPORTED')),verified_at timestamptz,revoked_at timestamptz,PRIMARY KEY(workspace_id,connection_id));
CREATE TABLE connection_secrets(workspace_id uuid NOT NULL,secret_id uuid NOT NULL,connection_id uuid NOT NULL,sealed_secret_ref uuid NOT NULL,sealed_secret_sha256 text NOT NULL CHECK(sealed_secret_sha256~'^sha256:[0-9a-f]{64}$'),key_id text NOT NULL CHECK(length(key_id)<=120),secret_kind text NOT NULL CHECK(secret_kind IN('STRIPE_READ_KEY','WEBHOOK_SIGNING_SECRET')),created_at timestamptz NOT NULL,expires_at timestamptz,PRIMARY KEY(workspace_id,secret_id),FOREIGN KEY(workspace_id,connection_id) REFERENCES source_connections(workspace_id,connection_id));
CREATE TABLE ingress_quota(workspace_id uuid PRIMARY KEY,signal_count integer NOT NULL DEFAULT 0 CHECK(signal_count BETWEEN 0 AND 1000),serialized_bytes bigint NOT NULL DEFAULT 0 CHECK(serialized_bytes BETWEEN 0 AND 16777216),updated_at timestamptz NOT NULL DEFAULT clock_timestamp());
CREATE TABLE signals(workspace_id uuid NOT NULL,signal_id uuid NOT NULL,connection_id uuid NOT NULL,event_id text NOT NULL CHECK(length(event_id)<=255),event_type text NOT NULL CHECK(event_type IN('checkout.session.completed','checkout.session.async_payment_succeeded','checkout.session.async_payment_failed')),session_id text NOT NULL CHECK(length(session_id)<=255),account_id text NOT NULL CHECK(length(account_id)<=255),livemode boolean NOT NULL,source_created_at timestamptz NOT NULL,first_received_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,serialized_bytes integer NOT NULL CHECK(serialized_bytes BETWEEN 1 AND 16777216),state text NOT NULL CHECK(state IN('RECEIVED','HELD','UNRESOLVED','APPLIED','NOT_APPLICABLE','EXPIRED')),case_id uuid,reason_code text NOT NULL CHECK(length(reason_code)<=120),PRIMARY KEY(workspace_id,signal_id),UNIQUE(workspace_id,connection_id,event_id),FOREIGN KEY(workspace_id,connection_id) REFERENCES source_connections(workspace_id,connection_id),CHECK(expires_at<=first_received_at+interval'72hours'));
CREATE INDEX signals_expiry ON signals(expires_at) WHERE state IN('RECEIVED','HELD','UNRESOLVED');
-- No raw body/customer contact/brief column exists in signals.
CREATE TABLE source_facts(workspace_id uuid NOT NULL,facts_id uuid NOT NULL,case_id uuid NOT NULL,connection_id uuid NOT NULL,facts_ref uuid NOT NULL,facts_bytes integer NOT NULL CHECK(facts_bytes BETWEEN 1 AND 65536),digest text NOT NULL CHECK(digest~'^sha256:[0-9a-f]{64}$'),complete boolean NOT NULL,oldest_observed_at timestamptz NOT NULL,completed_at timestamptz NOT NULL,valid_until timestamptz NOT NULL,expires_at timestamptz NOT NULL,PRIMARY KEY(workspace_id,facts_id),FOREIGN KEY(workspace_id,connection_id) REFERENCES source_connections(workspace_id,connection_id),CHECK(valid_until<=oldest_observed_at+interval'10seconds'),CHECK(completed_at>=oldest_observed_at),CHECK(expires_at<=completed_at+interval'90days'));
CREATE TABLE gap_markers(workspace_id uuid NOT NULL,gap_id uuid NOT NULL,start_at timestamptz NOT NULL,end_at timestamptz,reason_code text NOT NULL CHECK(length(reason_code)<=120),known_count bigint CHECK(known_count>=0),expires_at timestamptz NOT NULL,PRIMARY KEY(workspace_id,gap_id));
CREATE FUNCTION maintain_signal_quota() RETURNS trigger LANGUAGE plpgsql SET search_path=hp_intake,pg_catalog AS $$
DECLARE old_n integer:=0;new_n integer:=0;old_b bigint:=0;new_b bigint:=0;ws uuid;
BEGIN
 IF TG_OP<>'INSERT' AND OLD.state IN('RECEIVED','HELD','UNRESOLVED') THEN old_n:=1;old_b:=OLD.serialized_bytes;END IF;
 IF TG_OP<>'DELETE' AND NEW.state IN('RECEIVED','HELD','UNRESOLVED') THEN new_n:=1;new_b:=NEW.serialized_bytes;END IF;
 ws:=CASE WHEN TG_OP='DELETE' THEN OLD.workspace_id ELSE NEW.workspace_id END;
 INSERT INTO ingress_quota(workspace_id,signal_count,serialized_bytes) VALUES(ws,new_n,new_b) ON CONFLICT(workspace_id) DO UPDATE SET signal_count=ingress_quota.signal_count+new_n-old_n,serialized_bytes=ingress_quota.serialized_bytes+new_b-old_b,updated_at=clock_timestamp();
 IF TG_OP='DELETE' THEN RETURN OLD;ELSE RETURN NEW;END IF;
END $$;
CREATE TRIGGER signal_quota AFTER INSERT OR UPDATE OF state,serialized_bytes OR DELETE ON signals FOR EACH ROW EXECUTE FUNCTION maintain_signal_quota();
CREATE FUNCTION immutable_receipt() RETURNS trigger LANGUAGE plpgsql SET search_path=hp_intake,pg_catalog AS $$ BEGIN
 IF (NEW.workspace_id,NEW.signal_id,NEW.connection_id,NEW.event_id,NEW.session_id,NEW.first_received_at,NEW.expires_at) IS DISTINCT FROM (OLD.workspace_id,OLD.signal_id,OLD.connection_id,OLD.event_id,OLD.session_id,OLD.first_received_at,OLD.expires_at) THEN RAISE EXCEPTION 'receipt identity/first age immutable';END IF;RETURN NEW;END $$;
CREATE TRIGGER signal_identity BEFORE UPDATE ON signals FOR EACH ROW EXECUTE FUNCTION immutable_receipt();

CREATE TABLE inbox(workspace_id uuid NOT NULL,message_id uuid NOT NULL,digest text NOT NULL CHECK(digest~'^sha256:[0-9a-f]{64}$'),disposition text NOT NULL CHECK(disposition IN('APPLIED','HELD','REJECTED','CONFLICT')),record_id uuid,received_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(workspace_id,message_id));
CREATE TABLE outbox(workspace_id uuid NOT NULL,message_id uuid NOT NULL,kind text NOT NULL,payload_ref uuid NOT NULL,payload_sha256 text NOT NULL CHECK(payload_sha256~'^sha256:[0-9a-f]{64}$'),payload_bytes integer NOT NULL CHECK(payload_bytes BETWEEN 1 AND 65536),digest text NOT NULL CHECK(digest~'^sha256:[0-9a-f]{64}$'),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL,state text NOT NULL DEFAULT 'PENDING' CHECK(state IN('PENDING','CLAIMED','ACKED','HELD')),claim_token uuid,lease_until timestamptz,delivery_count integer NOT NULL DEFAULT 0 CHECK(delivery_count>=0),PRIMARY KEY(workspace_id,message_id),CHECK(expires_at>created_at));
CREATE INDEX outbox_due ON outbox(next_attempt_at) WHERE state IN('PENDING','CLAIMED');
-- Only workers see this global index. They must open a NEW scoped transaction before payload access.
CREATE TABLE job_index(job_id uuid PRIMARY KEY,workspace_id uuid NOT NULL,kind text NOT NULL CHECK(length(kind)<=80),due_at timestamptz NOT NULL,claim_token uuid,lease_until timestamptz);
CREATE INDEX job_index_due ON job_index(due_at);

ALTER TABLE source_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE source_connections FORCE ROW LEVEL SECURITY;
CREATE POLICY source_connections_tenant ON source_connections USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON source_connections TO hp_intake_app,hp_intake_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON source_connections TO hp_intake_purge;
ALTER TABLE connection_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE connection_secrets FORCE ROW LEVEL SECURITY;
CREATE POLICY connection_secrets_tenant ON connection_secrets USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON connection_secrets TO hp_intake_app,hp_intake_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON connection_secrets TO hp_intake_purge;
ALTER TABLE ingress_quota ENABLE ROW LEVEL SECURITY;
ALTER TABLE ingress_quota FORCE ROW LEVEL SECURITY;
CREATE POLICY ingress_quota_tenant ON ingress_quota USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON ingress_quota TO hp_intake_app,hp_intake_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON ingress_quota TO hp_intake_purge;
ALTER TABLE signals ENABLE ROW LEVEL SECURITY;
ALTER TABLE signals FORCE ROW LEVEL SECURITY;
CREATE POLICY signals_tenant ON signals USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON signals TO hp_intake_app,hp_intake_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON signals TO hp_intake_purge;
ALTER TABLE source_facts ENABLE ROW LEVEL SECURITY;
ALTER TABLE source_facts FORCE ROW LEVEL SECURITY;
CREATE POLICY source_facts_tenant ON source_facts USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT ON source_facts TO hp_intake_app,hp_intake_worker;
GRANT SELECT,DELETE ON source_facts TO hp_intake_purge;
ALTER TABLE gap_markers ENABLE ROW LEVEL SECURITY;
ALTER TABLE gap_markers FORCE ROW LEVEL SECURITY;
CREATE POLICY gap_markers_tenant ON gap_markers USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON gap_markers TO hp_intake_app,hp_intake_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON gap_markers TO hp_intake_purge;
ALTER TABLE inbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbox FORCE ROW LEVEL SECURITY;
CREATE POLICY inbox_tenant ON inbox USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON inbox TO hp_intake_app,hp_intake_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON inbox TO hp_intake_purge;
ALTER TABLE outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox FORCE ROW LEVEL SECURITY;
CREATE POLICY outbox_tenant ON outbox USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON outbox TO hp_intake_app,hp_intake_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON outbox TO hp_intake_purge;
GRANT SELECT,INSERT,UPDATE,DELETE ON job_index TO hp_intake_worker;
GRANT SELECT,INSERT,UPDATE,DELETE ON job_index TO hp_intake_purge;
GRANT SELECT ON schema_version TO hp_intake_app,hp_intake_worker,hp_intake_purge;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA hp_intake FROM PUBLIC;
COMMIT;
