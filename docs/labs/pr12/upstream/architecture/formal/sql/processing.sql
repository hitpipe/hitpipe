-- HitPipe processing persistence contract v1. Applies ONLY to its dedicated database.
-- Migrator owns schema/tables; hp_processing_app and hp_processing_worker are NOT owners,
-- NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS. Bootstrap roles before migration.
-- hp_processing_purge is a separately authorized maintenance role, never inherited by app/worker.
-- Real event/context/history/proof/outbox payloads and encrypted secrets live ONLY in
-- service-owned fsync/atomic-rename content files under BP006, never SQL/WAL/SQL backups.
-- SQL retains opaque refs/hashes/size/expiry plus minimal safety/accepted-rule state.
-- App/scoped workers SET LOCAL hp.workspace_id after authoritative scope validation.
-- Global job_index exposes ONLY tenant/job IDs and due/claim metadata, never payload/PII.
BEGIN;
CREATE SCHEMA hp_processing;
REVOKE ALL ON SCHEMA hp_processing FROM PUBLIC;
GRANT USAGE ON SCHEMA hp_processing TO hp_processing_app,hp_processing_worker,hp_processing_purge;
SET LOCAL search_path=hp_processing,pg_catalog;
CREATE TABLE schema_version(version integer PRIMARY KEY CHECK(version=1),installed_at timestamptz NOT NULL DEFAULT clock_timestamp());
INSERT INTO schema_version(version) VALUES(1);

