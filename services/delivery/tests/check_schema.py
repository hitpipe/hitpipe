"""Isolated Delivery schema checks. Requires Docker; no host ports or volumes."""
import argparse, datetime, hashlib, json, pathlib, platform, subprocess, time, uuid, sys
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--report', type=pathlib.Path, help='Save the actual successful run as UTF-8 JSON')
args = parser.parse_args()
started_at = datetime.datetime.now(datetime.timezone.utc).isoformat()
ROOT = pathlib.Path(__file__).resolve().parents[1]
IMAGE = "postgres@sha256:d8703cd7fba306b9fec9268ecedfa8a966846c053036a60e3635791957eb2f66"
NAME = "hit70-" + uuid.uuid4().hex[:12]
checks = []
def run(args, data=None):
    p = subprocess.run(args, input=data, text=True, capture_output=True, timeout=180)
    if p.returncode:
        raise RuntimeError(p.stderr or p.stdout)
    return p.stdout.strip()
def sql(text, role=None, fail=None):
    prefix = "BEGIN; " + (f"SET LOCAL ROLE {role}; " if role else "")
    p = subprocess.run(["docker","exec","-i",NAME,"psql","-X","-qAt","-v","ON_ERROR_STOP=1","-U","postgres","-d","delivery"],
                       input=prefix + text + "; ROLLBACK;", text=True, capture_output=True, timeout=30)
    if fail:
        if p.returncode == 0 or fail not in p.stderr:
            raise AssertionError(f"Expected {fail}: {p.stderr} {p.stdout}")
        return
    if p.returncode:
        raise RuntimeError(p.stderr)
    return p.stdout.strip()
def check(name, fn):
    fn()
    checks.append({"check":name,"status":"PASS"})
def eq(a,b):
    if a != b: raise AssertionError(f"{a!r} != {b!r}")
