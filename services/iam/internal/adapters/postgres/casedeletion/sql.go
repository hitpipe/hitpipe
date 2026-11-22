package casedeletion

// Check the same ownership, role separation, schema version and dedicated
// database assumptions as the admitted IAM authorization adapter. In
// addition, require the existing UPDATE privilege needed by SELECT FOR SHARE.
// Do not trust configurable search_path or accept owner/migrator privileges.
const checkApplicationAuthority = `SELECT current_user=session_user
 AND (SELECT rolcanlogin AND NOT(rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication)
      FROM pg_catalog.pg_roles WHERE rolname=current_user)
 AND pg_catalog.pg_has_role(current_user,'hp_iam_app','USAGE')
 AND NOT pg_catalog.pg_has_role(current_user,'hp_iam_worker','MEMBER')
 AND NOT pg_catalog.pg_has_role(current_user,'hp_iam_purge','MEMBER')
 AND NOT pg_catalog.pg_has_role(current_user,'hp_iam_migrator','MEMBER')
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members
      WHERE member=(SELECT oid FROM pg_catalog.pg_roles WHERE rolname=current_user)
        AND roleid<>(SELECT oid FROM pg_catalog.pg_roles WHERE rolname='hp_iam_app'))
 AND (SELECT count(*)=4 FROM pg_catalog.pg_roles
      WHERE rolname IN('hp_iam_app','hp_iam_worker','hp_iam_purge','hp_iam_migrator')
        AND NOT(rolcanlogin OR rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication))
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members
      WHERE member=(SELECT oid FROM pg_catalog.pg_roles WHERE rolname='hp_iam_app'))
 AND NOT pg_catalog.has_database_privilege(current_user,pg_catalog.current_database(),'CREATE')
 AND NOT pg_catalog.has_schema_privilege(current_user,'hp_iam','CREATE')
 AND (SELECT count(*)=17 FROM pg_catalog.pg_class c
      JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
      JOIN pg_catalog.pg_roles owner ON owner.oid=c.relowner AND owner.oid=n.nspowner
      WHERE n.nspname='hp_iam' AND c.relkind='r' AND owner.rolname='hp_iam_migrator')
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
      WHERE n.nspname='hp_iam' AND (c.relkind IN('p','f') OR c.relkind='r'
       AND c.relowner<>(SELECT oid FROM pg_catalog.pg_roles WHERE rolname='hp_iam_migrator')))
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
      WHERE c.relkind IN('r','p','f') AND n.nspname NOT IN('hp_iam','pg_catalog','information_schema')
       AND n.nspname NOT LIKE 'pg_toast%' AND n.nspname NOT LIKE 'pg_temp_%')
 AND (SELECT count(*)=1 AND min(version)=1 FROM hp_iam.schema_version)
 AND (SELECT count(*)=2 FROM pg_catalog.pg_class
      WHERE oid IN('hp_iam.workspaces'::regclass,'hp_iam.memberships'::regclass)
        AND relrowsecurity AND relforcerowsecurity)
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.unnest(
      ARRAY['principals','sessions','workspaces','memberships','revocations']) AS t(name)
      WHERE NOT pg_catalog.has_table_privilege(current_user,'hp_iam.'||name,'SELECT')
         OR NOT pg_catalog.has_table_privilege(current_user,'hp_iam.'||name,'UPDATE')
         OR pg_catalog.has_table_privilege(current_user,'hp_iam.'||name,'DELETE'))`

const lockOwner = `SELECT s.session_id::text,p.principal_id::text,w.workspace_id::text,
 s.epoch,w.security_epoch,m.revision,s.created_at,s.expires_at,s.idle_expires_at,s.revoked_at IS NOT NULL,
 p.disabled_at IS NOT NULL,w.lifecycle,m.role,m.status,m.revoked_at IS NOT NULL,m.accepted_at
 FROM hp_iam.sessions AS s
 JOIN hp_iam.principals AS p ON p.principal_id=s.principal_id
 JOIN hp_iam.memberships AS m ON m.principal_id=p.principal_id
 JOIN hp_iam.workspaces AS w ON w.workspace_id=m.workspace_id
 WHERE s.session_hash=$1 AND w.workspace_id=$2
 FOR SHARE OF s,p,w,m`

// This standalone action has no CONNECTION binding. Membership subjects have
// no canonical UUID in the accepted schema, so a workspace membership deny
// cannot be interpreted as harmless to a particular principal.
const currentDenials = `SELECT EXISTS(SELECT 1 FROM hp_iam.revocations
 WHERE (scope='SESSION' AND subject_id=$1)
    OR (scope='WORKSPACE' AND (workspace_id=$2 OR subject_id=$2))
    OR (scope='MEMBERSHIP' AND (workspace_id=$2 OR workspace_id IS NULL)))`
