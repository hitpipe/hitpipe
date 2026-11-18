-- HitPipe delivery persistence contract v1. Applies ONLY to its dedicated database.
-- Migrator owns schema/tables; hp_delivery_app and hp_delivery_worker are NOT owners,
-- NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS. Bootstrap roles before migration.
-- hp_delivery_purge is a separately authorized maintenance role, never inherited by app/worker.
-- Real event/context/history/proof/outbox payloads and encrypted secrets live ONLY in
-- service-owned fsync/atomic-rename content files under BP006, never SQL/WAL/SQL backups.
-- SQL retains opaque refs/hashes/size/expiry plus minimal safety/accepted-rule state.
-- App/scoped workers SET LOCAL hp.workspace_id after authoritative scope validation.
-- Global job_index exposes ONLY tenant/job IDs and due/claim metadata, never payload/PII.
BEGIN;
CREATE SCHEMA hp_delivery;
REVOKE ALL ON SCHEMA hp_delivery FROM PUBLIC;
GRANT USAGE ON SCHEMA hp_delivery TO hp_delivery_app,hp_delivery_worker,hp_delivery_purge;
SET LOCAL search_path=hp_delivery,pg_catalog;
CREATE TABLE schema_version(version integer PRIMARY KEY CHECK(version=1),installed_at timestamptz NOT NULL DEFAULT clock_timestamp());
INSERT INTO schema_version(version) VALUES(1);

