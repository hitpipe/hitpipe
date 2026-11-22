package revocation

// Require the HIT-81 IAM application role and migrator-owned original schema.
// No more privileged login may silently bypass workspace RLS or edit the roster.
const checkAuthority = `SELECT current_user=session_user
 AND current_setting('fsync')='on' AND current_setting('full_page_writes')='on'
 AND (SELECT rolcanlogin AND NOT(rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication)
      FROM pg_catalog.pg_roles WHERE rolname=current_user)
 AND pg_has_role(current_user,'hp_iam_app','USAGE')
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members
      WHERE member=(SELECT oid FROM pg_catalog.pg_roles WHERE rolname=current_user)
        AND roleid<>(SELECT oid FROM pg_catalog.pg_roles WHERE rolname='hp_iam_app'))
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members
      WHERE member=(SELECT oid FROM pg_catalog.pg_roles WHERE rolname='hp_iam_app'))
 AND NOT has_database_privilege(current_user,current_database(),'CREATE')
 AND NOT has_schema_privilege(current_user,'hp_iam','CREATE')
 AND (SELECT count(*)=6 FROM pg_catalog.pg_class c
      JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
      JOIN pg_catalog.pg_roles owner ON owner.oid=c.relowner AND owner.oid=n.nspowner
      WHERE n.nspname='hp_iam' AND c.relkind='r' AND c.relpersistence='p' AND owner.rolname='hp_iam_migrator'
        AND c.relname IN('schema_version','runtime_registry','revocations','revocation_participants','workspaces','sessions'))
 AND (SELECT count(*)=1 AND min(version)=1 FROM hp_iam.schema_version)
 AND NOT EXISTS(SELECT 1 FROM unnest(ARRAY['runtime_registry','revocations','revocation_participants','workspaces','sessions']) AS t(name)
      WHERE has_table_privilege(current_user,'hp_iam.'||t.name,'DELETE'))
 AND (SELECT relrowsecurity AND relforcerowsecurity FROM pg_catalog.pg_class
      WHERE oid='hp_iam.workspaces'::regclass)`

const existingSubject = `SELECT EXISTS (SELECT 1 FROM hp_iam.revocations
 WHERE scope=$1 AND subject_id=$2 AND workspace_id IS NOT DISTINCT FROM $3::uuid)`

const denySession = `UPDATE hp_iam.sessions SET revoked_at=$2,epoch=epoch+1
 WHERE session_id=$1 AND revoked_at IS NULL AND epoch<$3 RETURNING epoch`

// WORKSPACE deletion and lifecycle transitions belong to separate tasks.
// Persist the current epoch and durable denial together without changing life cycle.
const advanceWorkspace = `UPDATE hp_iam.workspaces SET security_epoch=security_epoch+1
 WHERE workspace_id=$1 AND security_epoch<$2 RETURNING security_epoch`

const insertRevocation = `INSERT INTO hp_iam.revocations
 (workspace_id,revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,reason_code)
 VALUES($1,$2,$3,$4,$5,$6,$7,'PENDING',$8)`

const insertParticipants = `INSERT INTO hp_iam.revocation_participants
 (workspace_id,revocation_id,runtime_id,generation,boot_id)
 SELECT $1,$2,runtime_id,generation,boot_id FROM hp_iam.runtime_registry`

const readRoster = `SELECT runtime_id::text,boot_id::text,generation,service,certificate_fingerprint
 FROM hp_iam.runtime_registry ORDER BY runtime_id LIMIT $1`

const getRevocation = `SELECT revocation_id::text,workspace_id::text,scope,subject_id::text,epoch,
 accepted_at,deadline_at,status,effective_at,deadline_breached_at,incident_id::text,reason_code
 FROM hp_iam.revocations WHERE revocation_id=$1`

// The schema stores exact runtime/generation/boot in the frozen roster. The
// current certificate pin is usable only while all three keys still match.
const getParticipants = `SELECT p.workspace_id::text,p.runtime_id::text,p.boot_id::text,p.generation,
 coalesce(r.certificate_fingerprint,''),p.applied_epoch,p.acknowledged_at
 FROM hp_iam.revocation_participants p
 LEFT JOIN hp_iam.runtime_registry r ON r.runtime_id=p.runtime_id AND r.generation=p.generation AND r.boot_id=p.boot_id
 WHERE p.revocation_id=$1 ORDER BY p.runtime_id LIMIT $2`

const listOpen = `SELECT revocation_id::text FROM hp_iam.revocations
 WHERE scope IN('SESSION','WORKSPACE') AND status IN('PENDING','INCIDENT')
 AND (NULLIF($1,'')::uuid IS NULL OR revocation_id>NULLIF($1,'')::uuid)
 ORDER BY revocation_id LIMIT $2`

const storeAck = `UPDATE hp_iam.revocation_participants SET applied_epoch=$5,acknowledged_at=$6
 WHERE revocation_id=$1 AND runtime_id=$2 AND generation=$3 AND boot_id=$4 AND acknowledged_at IS NULL`

const updateFinal = `UPDATE hp_iam.revocations
 SET status=$2,effective_at=$3,deadline_breached_at=$4,incident_id=$5 WHERE revocation_id=$1`
