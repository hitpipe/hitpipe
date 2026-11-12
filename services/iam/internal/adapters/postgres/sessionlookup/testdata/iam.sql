-- Canonical IAM persistence contract v1. Dedicated IAM database, migrator-owned.
-- Bootstrap hp_iam_app, hp_iam_worker, hp_iam_purge: NOLOGIN group roles,
-- NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS. Runtime login inherits ONLY own group.
-- User-facing reads require current authorization and actual-start gates; SQL RLS is defense-in-depth,
-- not proof of authority (a service can choose its own session GUC).
BEGIN;
CREATE SCHEMA hp_iam;
REVOKE ALL ON SCHEMA hp_iam FROM PUBLIC;
GRANT USAGE ON SCHEMA hp_iam TO hp_iam_app,hp_iam_worker,hp_iam_purge;
SET LOCAL search_path=hp_iam,pg_catalog;
CREATE TABLE schema_version(version integer PRIMARY KEY CHECK(version=1),installed_at timestamptz NOT NULL DEFAULT clock_timestamp());
INSERT INTO schema_version VALUES(1,clock_timestamp());
CREATE TABLE principals(principal_id uuid PRIMARY KEY,oidc_issuer text NOT NULL,oidc_subject text NOT NULL,profile_content_ref uuid,profile_digest text,created_at timestamptz NOT NULL,disabled_at timestamptz,UNIQUE(oidc_issuer,oidc_subject));
CREATE TABLE sessions(session_id uuid PRIMARY KEY,session_hash bytea NOT NULL UNIQUE CHECK(octet_length(session_hash)=32),principal_id uuid NOT NULL REFERENCES principals(principal_id),csrf_hash bytea NOT NULL CHECK(octet_length(csrf_hash)=32),created_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,idle_expires_at timestamptz NOT NULL,revoked_at timestamptz,epoch bigint NOT NULL CHECK(epoch>0),CHECK(expires_at>created_at),CHECK(idle_expires_at<=expires_at));
CREATE TABLE workspaces(workspace_id uuid PRIMARY KEY,name_content_ref uuid NOT NULL,revision bigint NOT NULL CHECK(revision>0),security_epoch bigint NOT NULL CHECK(security_epoch>0),lifecycle text NOT NULL CHECK(lifecycle IN('ONBOARDING','ACTIVE','DELETING','DELETED')),created_at timestamptz NOT NULL,owner_confirmed_at timestamptz,initial_activation_at timestamptz);
CREATE TABLE memberships(workspace_id uuid NOT NULL REFERENCES workspaces(workspace_id),principal_id uuid NOT NULL REFERENCES principals(principal_id),role text NOT NULL CHECK(role IN('OWNER','OPERATOR','READER')),status text NOT NULL CHECK(status IN('INVITED','ACTIVE','REVOKED')),revision bigint NOT NULL CHECK(revision>0),accepted_at timestamptz,revoked_at timestamptz,PRIMARY KEY(workspace_id,principal_id));
CREATE TABLE owner_transfers(workspace_id uuid NOT NULL,transfer_id uuid NOT NULL,from_principal_id uuid NOT NULL,to_principal_id uuid NOT NULL,expected_workspace_revision bigint NOT NULL,created_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,accepted_at timestamptz,PRIMARY KEY(workspace_id,transfer_id),FOREIGN KEY(workspace_id,from_principal_id) REFERENCES memberships(workspace_id,principal_id),FOREIGN KEY(workspace_id,to_principal_id) REFERENCES memberships(workspace_id,principal_id),CHECK(to_principal_id<>from_principal_id),CHECK(expires_at>created_at));
CREATE TABLE external_bindings(workspace_id uuid NOT NULL,grant_id uuid NOT NULL,provider text NOT NULL CHECK(provider IN('STRIPE','CLICKUP')),provider_account_id text NOT NULL,livemode boolean NOT NULL,pack text NOT NULL CHECK(pack='PAID_ORDER_TASK_V1'),status text NOT NULL CHECK(status IN('RESERVED','VERIFIED','REVOKED','DELETING')),security_epoch bigint NOT NULL CHECK(security_epoch>0),created_at timestamptz NOT NULL,verified_at timestamptz,PRIMARY KEY(workspace_id,grant_id),UNIQUE(provider,provider_account_id,livemode,pack));
CREATE TABLE revocations(workspace_id uuid,revocation_id uuid NOT NULL,scope text NOT NULL CHECK(scope IN('SESSION','MEMBERSHIP','CONNECTION','WORKSPACE')),subject_id uuid NOT NULL,epoch bigint NOT NULL CHECK(epoch>0),accepted_at timestamptz NOT NULL,deadline_at timestamptz NOT NULL,status text NOT NULL CHECK(status IN('PENDING','EFFECTIVE','INCIDENT')),effective_at timestamptz,reason_code text NOT NULL,deadline_breached_at timestamptz,incident_id uuid,PRIMARY KEY(revocation_id),UNIQUE(workspace_id,revocation_id),CHECK(deadline_at=accepted_at+interval'5seconds'),CHECK((status='EFFECTIVE')=(effective_at IS NOT NULL)));
-- Participant generation is sealed before serving. New process begins CLOSED.
CREATE TABLE runtime_registry(runtime_id uuid PRIMARY KEY,service text NOT NULL CHECK(service IN('IAM','INTAKE','PROCESSING','DELIVERY','BFF','AI')),generation bigint NOT NULL CHECK(generation>0),boot_id uuid NOT NULL UNIQUE,certificate_fingerprint text NOT NULL CHECK(certificate_fingerprint~'^sha256:[0-9a-f]{64}$'),status text NOT NULL CHECK(status IN('CLOSED','SYNCING','OPEN','FENCED')),registered_at timestamptz NOT NULL,last_observed_at timestamptz NOT NULL,fence_evidence_ref uuid);
CREATE TABLE revocation_participants(workspace_id uuid,revocation_id uuid NOT NULL,runtime_id uuid NOT NULL REFERENCES runtime_registry(runtime_id),generation bigint NOT NULL,boot_id uuid NOT NULL,applied_epoch bigint,acknowledged_at timestamptz,fence_evidence_ref uuid,PRIMARY KEY(revocation_id,runtime_id,generation,boot_id),FOREIGN KEY(revocation_id) REFERENCES revocations(revocation_id),CHECK((acknowledged_at IS NULL) OR (applied_epoch IS NOT NULL)));
CREATE TABLE deletion_overlays(workspace_id uuid PRIMARY KEY,deletion_id uuid NOT NULL UNIQUE,confirmed_at timestamptz NOT NULL,active_deadline_at timestamptz NOT NULL,backup_deadline_at timestamptz NOT NULL,status text NOT NULL CHECK(status IN('CLOSING','PURGING','ACTIVE_PURGED','BACKUPS_PURGED','INCIDENT')),CHECK(active_deadline_at<=confirmed_at+interval'7days'),CHECK(backup_deadline_at<=confirmed_at+interval'35days'));
CREATE TABLE deletion_receipts(workspace_id uuid NOT NULL,service text NOT NULL,active_purged_at timestamptz,backup_last_copy_expires_at timestamptz,receipt_digest text NOT NULL,PRIMARY KEY(workspace_id,service),FOREIGN KEY(workspace_id) REFERENCES deletion_overlays(workspace_id));
CREATE TABLE support_grants(workspace_id uuid NOT NULL,grant_id uuid NOT NULL,support_principal_id uuid NOT NULL REFERENCES principals(principal_id),authorized_by uuid NOT NULL,action_code text NOT NULL,resource_id uuid NOT NULL,created_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,revoked_at timestamptz,PRIMARY KEY(workspace_id,grant_id),CHECK(expires_at<=created_at+interval'1hour'),FOREIGN KEY(workspace_id,authorized_by) REFERENCES memberships(workspace_id,principal_id));
CREATE TABLE command_receipts(workspace_id uuid NOT NULL,actor_id uuid NOT NULL,request_id uuid NOT NULL,body_digest text NOT NULL CHECK(body_digest~'^sha256:[0-9a-f]{64}$'),result_id uuid NOT NULL,result_kind text NOT NULL,created_at timestamptz NOT NULL,PRIMARY KEY(workspace_id,actor_id,request_id));
CREATE TABLE audit_minimal(workspace_id uuid NOT NULL,entry_id uuid NOT NULL,actor_id uuid,action_code text NOT NULL,resource_id uuid NOT NULL,at timestamptz NOT NULL,reason_code text NOT NULL,evidence_content_ref uuid,evidence_expires_at timestamptz,PRIMARY KEY(workspace_id,entry_id));
CREATE TABLE outbox(workspace_id uuid NOT NULL,message_id uuid NOT NULL,digest text NOT NULL CHECK(digest~'^sha256:[0-9a-f]{64}$'),kind text NOT NULL,payload_content_ref uuid NOT NULL,payload_expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL,state text NOT NULL CHECK(state IN('PENDING','CLAIMED','ACKED','HELD')),next_attempt_at timestamptz NOT NULL,lease_until timestamptz,PRIMARY KEY(workspace_id,message_id));
CREATE TABLE job_index(job_id uuid PRIMARY KEY,workspace_id uuid NOT NULL,kind text NOT NULL,due_at timestamptz NOT NULL,claim_token uuid,lease_until timestamptz);
CREATE INDEX job_index_due ON job_index(due_at);
DO $$ DECLARE tab text; BEGIN
 FOREACH tab IN ARRAY ARRAY['workspaces','memberships','owner_transfers','external_bindings','deletion_overlays','deletion_receipts','support_grants','command_receipts','audit_minimal','outbox'] LOOP
 EXECUTE format('ALTER TABLE hp_iam.%I ENABLE ROW LEVEL SECURITY',tab);
 EXECUTE format('ALTER TABLE hp_iam.%I FORCE ROW LEVEL SECURITY',tab);
 EXECUTE format('CREATE POLICY tenant ON hp_iam.%I USING(workspace_id=NULLIF(current_setting(''hp.workspace_id'',true),'''')::uuid) WITH CHECK(workspace_id=NULLIF(current_setting(''hp.workspace_id'',true),'''')::uuid)',tab);
 EXECUTE format('GRANT SELECT,INSERT,UPDATE ON hp_iam.%I TO hp_iam_app,hp_iam_worker',tab);
 EXECUTE format('GRANT SELECT,DELETE ON hp_iam.%I TO hp_iam_purge',tab);
 END LOOP;
END $$;
-- Principals/sessions/global runtime roster have explicit IAM-only authority and no tenant GUC fiction.
-- Global/session revocation may span workspaces (workspace_id NULL). IAM-only coordinator enforces validated scope; these tables are not tenant-GUC authorities.
GRANT SELECT,INSERT,UPDATE ON revocations,revocation_participants TO hp_iam_app,hp_iam_worker;
GRANT SELECT,DELETE ON revocations,revocation_participants TO hp_iam_purge;
GRANT SELECT,INSERT,UPDATE ON principals,sessions,runtime_registry TO hp_iam_app,hp_iam_worker;
GRANT SELECT,DELETE ON principals,sessions,runtime_registry TO hp_iam_purge;
GRANT SELECT,INSERT,UPDATE,DELETE ON job_index TO hp_iam_worker,hp_iam_purge;
GRANT SELECT ON schema_version TO hp_iam_app,hp_iam_worker,hp_iam_purge;
REVOKE UPDATE ON audit_minimal,command_receipts FROM hp_iam_app,hp_iam_worker;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA hp_iam FROM PUBLIC;
COMMIT;