CREATE TABLE destination_connections(workspace_id uuid NOT NULL,connection_id uuid NOT NULL,external_workspace_id text NOT NULL CHECK(length(external_workspace_id)<=255),security_epoch bigint NOT NULL CHECK(security_epoch>0),token_fingerprint bytea NOT NULL CHECK(octet_length(token_fingerprint)=32),health text NOT NULL,revoked_at timestamptz,PRIMARY KEY(workspace_id,connection_id));
CREATE TABLE connection_secrets(workspace_id uuid NOT NULL,secret_id uuid NOT NULL,connection_id uuid NOT NULL,sealed_secret_ref uuid NOT NULL,sealed_secret_sha256 text NOT NULL CHECK(sealed_secret_sha256~'^sha256:[0-9a-f]{64}$'),key_id text NOT NULL,created_at timestamptz NOT NULL,expires_at timestamptz,PRIMARY KEY(workspace_id,secret_id),FOREIGN KEY(workspace_id,connection_id) REFERENCES destination_connections(workspace_id,connection_id));
CREATE TABLE effect_lineage(workspace_id uuid NOT NULL,lineage_id uuid NOT NULL,case_id uuid NOT NULL,connection_id uuid NOT NULL,effect_kind text NOT NULL CHECK(effect_kind='CREATE_TASK'),config_id uuid NOT NULL,state text NOT NULL CHECK(state IN('PREPARED','DISPATCHING','KNOWN_NO_EFFECT','RESULT_ID_OBSERVED','AMBIGUOUS','TARGET_OBSERVED')),ever_dispatch_fenced boolean NOT NULL DEFAULT false,known_task_id text,no_effect_proof_ref uuid,updated_at timestamptz NOT NULL,PRIMARY KEY(workspace_id,lineage_id),UNIQUE(workspace_id,case_id,effect_kind),FOREIGN KEY(workspace_id,connection_id) REFERENCES destination_connections(workspace_id,connection_id),CHECK(state<>'KNOWN_NO_EFFECT' OR no_effect_proof_ref IS NOT NULL));
CREATE TABLE attempts(workspace_id uuid NOT NULL,attempt_id uuid NOT NULL,lineage_id uuid NOT NULL,episode_id uuid NOT NULL,attempt_number integer NOT NULL CHECK(attempt_number BETWEEN 1 AND 3),command_message_id uuid NOT NULL,source_facts_digest text NOT NULL CHECK(source_facts_digest~'^sha256:[0-9a-f]{64}$'),facts_valid_until timestamptz NOT NULL,episode_deadline_at timestamptz NOT NULL,connection_epoch bigint NOT NULL,prepared_at timestamptz NOT NULL,dispatch_fenced_at timestamptz,network_start_at timestamptz,PRIMARY KEY(workspace_id,attempt_id),UNIQUE(workspace_id,lineage_id,episode_id,attempt_number),UNIQUE(workspace_id,command_message_id),FOREIGN KEY(workspace_id,lineage_id) REFERENCES effect_lineage(workspace_id,lineage_id));
CREATE TABLE effect_observations(workspace_id uuid NOT NULL,observation_id uuid NOT NULL,lineage_id uuid NOT NULL,attempt_id uuid,knowledge text NOT NULL CHECK(knowledge IN('KNOWN_NO_EFFECT','POSSIBLE_EFFECT','TARGET_ID_OBSERVED','RESULT_VERIFIED','RESULT_MISMATCH','INCOMPLETE')),observed_at timestamptz NOT NULL,proof_code text NOT NULL CHECK(proof_code~'^[A-Z0-9_]{1,120}$'),proof_digest text NOT NULL CHECK(proof_digest~'^sha256:[0-9a-f]{64}$'),known_task_id text,PRIMARY KEY(workspace_id,observation_id),FOREIGN KEY(workspace_id,lineage_id) REFERENCES effect_lineage(workspace_id,lineage_id),FOREIGN KEY(workspace_id,attempt_id) REFERENCES attempts(workspace_id,attempt_id));
CREATE TABLE observation_content(workspace_id uuid NOT NULL,observation_id uuid NOT NULL,actual_attributes_ref uuid NOT NULL,actual_attributes_sha256 text NOT NULL CHECK(actual_attributes_sha256~'^sha256:[0-9a-f]{64}$'),actual_attributes_bytes integer NOT NULL CHECK(actual_attributes_bytes BETWEEN 1 AND 65536),expected_attributes_digest text NOT NULL,created_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,PRIMARY KEY(workspace_id,observation_id),FOREIGN KEY(workspace_id,observation_id) REFERENCES effect_observations(workspace_id,observation_id),CHECK(expires_at<=created_at+interval'90days'));
CREATE TABLE intent_content(workspace_id uuid NOT NULL,attempt_id uuid NOT NULL,rendered_payload_ref uuid NOT NULL,rendered_payload_sha256 text NOT NULL CHECK(rendered_payload_sha256~'^sha256:[0-9a-f]{64}$'),rendered_payload_bytes integer NOT NULL CHECK(rendered_payload_bytes BETWEEN 1 AND 65536),created_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,PRIMARY KEY(workspace_id,attempt_id),FOREIGN KEY(workspace_id,attempt_id) REFERENCES attempts(workspace_id,attempt_id),CHECK(expires_at<=created_at+interval'90days'));
-- One service-wide token budget across workspaces; worker-only pseudonymous fingerprint + counters.
CREATE TABLE credential_budget(token_fingerprint bytea PRIMARY KEY CHECK(octet_length(token_fingerprint)=32),window_started_at timestamptz NOT NULL,limit_count integer NOT NULL CHECK(limit_count>0),used_count integer NOT NULL CHECK(used_count>=0),next_eligible_at timestamptz NOT NULL);
CREATE FUNCTION protect_effect_fence() RETURNS trigger LANGUAGE plpgsql SET search_path=hp_delivery,pg_catalog AS $$ BEGIN
 IF (NEW.workspace_id,NEW.lineage_id,NEW.case_id,NEW.connection_id,NEW.effect_kind,NEW.config_id) IS DISTINCT FROM (OLD.workspace_id,OLD.lineage_id,OLD.case_id,OLD.connection_id,OLD.effect_kind,OLD.config_id) OR (OLD.ever_dispatch_fenced AND NOT NEW.ever_dispatch_fenced) THEN RAISE EXCEPTION 'effect lineage/fence immutable';END IF;
 IF OLD.state IN('DISPATCHING','AMBIGUOUS','RESULT_ID_OBSERVED','TARGET_OBSERVED') AND NEW.state='PREPARED' THEN RAISE EXCEPTION 'possible effect cannot be reset to prepared';END IF;
 IF NEW.state='DISPATCHING' THEN NEW.ever_dispatch_fenced:=true;END IF;
 RETURN NEW;END $$;
CREATE TRIGGER effect_fence_immutable BEFORE UPDATE ON effect_lineage FOR EACH ROW EXECUTE FUNCTION protect_effect_fence();