CREATE FUNCTION safe_config_valid(v jsonb) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
 SELECT jsonb_typeof(v)='object' AND NOT EXISTS(
  SELECT 1 FROM jsonb_each(v)e WHERE e.key NOT IN('pack_id','source_connection_id','destination_connection_id','destination_workspace_id','price_alias','list_alias','assignee_alias','price_id','list_id','assignee_id','mode','required_context_keys','title_pattern','service_label')
  OR CASE WHEN e.key='service_label' THEN jsonb_typeof(e.value)<>'string' OR length(e.value#>>'{}') NOT BETWEEN 1 AND 120 OR e.value#>>'{}'~'[[:cntrl:]]'
  WHEN e.key='required_context_keys' THEN CASE WHEN jsonb_typeof(e.value)='array' THEN EXISTS(SELECT 1 FROM jsonb_array_elements(e.value)a WHERE jsonb_typeof(a)<>'string' OR a#>>'{}'!~'^[a-z][a-z0-9_]{0,63}$') ELSE true END
  ELSE jsonb_typeof(e.value)<>'string' OR e.value#>>'{}'!~'^[A-Za-z0-9_.:-]{1,255}$' END
 );
$$;
-- service_label is necessary immutable catalog rendering meaning, NOT customer contact/brief/AI prose.
-- Source catalog origin + owner confirmation + known-secret screening are application obligations;
-- this SQL shape check does not claim universal PII detection.
CREATE TABLE operations(workspace_id uuid NOT NULL,operation_id uuid NOT NULL,owner_user_id uuid NOT NULL,revision bigint NOT NULL CHECK(revision>0),state text NOT NULL CHECK(state IN('DRAFT','PREPARED','ACTIVE','DRAINING','PAUSED','ARCHIVED')),first_activation_at timestamptz,selected_config_id uuid,pause_accepted_at timestamptz,drain_deadline_at timestamptz,PRIMARY KEY(workspace_id,operation_id));
-- Exactly one active SINGLE operation. A future TEAM revision must replace this explicit index.
CREATE UNIQUE INDEX one_single_active ON operations(workspace_id) WHERE state IN('ACTIVE','DRAINING');
CREATE TABLE drafts(workspace_id uuid NOT NULL,draft_id uuid NOT NULL,operation_id uuid,revision bigint NOT NULL CHECK(revision>0),technical_config jsonb NOT NULL CHECK(safe_config_valid(technical_config)),prose_ref uuid,prose_sha256 text,prose_expires_at timestamptz,updated_at timestamptz NOT NULL,expires_at timestamptz,PRIMARY KEY(workspace_id,draft_id),FOREIGN KEY(workspace_id,operation_id) REFERENCES operations(workspace_id,operation_id));
CREATE TABLE accepted_configs(workspace_id uuid NOT NULL,config_id uuid NOT NULL,operation_id uuid NOT NULL,schema_version text NOT NULL CHECK(schema_version='1.0'),evaluator_version text NOT NULL CHECK(length(evaluator_version)<=80),canonical_config jsonb NOT NULL CHECK(safe_config_valid(canonical_config)),digest text NOT NULL CHECK(digest~'^sha256:[0-9a-f]{64}$'),accepted_by uuid NOT NULL,accepted_at timestamptz NOT NULL,rehearsal_digest text NOT NULL,PRIMARY KEY(workspace_id,config_id),UNIQUE(workspace_id,operation_id,digest),UNIQUE(workspace_id,operation_id,config_id),FOREIGN KEY(workspace_id,operation_id) REFERENCES operations(workspace_id,operation_id));
ALTER TABLE operations ADD CONSTRAINT selected_config_same_ws FOREIGN KEY(workspace_id,operation_id,selected_config_id) REFERENCES accepted_configs(workspace_id,operation_id,config_id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE case_identity(workspace_id uuid NOT NULL,case_id uuid NOT NULL,operation_id uuid NOT NULL,config_id uuid NOT NULL,source_account_id text NOT NULL CHECK(length(source_account_id)<=255),livemode boolean NOT NULL,session_id text NOT NULL CHECK(length(session_id)<=255),admitted_at timestamptz NOT NULL,owner_user_id uuid NOT NULL,state text NOT NULL CHECK(state IN('RECEIVED','WAITING_DATA','WORKING','UNKNOWN','ATTENTION','CONFIRMED','EXCLUDED','CLOSED_WITHOUT_CONFIRMATION')),revision bigint NOT NULL CHECK(revision>0),ever_dispatch_fenced boolean NOT NULL DEFAULT false,outcome_knowledge text NOT NULL CHECK(outcome_knowledge IN('UNESTABLISHED','NO_EFFECT','POSSIBLE_EFFECT','TARGET_ID_KNOWN','RESULT_VERIFIED','RESULT_MISMATCH')),lineage_id uuid,known_task_id text,last_confirmed_at timestamptz,PRIMARY KEY(workspace_id,case_id),UNIQUE(workspace_id,source_account_id,livemode,session_id),FOREIGN KEY(workspace_id,operation_id) REFERENCES operations(workspace_id,operation_id),FOREIGN KEY(workspace_id,operation_id,config_id) REFERENCES accepted_configs(workspace_id,operation_id,config_id));
CREATE TABLE episodes(workspace_id uuid NOT NULL,episode_id uuid NOT NULL,case_id uuid NOT NULL,started_at timestamptz NOT NULL,deadline_at timestamptz NOT NULL,automatic_writes integer NOT NULL DEFAULT 0 CHECK(automatic_writes BETWEEN 0 AND 3),qualified_manual_command_id uuid,completed_at timestamptz,PRIMARY KEY(workspace_id,episode_id),FOREIGN KEY(workspace_id,case_id) REFERENCES case_identity(workspace_id,case_id),CHECK(deadline_at<=started_at+interval'300seconds'));
CREATE FUNCTION protect_episode_clock() RETURNS trigger LANGUAGE plpgsql SET search_path=hp_processing,pg_catalog AS $$ BEGIN
 IF (NEW.workspace_id,NEW.episode_id,NEW.case_id,NEW.started_at,NEW.deadline_at) IS DISTINCT FROM (OLD.workspace_id,OLD.episode_id,OLD.case_id,OLD.started_at,OLD.deadline_at) OR NEW.automatic_writes<OLD.automatic_writes THEN RAISE EXCEPTION 'episode clock/attempt count cannot reset';END IF;RETURN NEW;END $$;
CREATE TRIGGER episode_clock_immutable BEFORE UPDATE ON episodes FOR EACH ROW EXECUTE FUNCTION protect_episode_clock();
CREATE TABLE case_content(workspace_id uuid NOT NULL,content_id uuid NOT NULL,case_id uuid NOT NULL,field_key text NOT NULL CHECK(field_key~'^[a-z][a-z0-9_]{0,63}$'),value_ref uuid NOT NULL,value_sha256 text NOT NULL CHECK(value_sha256~'^sha256:[0-9a-f]{64}$'),serialized_value_bytes integer NOT NULL CHECK(serialized_value_bytes BETWEEN 2 AND 32768),author_user_id uuid NOT NULL,added_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,PRIMARY KEY(workspace_id,content_id),UNIQUE(workspace_id,case_id,field_key),FOREIGN KEY(workspace_id,case_id) REFERENCES case_identity(workspace_id,case_id),CHECK(expires_at<=added_at+interval'90days'));
CREATE FUNCTION context_total_bound() RETURNS trigger LANGUAGE plpgsql SET search_path=hp_processing,pg_catalog AS $$
DECLARE ws uuid;cid uuid;sz integer;
BEGIN
 ws:=CASE WHEN TG_OP='DELETE' THEN OLD.workspace_id ELSE NEW.workspace_id END;cid:=CASE WHEN TG_OP='DELETE' THEN OLD.case_id ELSE NEW.case_id END;
 PERFORM 1 FROM case_identity WHERE workspace_id=ws AND case_id=cid FOR UPDATE;
 SELECT CASE WHEN count(*)=0 THEN 2 ELSE sum(serialized_value_bytes+octet_length(field_key)+4)+1 END INTO sz FROM case_content WHERE workspace_id=ws AND case_id=cid;
 IF sz>32768 THEN RAISE EXCEPTION 'serialized current business context exceeds32KiB';END IF;
 IF TG_OP='DELETE' THEN RETURN OLD;ELSE RETURN NEW;END IF;
END $$;
CREATE TRIGGER context_aggregate_limit AFTER INSERT OR UPDATE OR DELETE ON case_content FOR EACH ROW EXECUTE FUNCTION context_total_bound();
CREATE TABLE safety_history(workspace_id uuid NOT NULL,entry_id uuid NOT NULL,case_id uuid NOT NULL,actor_kind text NOT NULL CHECK(actor_kind IN('USER','SYSTEM','PROVIDER','SUPPORT')),actor_id uuid,at timestamptz NOT NULL,reason_code text NOT NULL CHECK(reason_code~'^[A-Z0-9_]{1,120}$'),old_state text,new_state text,proof_ref uuid,PRIMARY KEY(workspace_id,entry_id),FOREIGN KEY(workspace_id,case_id) REFERENCES case_identity(workspace_id,case_id));
CREATE TABLE history_content(workspace_id uuid NOT NULL,entry_id uuid NOT NULL,detail_ref uuid NOT NULL,detail_sha256 text NOT NULL CHECK(detail_sha256~'^sha256:[0-9a-f]{64}$'),detail_bytes integer NOT NULL CHECK(detail_bytes BETWEEN 1 AND 65536),created_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,PRIMARY KEY(workspace_id,entry_id),FOREIGN KEY(workspace_id,entry_id) REFERENCES safety_history(workspace_id,entry_id),CHECK(expires_at<=created_at+interval'90days'));
CREATE TABLE user_commands(workspace_id uuid NOT NULL,request_id uuid NOT NULL,actor_id uuid NOT NULL,digest text NOT NULL CHECK(digest~'^sha256:[0-9a-f]{64}$'),aggregate_id uuid NOT NULL,expected_revision bigint,result_revision bigint,disposition text NOT NULL,recorded_at timestamptz NOT NULL,PRIMARY KEY(workspace_id,request_id));
CREATE TABLE notifications(workspace_id uuid NOT NULL,notification_id uuid NOT NULL,case_id uuid NOT NULL,reason_code text NOT NULL,updated_at timestamptz NOT NULL,acknowledged_at timestamptz,PRIMARY KEY(workspace_id,notification_id),UNIQUE(workspace_id,case_id,reason_code),FOREIGN KEY(workspace_id,case_id) REFERENCES case_identity(workspace_id,case_id));
CREATE FUNCTION protect_case_identity() RETURNS trigger LANGUAGE plpgsql SET search_path=hp_processing,pg_catalog AS $$ BEGIN
 IF (NEW.workspace_id,NEW.case_id,NEW.operation_id,NEW.config_id,NEW.source_account_id,NEW.livemode,NEW.session_id,NEW.admitted_at) IS DISTINCT FROM (OLD.workspace_id,OLD.case_id,OLD.operation_id,OLD.config_id,OLD.source_account_id,OLD.livemode,OLD.session_id,OLD.admitted_at) OR (OLD.ever_dispatch_fenced AND NOT NEW.ever_dispatch_fenced) THEN RAISE EXCEPTION 'pinned case/safety identity immutable';END IF;RETURN NEW;END $$;
CREATE TRIGGER case_identity_immutable BEFORE UPDATE ON case_identity FOR EACH ROW EXECUTE FUNCTION protect_case_identity();
CREATE FUNCTION protect_operation_boundary() RETURNS trigger LANGUAGE plpgsql SET search_path=hp_processing,pg_catalog AS $$ BEGIN
 IF OLD.first_activation_at IS NOT NULL AND NEW.first_activation_at IS DISTINCT FROM OLD.first_activation_at THEN RAISE EXCEPTION 'initial activation boundary immutable';END IF;RETURN NEW;END $$;
CREATE TRIGGER first_activation_immutable BEFORE UPDATE ON operations FOR EACH ROW EXECUTE FUNCTION protect_operation_boundary();

CREATE TABLE inbox(workspace_id uuid NOT NULL,message_id uuid NOT NULL,digest text NOT NULL CHECK(digest~'^sha256:[0-9a-f]{64}$'),disposition text NOT NULL CHECK(disposition IN('APPLIED','HELD','REJECTED','CONFLICT')),record_id uuid,received_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(workspace_id,message_id));
CREATE TABLE outbox(workspace_id uuid NOT NULL,message_id uuid NOT NULL,kind text NOT NULL,payload_ref uuid NOT NULL,payload_sha256 text NOT NULL CHECK(payload_sha256~'^sha256:[0-9a-f]{64}$'),payload_bytes integer NOT NULL CHECK(payload_bytes BETWEEN 1 AND 65536),digest text NOT NULL CHECK(digest~'^sha256:[0-9a-f]{64}$'),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL,state text NOT NULL DEFAULT 'PENDING' CHECK(state IN('PENDING','CLAIMED','ACKED','HELD')),claim_token uuid,lease_until timestamptz,delivery_count integer NOT NULL DEFAULT 0 CHECK(delivery_count>=0),PRIMARY KEY(workspace_id,message_id),CHECK(expires_at>created_at));
CREATE INDEX outbox_due ON outbox(next_attempt_at) WHERE state IN('PENDING','CLAIMED');
-- Only workers see this global index. They must open a NEW scoped transaction before payload access.
CREATE TABLE job_index(job_id uuid PRIMARY KEY,workspace_id uuid NOT NULL,kind text NOT NULL CHECK(length(kind)<=80),due_at timestamptz NOT NULL,claim_token uuid,lease_until timestamptz);
CREATE INDEX job_index_due ON job_index(due_at);

ALTER TABLE operations ENABLE ROW LEVEL SECURITY;
ALTER TABLE operations FORCE ROW LEVEL SECURITY;
CREATE POLICY operations_tenant ON operations USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON operations TO hp_processing_app,hp_processing_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON operations TO hp_processing_purge;
ALTER TABLE drafts ENABLE ROW LEVEL SECURITY;
ALTER TABLE drafts FORCE ROW LEVEL SECURITY;
CREATE POLICY drafts_tenant ON drafts USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON drafts TO hp_processing_app,hp_processing_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON drafts TO hp_processing_purge;
ALTER TABLE accepted_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE accepted_configs FORCE ROW LEVEL SECURITY;
CREATE POLICY accepted_configs_tenant ON accepted_configs USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT ON accepted_configs TO hp_processing_app,hp_processing_worker;
GRANT SELECT,DELETE ON accepted_configs TO hp_processing_purge;
ALTER TABLE case_identity ENABLE ROW LEVEL SECURITY;
ALTER TABLE case_identity FORCE ROW LEVEL SECURITY;
CREATE POLICY case_identity_tenant ON case_identity USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON case_identity TO hp_processing_app,hp_processing_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON case_identity TO hp_processing_purge;
ALTER TABLE episodes ENABLE ROW LEVEL SECURITY;
ALTER TABLE episodes FORCE ROW LEVEL SECURITY;
CREATE POLICY episodes_tenant ON episodes USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON episodes TO hp_processing_app,hp_processing_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON episodes TO hp_processing_purge;
ALTER TABLE case_content ENABLE ROW LEVEL SECURITY;
ALTER TABLE case_content FORCE ROW LEVEL SECURITY;
CREATE POLICY case_content_tenant ON case_content USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON case_content TO hp_processing_app,hp_processing_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON case_content TO hp_processing_purge;
ALTER TABLE safety_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE safety_history FORCE ROW LEVEL SECURITY;
CREATE POLICY safety_history_tenant ON safety_history USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT ON safety_history TO hp_processing_app,hp_processing_worker;
GRANT SELECT,DELETE ON safety_history TO hp_processing_purge;
ALTER TABLE history_content ENABLE ROW LEVEL SECURITY;
ALTER TABLE history_content FORCE ROW LEVEL SECURITY;
CREATE POLICY history_content_tenant ON history_content USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT ON history_content TO hp_processing_app,hp_processing_worker;
GRANT SELECT,DELETE ON history_content TO hp_processing_purge;
ALTER TABLE user_commands ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_commands FORCE ROW LEVEL SECURITY;
CREATE POLICY user_commands_tenant ON user_commands USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT ON user_commands TO hp_processing_app,hp_processing_worker;
GRANT SELECT,DELETE ON user_commands TO hp_processing_purge;
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications FORCE ROW LEVEL SECURITY;
CREATE POLICY notifications_tenant ON notifications USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON notifications TO hp_processing_app,hp_processing_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON notifications TO hp_processing_purge;
ALTER TABLE inbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbox FORCE ROW LEVEL SECURITY;
CREATE POLICY inbox_tenant ON inbox USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON inbox TO hp_processing_app,hp_processing_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON inbox TO hp_processing_purge;
ALTER TABLE outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox FORCE ROW LEVEL SECURITY;
CREATE POLICY outbox_tenant ON outbox USING(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting('hp.workspace_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON outbox TO hp_processing_app,hp_processing_worker;
GRANT SELECT,DELETE,INSERT,UPDATE ON outbox TO hp_processing_purge;
GRANT SELECT,INSERT,UPDATE,DELETE ON job_index TO hp_processing_worker;
GRANT SELECT,INSERT,UPDATE,DELETE ON job_index TO hp_processing_purge;
GRANT SELECT ON schema_version TO hp_processing_app,hp_processing_worker,hp_processing_purge;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA hp_processing FROM PUBLIC;
GRANT EXECUTE ON FUNCTION safe_config_valid(jsonb) TO hp_processing_app,hp_processing_worker,hp_processing_purge;
COMMIT;