W = "00000000-0000-4000-8000-000000000001"
OTHER = "00000000-0000-4000-8000-000000000002"
C = "00000000-0000-4000-8000-000000000003"
L = "00000000-0000-4000-8000-000000000004"
CASE = "00000000-0000-4000-8000-000000000005"
CONF = "00000000-0000-4000-8000-000000000006"
A = "00000000-0000-4000-8000-000000000007"
MSG = "00000000-0000-4000-8000-000000000008"
tenant = ["destination_connections","connection_secrets","effect_lineage","attempts","effect_observations","observation_content","intent_content","inbox","outbox"]
alltables = tenant + ["schema_version","credential_budget","job_index"]
connection = f"INSERT INTO hp_delivery.destination_connections VALUES ('{W}','{C}','synthetic',1,decode(repeat('ab',32),'hex'),'OK',NULL)"
lineage = f"INSERT INTO hp_delivery.effect_lineage(workspace_id,lineage_id,case_id,connection_id,effect_kind,config_id,state,updated_at) VALUES ('{W}','{L}','{CASE}','{C}','CREATE_TASK','{CONF}','PREPARED',now())"
attempt = f"INSERT INTO hp_delivery.attempts VALUES ('{W}','{A}','{L}','{CONF}',1,'{MSG}','sha256:'||repeat('a',64),now(),now(),1,now(),NULL,NULL)"
seed = connection + ";" + lineage + ";" + attempt + ";"
fullseed = seed + f"""
INSERT INTO hp_delivery.connection_secrets VALUES('{W}','{A}','{C}','{MSG}','sha256:'||repeat('a',64),'synthetic',now(),NULL);
INSERT INTO hp_delivery.effect_observations VALUES('{W}','{MSG}','{L}','{A}','POSSIBLE_EFFECT',now(),'SYNTHETIC','sha256:'||repeat('a',64),NULL);
INSERT INTO hp_delivery.observation_content VALUES('{W}','{MSG}','{CONF}','sha256:'||repeat('a',64),1,'synthetic',now(),now()+interval '1 day');
INSERT INTO hp_delivery.intent_content VALUES('{W}','{A}','{CONF}','sha256:'||repeat('a',64),1,now(),now()+interval '1 day');
INSERT INTO hp_delivery.inbox(workspace_id,message_id,digest,disposition) VALUES('{W}','{MSG}','sha256:'||repeat('a',64),'APPLIED');
INSERT INTO hp_delivery.outbox(workspace_id,message_id,kind,payload_ref,payload_sha256,payload_bytes,digest,expires_at) VALUES('{W}','{MSG}','synthetic','{CONF}','sha256:'||repeat('a',64),1,'sha256:'||repeat('a',64),now()+interval '1 day');
"""
try:
    run(["docker","run","--detach","--name",NAME,"--network","none","--env","POSTGRES_HOST_AUTH_METHOD=trust",IMAGE])
    for _ in range(60):
        p = subprocess.run(["docker","exec",NAME,"pg_isready","-U","postgres"],capture_output=True)
        if p.returncode == 0:
            time.sleep(2)
            ready = subprocess.run(["docker","exec",NAME,"pg_isready","-h","127.0.0.1","-U","postgres"],capture_output=True)
            if ready.returncode == 0: break
        time.sleep(1)
    else: raise RuntimeError("PostgreSQL did not become ready")
    bootstrap = "CREATE ROLE hp_delivery_migrator NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;"
    for role in ["hp_delivery_app","hp_delivery_worker","hp_delivery_purge"]:
        bootstrap += f"CREATE ROLE {role} NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;"
    bootstrap += "CREATE DATABASE delivery OWNER hp_delivery_migrator;"
    run(["docker","exec","-i",NAME,"psql","-X","-v","ON_ERROR_STOP=1","-U","postgres"],bootstrap)
    migration = (ROOT/"migrations/001_delivery.sql").read_text(encoding="utf-8")
    check("migration matches pinned HP-MVP-SQL-DELIVERY",lambda:eq(hashlib.sha256(migration.encode()).hexdigest(),"9b23a5a0f8617025f20012f38d0b961010d9d4fbbbc47c3059c1b651a16f155e"))
    run(["docker","exec","-i",NAME,"psql","-X","-v","ON_ERROR_STOP=1","-U","postgres","-d","delivery"],
        "SET ROLE hp_delivery_migrator;\n" + migration)
    check("migration applies",lambda:eq(sql("SELECT version FROM hp_delivery.schema_version"),"1"))
    check("all 12 tables owned by migrator",lambda:eq(sql("SELECT count(*) FROM pg_tables WHERE schemaname='hp_delivery' AND tableowner='hp_delivery_migrator'"),"12"))
    check("exact tenant RLS policies and FORCE",lambda:eq(sql("SELECT count(*) FROM pg_class c JOIN pg_namespace n ON c.relnamespace=n.oid JOIN pg_policy p ON p.polrelid=c.oid WHERE n.nspname='hp_delivery' AND c.relrowsecurity AND c.relforcerowsecurity AND p.polqual IS NOT NULL AND p.polwithcheck IS NOT NULL"),"9"))
    check("trigger and function installed",lambda:eq(sql("SELECT count(*) FROM pg_trigger WHERE tgname='effect_fence_immutable' AND NOT tgisinternal"),"1"))
    check("schema owned by migrator",lambda:eq(sql("SELECT pg_get_userbyid(nspowner) FROM pg_namespace WHERE nspname='hp_delivery'"),"hp_delivery_migrator"))
    check("fence function owned by migrator",lambda:eq(sql("SELECT pg_get_userbyid(proowner) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='hp_delivery' AND p.proname='protect_effect_fence'"),"hp_delivery_migrator"))
    check("roles cannot bypass RLS or inherit purge",lambda:eq(sql("SELECT count(*) FROM pg_roles WHERE rolname IN ('hp_delivery_migrator','hp_delivery_app','hp_delivery_worker','hp_delivery_purge') AND NOT rolsuper AND NOT rolbypassrls AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolcanlogin"),"4"))
    for role in ["hp_delivery_app","hp_delivery_worker","hp_delivery_purge"]:
        check(role+" not table owner",lambda role=role:eq(sql(f"SELECT count(*) FROM pg_tables WHERE schemaname='hp_delivery' AND tableowner='{role}'"),"0"))
        check(role+" no DDL",lambda role=role:sql("CREATE TABLE hp_delivery.forbidden(x int)",role,"permission denied"))
        check(role+" schema USAGE only",lambda role=role:eq(sql(f"SELECT has_schema_privilege('{role}','hp_delivery','USAGE') AND NOT has_schema_privilege('{role}','hp_delivery','CREATE')"),"t"))
        check(role+" cannot execute fence function directly",lambda role=role:eq(sql(f"SELECT has_function_privilege('{role}','hp_delivery.protect_effect_fence()','EXECUTE')"),"f"))
        check(role+" own tenant read",lambda role=role:eq(sql(connection+f";SET LOCAL ROLE {role};SET LOCAL hp.workspace_id='{W}';SELECT count(*) FROM hp_delivery.destination_connections"),"1"))
        check(role+" other tenant invisible",lambda role=role:eq(sql(connection+f";SET LOCAL ROLE {role};SET LOCAL hp.workspace_id='{OTHER}';SELECT count(*) FROM hp_delivery.destination_connections"),"0"))
        check(role+" unset tenant invisible",lambda role=role:eq(sql(connection+f";SET LOCAL ROLE {role};SELECT count(*) FROM hp_delivery.destination_connections"),"0"))
        for table in tenant:
            check(role+" own row "+table,lambda role=role,table=table:eq(sql(fullseed+f"SET LOCAL ROLE {role};SET LOCAL hp.workspace_id='{W}';SELECT count(*) FROM hp_delivery.{table}"),"1"))
            check(role+" foreign row hidden "+table,lambda role=role,table=table:eq(sql(fullseed+f"SET LOCAL ROLE {role};SET LOCAL hp.workspace_id='{OTHER}';SELECT count(*) FROM hp_delivery.{table}"),"0"))
            if table not in ["effect_observations","observation_content","intent_content","effect_lineage"]:
                check(role+" cross-tenant update rejected "+table,lambda role=role,table=table:sql(fullseed+f"SET LOCAL ROLE {role};SET LOCAL hp.workspace_id='{W}';UPDATE hp_delivery.{table} SET workspace_id='{OTHER}'",fail="row-level security"))
        for table in alltables:
            allowed = {"SELECT"}
            if table in tenant:
                allowed |= {"INSERT"}
                if table not in ["effect_observations","observation_content","intent_content"]: allowed.add("UPDATE")
                if role == "hp_delivery_purge":
                    allowed = {"SELECT","DELETE"} if table in ["effect_observations","observation_content","intent_content"] else {"SELECT","INSERT","UPDATE","DELETE"}
            elif table == "job_index":
                allowed = set() if role == "hp_delivery_app" else {"SELECT","INSERT","UPDATE","DELETE"}
            elif table == "credential_budget":
                allowed = set() if role == "hp_delivery_app" else {"SELECT","INSERT","UPDATE"}
                if role == "hp_delivery_purge": allowed.add("DELETE")
            for privilege in ["SELECT","INSERT","UPDATE","DELETE","TRUNCATE","REFERENCES","TRIGGER","MAINTAIN"]:
                check(role+" grant "+table+" "+privilege,lambda role=role,table=table,privilege=privilege,allowed=allowed:eq(sql(f"SELECT has_table_privilege('{role}','hp_delivery.{table}','{privilege}')"),"t" if privilege in allowed else "f"))
        check(role+" cross-tenant insert rejected",lambda role=role:sql(f"SET LOCAL hp.workspace_id='{OTHER}';"+connection,role,"row-level security"))
    check("app cannot read global job index",lambda:sql("SELECT * FROM hp_delivery.job_index","hp_delivery_app","permission denied"))
    check("app cannot read credential budget",lambda:sql("SELECT * FROM hp_delivery.credential_budget","hp_delivery_app","permission denied"))
    for role in ["hp_delivery_app","hp_delivery_worker"]:
        check(role+" cannot delete",lambda role=role:sql("DELETE FROM hp_delivery.destination_connections",role,"permission denied"))
        for table in ["effect_observations","observation_content","intent_content"]:
            check(role+" immutable "+table,lambda role=role,table=table:sql(f"UPDATE hp_delivery.{table} SET workspace_id='{W}'",role,"permission denied"))
        check(role+" no purge membership",lambda role=role:eq(sql(f"SELECT pg_has_role('{role}','hp_delivery_purge','MEMBER')"),"f"))
    check("lineage unique per case/effect",lambda:sql(seed+lineage,fail="duplicate key"))
    check("attempt command unique",lambda:sql(seed+attempt.replace(A,OTHER).replace(",1,",",2,"),fail="duplicate key"))
    check("inbox message unique",lambda:sql(f"INSERT INTO hp_delivery.inbox(workspace_id,message_id,digest,disposition) VALUES('{W}','{MSG}','sha256:'||repeat('a',64),'APPLIED'); INSERT INTO hp_delivery.inbox SELECT * FROM hp_delivery.inbox",fail="duplicate key"))
    check("outbox message unique",lambda:sql(fullseed+"INSERT INTO hp_delivery.outbox SELECT * FROM hp_delivery.outbox",fail="duplicate key"))
    check("KNOWN_NO_EFFECT requires proof",lambda:sql(seed+"UPDATE hp_delivery.effect_lineage SET state='KNOWN_NO_EFFECT'",fail="check constraint"))
    check("KNOWN_NO_EFFECT with proof allowed",lambda:sql(seed+f"UPDATE hp_delivery.effect_lineage SET state='KNOWN_NO_EFFECT',no_effect_proof_ref='{MSG}'"))
    check("dispatch sets permanent fence",lambda:eq(sql(seed+"UPDATE hp_delivery.effect_lineage SET state='DISPATCHING';SELECT ever_dispatch_fenced FROM hp_delivery.effect_lineage"),"t"))
    check("fence cannot clear",lambda:sql(seed+"UPDATE hp_delivery.effect_lineage SET state='DISPATCHING';UPDATE hp_delivery.effect_lineage SET ever_dispatch_fenced=false",fail="immutable"))
    for state in ["DISPATCHING","AMBIGUOUS","RESULT_ID_OBSERVED","TARGET_OBSERVED"]:
        check(state+" cannot reset",lambda state=state:sql(seed+f"UPDATE hp_delivery.effect_lineage SET state='{state}';UPDATE hp_delivery.effect_lineage SET state='PREPARED'",fail="cannot be reset"))
    check("lineage identity immutable",lambda:sql(seed+f"UPDATE hp_delivery.effect_lineage SET config_id='{OTHER}'",fail="immutable"))
    report={"source_commit":run(["git","rev-parse","HEAD"]),"started_at":started_at,
            "finished_at":datetime.datetime.now(datetime.timezone.utc).isoformat(),
            "command":[sys.executable,*sys.argv],"python":platform.python_version(),"host":platform.platform(),
            "docker":run(["docker","--version"]),"isolation":{"network":"none","published_ports":[],"host_volumes":[]},
            "runner_sha256":hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),
            "image":IMAGE,"server":sql("SELECT version()"),
            "migration_sha256":hashlib.sha256(migration.encode()).hexdigest(),"checks":checks,
            "limits":["isolated SQL only","no external provider","RLS is not user authorization","no deployment or full revocation protocol"]}
    rendered = json.dumps(report,ensure_ascii=False,indent=2) + "\n"
    if args.report:
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(rendered,encoding="utf-8")
    print(json.dumps({"checks":len(checks),"status":"PASS","source_commit":report["source_commit"],"report":str(args.report)}) if args.report else rendered)
finally:
    subprocess.run(["docker","rm","-f","-v",NAME],capture_output=True)