CREATE TABLE inbox(workspace_id uuid NOT NULL,message_id uuid NOT NULL,digest text NOT NULL CHECK(digest~'^sha256:[0-9a-f]{64}$'),disposition text NOT NULL CHECK(disposition IN('APPLIED','HELD','REJECTED','CONFLICT')),record_id uuid,received_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(workspace_id,message_id));
CREATE TABLE outbox(workspace_id uuid NOT NULL,message_id uuid NOT NULL,kind text NOT NULL,payload_ref uuid NOT NULL,payload_sha256 text NOT NULL CHECK(payload_sha256~'^sha256:[0-9a-f]{64}$'),payload_bytes integer NOT NULL CHECK(payload_bytes BETWEEN 1 AND 65536),digest text NOT NULL CHECK(digest~'^sha256:[0-9a-f]{64}$'),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL,state text NOT NULL DEFAULT 'PENDING' CHECK(state IN('PENDING','CLAIMED','ACKED','HELD')),claim_token uuid,lease_until timestamptz,delivery_count integer NOT NULL DEFAULT 0 CHECK(delivery_count>=0),PRIMARY KEY(workspace_id,message_id),CHECK(expires_at>created_at));
CREATE INDEX outbox_due ON outbox(next_attempt_at) WHERE state IN('PENDING','CLAIMED');
-- Only workers see this global index. They must open a NEW scoped transaction before payload access.
CREATE TABLE job_index(job_id uuid PRIMARY KEY,workspace_id uuid NOT NULL,kind text NOT NULL CHECK(length(kind)<=80),due_at timestamptz NOT NULL,claim_token uuid,lease_until timestamptz);
CREATE INDEX job_index_due ON job_index(due_at);

ALTER TABLE destination_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE destination_connections FORCE ROW LEVEL SECURITY;
CREATE POLICY destination_connections_tenant ON destination_connections USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON destination_connections TO hp_delivery_app,hp_delivery_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON destination_connections TO hp_delivery_purge;
ALTER TABLE connection_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE connection_secrets FORCE ROW LEVEL SECURITY;
CREATE POLICY connection_secrets_tenant ON connection_secrets USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON connection_secrets TO hp_delivery_app,hp_delivery_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON connection_secrets TO hp_delivery_purge;
ALTER TABLE effect_lineage ENABLE ROW LEVEL SECURITY;
ALTER TABLE effect_lineage FORCE ROW LEVEL SECURITY;
CREATE POLICY effect_lineage_tenant ON effect_lineage USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON effect_lineage TO hp_delivery_app,hp_delivery_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON effect_lineage TO hp_delivery_purge;
ALTER TABLE attempts ENABLE ROW LEVEL SECURITY;
ALTER TABLE attempts FORCE ROW LEVEL SECURITY;
CREATE POLICY attempts_tenant ON attempts USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON attempts TO hp_delivery_app,hp_delivery_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON attempts TO hp_delivery_purge;
ALTER TABLE effect_observations ENABLE ROW LEVEL SECURITY;
ALTER TABLE effect_observations FORCE ROW LEVEL SECURITY;
CREATE POLICY effect_observations_tenant ON effect_observations USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT ON effect_observations TO hp_delivery_app,hp_delivery_worker;
GRANT SELECT,DELETE ON effect_observations TO hp_delivery_purge;
ALTER TABLE observation_content ENABLE ROW LEVEL SECURITY;
ALTER TABLE observation_content FORCE ROW LEVEL SECURITY;
CREATE POLICY observation_content_tenant ON observation_content USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT ON observation_content TO hp_delivery_app,hp_delivery_worker;
GRANT SELECT,DELETE ON observation_content TO hp_delivery_purge;
ALTER TABLE intent_content ENABLE ROW LEVEL SECURITY;
ALTER TABLE intent_content FORCE ROW LEVEL SECURITY;
CREATE POLICY intent_content_tenant ON intent_content USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT ON intent_content TO hp_delivery_app,hp_delivery_worker;
GRANT SELECT,DELETE ON intent_content TO hp_delivery_purge;
ALTER TABLE inbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbox FORCE ROW LEVEL SECURITY;
CREATE POLICY inbox_tenant ON inbox USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON inbox TO hp_delivery_app,hp_delivery_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON inbox TO hp_delivery_purge;
ALTER TABLE outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox FORCE ROW LEVEL SECURITY;
CREATE POLICY outbox_tenant ON outbox USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON outbox TO hp_delivery_app,hp_delivery_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON outbox TO hp_delivery_purge;
GRANT SELECT,INSERT,UPDATE,DELETE ON job_index TO hp_delivery_worker;
GRANT SELECT,INSERT,UPDATE,DELETE ON job_index TO hp_delivery_purge;
GRANT SELECT ON schema_version TO hp_delivery_app,hp_delivery_worker,hp_delivery_purge;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA hp_delivery FROM PUBLIC;
GRANT SELECT,INSERT,UPDATE ON credential_budget TO hp_delivery_worker;
GRANT SELECT,INSERT,UPDATE,DELETE ON credential_budget TO hp_delivery_purge;
COMMIT;
