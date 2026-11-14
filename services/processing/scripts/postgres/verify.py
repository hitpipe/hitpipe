#!/usr/bin/env python3
"""Real PostgreSQL qualification of the bounded HIT-76 slice, stdlib + Docker CLI.
Never accepts a DSN or an existing server. Owns exactly one isolated test container.
"""
import argparse
import datetime as dt
import hashlib
import json
import os
import pathlib
import subprocess
import time
import uuid

IMAGE='docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f'
SQL_SHA='a4f89c9b78a09c6d548422d661d9dcab7171e87397dbb2ebb2f7bc7f9958bb93'
MODULE=pathlib.Path(__file__).resolve().parents[2]
REPO=MODULE.parents[1]
MIGRATIONS=MODULE/'scripts/migrations'

def now(): return dt.datetime.now(dt.timezone.utc).isoformat()

class Run:
 def __init__(self,output):
  self.output=output;output.mkdir(parents=True,exist_ok=False)
  self.records=[];self.name='hit76-'+uuid.uuid4().hex;self.db='hit76_'+uuid.uuid4().hex
  self.source=subprocess.check_output(['git','rev-parse','HEAD'],cwd=REPO,text=True).strip()
  self.container=None
  self.tree_status=subprocess.check_output(['git','status','--porcelain'],cwd=REPO,text=True)
  self.script_sha256=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
 def command(self,label,args,stdin=None,required=True):
  started=now()
  r=subprocess.run(args,input=stdin,capture_output=True,cwd=REPO,timeout=180)
  entry={'label':label,'argv':args,'cwd':str(REPO),'environment':'Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.','source_sha':self.source,'tree_status_at_start':self.tree_status,'runner_sha256':self.script_sha256,'started_at':started,'finished_at':now(),'exit_code':r.returncode}
  for kind,b in [('stdin',stdin),('stdout',r.stdout),('stderr',r.stderr)]:
   if b is not None:
    file=f'{len(self.records):03d}-{label}.{kind}';(self.output/file).write_bytes(b)
    entry[kind]={'file':file,'bytes':len(b),'sha256':hashlib.sha256(b).hexdigest()}
  self.records.append(entry);self.save()
  if required and r.returncode:raise RuntimeError(f'{label}: exit {r.returncode}: '+r.stderr.decode()[-1000:])
  return r
 def save(self):
  (self.output/'commands.json').write_text(json.dumps(self.records,indent=2)+'\n')
 def sql(self,label,text,role='hp_processing_app',database=None,required=True):
  return self.command(label,['docker','exec','-i',self.container,'psql','-X','-q','-A','-t','-v','VERBOSITY=verbose','-v','ON_ERROR_STOP=1','-U',role,'-d',database or self.db],text.encode(),required=required)
 def expect(self,label,text,expected,role='hp_processing_app',database=None):
  got=self.sql(label,text,role,database).stdout.decode().strip()
  if got!=expected:raise AssertionError(f'{label}: wanted {expected!r}, got {got!r}')
 def main(self):
  raw=(MIGRATIONS/'001_processing.sql').read_bytes()
  if hashlib.sha256(raw).hexdigest()!=SQL_SHA:raise RuntimeError('migration differs from accepted bytes')
  if os.environ.get('DOCKER_HOST'):
   raise RuntimeError('unset DOCKER_HOST; only the selected local Unix-socket context is supported')
  context=self.command('docker-context',['docker','context','inspect'])
  endpoint=json.loads(context.stdout)[0]['Endpoints']['docker']['Host']
  if not endpoint.startswith('unix://'):raise RuntimeError('only a local Unix-socket Docker context is supported')
  self.command('docker-version',['docker','version','--format','{{json .}}'])
  self.command('image-inspect',['docker','image','inspect',IMAGE])
  try:
   r=self.command('container-create',['docker','run','--detach','--rm','--name',self.name,'--label','hitpipe.task=HIT-76','--network','none','--read-only','--tmpfs','/tmp:rw','--tmpfs','/var/run/postgresql:rw','--tmpfs','/var/lib/postgresql:rw,size=536870912','--mount',f'type=bind,source={MIGRATIONS},target=/migrations,readonly','-e','POSTGRES_HOST_AUTH_METHOD=trust',IMAGE,'postgres','-c','listen_addresses=','-c','fsync=on','-c','full_page_writes=on','-c','synchronous_commit=on'])
   self.container=r.stdout.decode().strip()
   for attempt in range(60):
    r=self.command('ready',['docker','exec',self.container,'pg_isready','-U','postgres'],required=False)
    logs=self.command('init-readiness',['docker','logs',self.container],required=False)
    if r.returncode==0 and b'PostgreSQL init process complete; ready for start up.' in logs.stdout+logs.stderr:break
    time.sleep(0.5)
   else:raise RuntimeError('PostgreSQL did not become ready')
   self.command('postgres-version',['docker','exec',self.container,'postgres','--version'])
   self.expect('postgres-settings',"SELECT current_setting('server_version_num'),current_setting('fsync'),current_setting('full_page_writes'),current_setting('synchronous_commit'),current_setting('listen_addresses');",'180007|on|on|on|',role='postgres',database='postgres')
   self.sql('roles',"\\i /migrations/roles.sql\n",role='postgres',database='postgres')
   self.sql('database',f"CREATE DATABASE {self.db} OWNER hp_processing_owner;\nREVOKE ALL ON DATABASE {self.db} FROM PUBLIC;\nGRANT CONNECT ON DATABASE {self.db} TO hp_processing_migrator,hp_processing_app,hp_processing_worker,hp_processing_purge;",role='postgres',database='postgres')
   self.sql('migration-first',"\\i /migrations/apply.sql\n",role='hp_processing_migrator')
   first=self.sql('migration-identity-before',"SET ROLE hp_processing_owner;SELECT installed_at FROM hp_processing.schema_version;",role='hp_processing_migrator')
   again=self.sql('migration-again',"\\i /migrations/apply.sql\n",role='hp_processing_migrator')
   if b'ALREADY_INSTALLED processing schema version 1' not in again.stdout:raise AssertionError('no explicit already-installed result')
   last=self.sql('migration-identity-after',"SET ROLE hp_processing_owner;SELECT installed_at FROM hp_processing.schema_version;",role='hp_processing_migrator')
   if first.stdout!=last.stdout:raise AssertionError('migration reinstalled')
   self.checks()
   summary={'status':'PASS','source_sha':self.source,'tree_status_at_start':self.tree_status,'runner_sha256':self.script_sha256,'migration_sha256':SQL_SHA,'image':IMAGE,'database':self.db,'container':self.container,'finished_at':now(),'scope':'Real PostgreSQL schema/role/RLS/constraint/transaction tests; no event ingestion, IAM authority or Stripe→ClickUp E2E.'}
  finally:
   if self.container:
    self.command('container-logs',['docker','logs',self.container],required=False)
    self.command('cleanup',['docker','rm','-f','-v',self.container])
    probe=self.command('cleanup-probe',['docker','container','inspect',self.container],required=False)
    if probe.returncode==0:raise AssertionError('test container remains')
  (self.output/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
  print(json.dumps(summary))
 def failure(self,label,text,sqlstate,role='hp_processing_app'):
  r=self.sql(label,text,role,required=False)
  if r.returncode!=3 or (sqlstate+':').encode() not in r.stderr:
   raise AssertionError(f'{label}: expected SQLSTATE {sqlstate}, got '+r.stderr.decode())
  self.records[-1]['expected_exit_code']=3
  self.records[-1]['expected_sqlstate']=sqlstate
  self.save()
 def checks(self):
  ws1='11111111-1111-4111-8111-111111111111';ws2='22222222-2222-4222-8222-222222222222'
  op='33333333-3333-4333-8333-333333333333';config='44444444-4444-4444-8444-444444444444'
  case='55555555-5555-4555-8555-555555555555';episode='66666666-6666-4666-8666-666666666666'
  other='77777777-7777-4777-8777-777777777777';actor='88888888-8888-4888-8888-888888888888'
  digest='sha256:'+'a'*64
  def scope(ws): return f"BEGIN; SET LOCAL hp.workspace_id='{ws}'; SET LOCAL search_path=hp_processing,pg_catalog; "
  for role in ['hp_processing_app','hp_processing_worker']:
   self.expect('role-'+role,"SELECT current_user,rolsuper,rolbypassrls,rolcreatedb,rolcreaterole,rolreplication,pg_has_role(current_user,'hp_processing_owner','MEMBER'),pg_has_role(current_user,'hp_processing_migrator','MEMBER'),pg_has_role(current_user,'hp_processing_purge','MEMBER'),has_schema_privilege(current_user,'hp_processing','CREATE'),(SELECT nspowner=current_user::regrole FROM pg_namespace WHERE nspname='hp_processing') FROM pg_roles WHERE rolname=current_user;",role+'|f|f|f|f|f|f|f|f|f|f',role)
  self.expect('owner-and-rls',"SELECT count(*) FROM pg_class c JOIN pg_namespace n ON c.relnamespace=n.oid WHERE n.nspname='hp_processing' AND c.relkind='r' AND c.relname NOT IN ('job_index','schema_version') AND c.relrowsecurity AND c.relforcerowsecurity AND c.relowner='hp_processing_owner'::regrole;",'12','postgres')
  for ws in [ws1,ws2]:
   self.sql('seed-'+ws[:8],scope(ws)+f"""
INSERT INTO operations VALUES('{ws}','{op}','{actor}',1,'ACTIVE','2026-11-13T00:00:00Z',NULL,NULL,NULL);
INSERT INTO accepted_configs VALUES('{ws}','{config}','{op}','1.0','paid-order-v1','{{"pack_id":"synthetic"}}','{digest}','{actor}','2026-11-13T00:00:00Z','synthetic');
UPDATE operations SET selected_config_id='{config}' WHERE operation_id='{op}';
INSERT INTO case_identity(workspace_id,case_id,operation_id,config_id,source_account_id,livemode,session_id,admitted_at,owner_user_id,state,revision,outcome_knowledge) VALUES('{ws}','{case}','{op}','{config}','acct_synthetic',false,'cs_synthetic','2026-11-13T00:01:00Z','{actor}','RECEIVED',1,'UNESTABLISHED');
INSERT INTO episodes(workspace_id,episode_id,case_id,started_at,deadline_at,automatic_writes) VALUES('{ws}','{episode}','{case}','2026-11-13T00:01:00Z','2026-11-13T00:06:00Z',1);
COMMIT;""")
  tables=['operations','drafts','accepted_configs','case_identity','episodes','case_content','safety_history','history_content','user_commands','notifications','inbox','outbox']
  for role in ['hp_processing_app','hp_processing_worker']:
   for table in tables:self.expect('no-scope-'+role+'-'+table,f'SELECT count(*) FROM hp_processing.{table};','0',role)
   self.expect('scoped-reset-'+role,scope(ws1)+"SELECT count(*) FROM operations; COMMIT; BEGIN; SELECT count(*) FROM hp_processing.operations; COMMIT;",'1\n0',role)
   self.expect('own-only-'+role,scope(ws2)+f"SELECT count(*) FROM case_identity WHERE workspace_id='{ws1}'; SELECT count(*) FROM case_identity WHERE workspace_id='{ws2}';COMMIT;",'0\n1',role)
   self.failure('foreign-write-'+role,scope(ws1)+f"INSERT INTO operations(workspace_id,operation_id,owner_user_id,revision,state) VALUES('{ws2}','{other}','{actor}',1,'DRAFT');",'42501',role)
   self.failure('no-scope-write-'+role,f"INSERT INTO hp_processing.operations(workspace_id,operation_id,owner_user_id,revision,state) VALUES('{ws1}','{other}','{actor}',1,'DRAFT');",'42501',role)
   self.failure('no-owner-role-'+role,'SET ROLE hp_processing_owner;','42501',role)
   self.failure('config-immutable-'+role,scope(ws1)+f"UPDATE accepted_configs SET evaluator_version='replacement' WHERE config_id='{config}';",'42501',role)
  self.failure('no-global-index-app','SELECT * FROM hp_processing.job_index;','42501')
  self.expect('worker-index-metadata',"SELECT string_agg(column_name,',' ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema='hp_processing' AND table_name='job_index';",'job_id,workspace_id,kind,due_at,claim_token,lease_until','hp_processing_worker')
  self.failure('session-unique',scope(ws1)+f"INSERT INTO case_identity(workspace_id,case_id,operation_id,config_id,source_account_id,livemode,session_id,admitted_at,owner_user_id,state,revision,outcome_knowledge) VALUES('{ws1}','{other}','{op}','{config}','acct_synthetic',false,'cs_synthetic',now(),'{actor}','RECEIVED',1,'UNESTABLISHED');",'23505')
  self.failure('single-active',scope(ws1)+f"INSERT INTO operations(workspace_id,operation_id,owner_user_id,revision,state) VALUES('{ws1}','{other}','{actor}',1,'ACTIVE');",'23505')
  for field,value in {'workspace_id':f"'{ws2}'",'case_id':f"'{other}'",'operation_id':f"'{other}'",'config_id':f"'{other}'",'source_account_id':"'other'",'livemode':'true','session_id':"'other'",'admitted_at':"'2026-11-14T00:00:00Z'"}.items():
   self.failure('case-pin-'+field,scope(ws1)+f"UPDATE case_identity SET {field}={value} WHERE case_id='{case}';",'P0001')
  for value in ["NULL","'2026-11-14T00:00:00Z'"]:
   self.failure('activation-pin-'+('null' if value=='NULL' else 'later'),scope(ws1)+f"UPDATE operations SET first_activation_at={value} WHERE operation_id='{op}';",'P0001')
  for field,value in {'workspace_id':f"'{ws2}'",'episode_id':f"'{other}'",'case_id':f"'{other}'",'started_at':"'2026-11-13T00:02:00Z'",'deadline_at':"'2026-11-13T00:07:00Z'",'automatic_writes':'0'}.items():
   self.failure('episode-pin-'+field,scope(ws1)+f"UPDATE episodes SET {field}={value} WHERE episode_id='{episode}';",'P0001')
  self.failure('attempt-limit',scope(ws1)+f"UPDATE episodes SET automatic_writes=4 WHERE episode_id='{episode}';",'23514')
  self.failure('episode-duration',scope(ws1)+f"INSERT INTO episodes(workspace_id,episode_id,case_id,started_at,deadline_at) VALUES('{ws1}','{other}','{case}','2026-11-13T00:01:00Z','2026-11-13T00:06:01Z');",'23514')
  # One statement succeeds, the following statement fails: both must roll back
  # when that connection closes. A separate successful pair persists below.
  payload='99999999-9999-4999-8999-999999999999'
  self.failure('atomic-rollback',scope(ws1)+f"UPDATE case_identity SET revision=2 WHERE case_id='{case}'; INSERT INTO outbox(workspace_id,message_id,kind,payload_ref,payload_sha256,payload_bytes,digest,expires_at) VALUES('{ws1}','{other}','synthetic','{payload}','{digest}',1,'{digest}',now()-interval '1second');COMMIT;",'23514')
  self.expect('rollback-new-connection',scope(ws1)+f"SELECT revision FROM case_identity WHERE case_id='{case}';SELECT count(*) FROM outbox;COMMIT;",'1\n0')
  self.sql('atomic-commit',scope(ws1)+f"UPDATE case_identity SET revision=2 WHERE case_id='{case}';INSERT INTO outbox(workspace_id,message_id,kind,payload_ref,payload_sha256,payload_bytes,digest,expires_at) VALUES('{ws1}','{other}','synthetic','{payload}','{digest}',1,'{digest}',now()+interval '1hour');COMMIT;")
  self.expect('committed-new-connection',scope(ws1)+f"SELECT revision FROM case_identity WHERE case_id='{case}';SELECT count(*) FROM outbox WHERE message_id='{other}'; SELECT automatic_writes,started_at AT TIME ZONE 'UTC',deadline_at AT TIME ZONE 'UTC' FROM episodes WHERE episode_id='{episode}';COMMIT;",'2\n1\n1|2026-11-13 00:01:00|2026-11-13 00:06:00')
  self.failure('constraint-after-reconnect',scope(ws1)+f"UPDATE episodes SET automatic_writes=0 WHERE episode_id='{episode}';",'P0001')

if __name__=='__main__':
 parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--output',required=True,type=pathlib.Path);a=parser.parse_args()
 Run(a.output.resolve()).main()
