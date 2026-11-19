#!/usr/bin/env python3
"""Intake receipt and sender use only a newly owned PostgreSQL 18.7 sandbox."""
import argparse,hashlib,json,os,pathlib,platform,subprocess,time,uuid,sys
ROOT=pathlib.Path(__file__).resolve().parents[1]
IMAGE='docker.io/library/postgres@sha256:75dcac3dfb1cd01ca3536910bbc84128759362f8b118bf8c55b38a94f68f3b73'
a=argparse.ArgumentParser();a.add_argument('--go',default='go');a.add_argument('--output',type=pathlib.Path,default=ROOT/'evidence/HIT-118');args=a.parse_args();out=args.output.resolve();out.mkdir(parents=True,exist_ok=True)
env=dict(os.environ,GOTOOLCHAIN='local')
def run(cmd,**kw):return subprocess.run(cmd,cwd=ROOT,capture_output=True,text=True,env=env,timeout=180,**kw)
def require(r):
 if r.returncode:raise RuntimeError('sandbox setup failed: '+r.stderr[-2000:])
 return r.stdout.strip()
version=require(run([args.go,'version']));assert version.startswith('go version go1.27.2 ')
name='hit118-'+uuid.uuid4().hex[:12];started=False
report={'source_sha':require(run(['git','rev-parse','HEAD'])),'dirty':require(run(['git','status','--porcelain'])).splitlines(),'go':version,'host':platform.platform(),'image':IMAGE,'started_utc':time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime()),'scope':'local receipt and guarded HTTPS/mTLS sender against controlled Processing; no live IAM/Stripe or deployment','commands':[]}
migration_sha=hashlib.sha256((ROOT/'migrations/0001_intake.sql').read_bytes()).hexdigest()
assert migration_sha=='0d477eac94bf5dd9a861c614b9970a7f828558ae5a7663874b62475f26247093'
report['migration_sha256']=migration_sha
report['python']=sys.version
report['source_hashes']={str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(ROOT.rglob('*')) if p.is_file() and (p.suffix in ('.go','.sql','.py') or p.name in ('go.mod','go.sum','normalized_signal.schema.json'))}
try:
 require(run(['docker','image','inspect',IMAGE]))
 require(run(['docker','run','--detach','--name',name,'--publish','127.0.0.1::5432','--memory','384m','--cpus','1','--pids-limit','128','--tmpfs','/var/lib/postgresql:rw,nosuid,nodev,size=256m','--env','POSTGRES_HOST_AUTH_METHOD=trust',IMAGE,'postgres','-c','shared_buffers=32MB','-c','max_connections=40','-c','log_statement=none','-c','log_min_error_statement=panic','-c','log_parameter_max_length=0','-c','log_parameter_max_length_on_error=0']));started=True
 for _ in range(60):
  if run(['docker','exec',name,'pg_isready','-h','127.0.0.1','-U','postgres']).returncode==0:break
  time.sleep(.5)
 else:raise RuntimeError('PostgreSQL did not become ready')
 psql=['docker','exec','-i',name,'psql','-h','127.0.0.1','-X','-v','ON_ERROR_STOP=1','-U','postgres']
 bootstrap=(ROOT/'tests/postgres/bootstrap.sql').read_text();require(run(psql,input=bootstrap))
 migrate=['docker','exec','-i',name,'psql','-h','127.0.0.1','-X','-v','ON_ERROR_STOP=1','-U','hp_intake_migrator','-d','intake_test'];require(run(migrate,input=(ROOT/'migrations/0001_intake.sql').read_text()))
 # Visibility for an observer test only; neither application privileges nor RLS change.
 require(run(psql,input='GRANT pg_read_all_stats TO hp_intake_migrator;'))
 report['storage_settings']={k:require(run(psql+['-Atc','SHOW '+k])) for k in ['fsync','full_page_writes','synchronous_commit']}
 assert all(v=='on' for v in report['storage_settings'].values())
 report['postgres']=require(run(psql+['-Atc','SHOW server_version']));assert report['postgres'].startswith('18.7')
 address=require(run(['docker','port',name,'5432/tcp']));assert address.startswith('127.0.0.1:') and '\n' not in address
 env['HIT86_APP_DSN']='postgres://hp_intake_app@'+address+'/intake_test?sslmode=disable'
 env['HIT118_WORKER_DSN']='postgres://hp_intake_worker@'+address+'/intake_test?sslmode=disable'
 env['HIT86_MIGRATOR_DSN']='postgres://hp_intake_migrator@'+address+'/intake_test?sslmode=disable'
 for label,cmd in [('unit',[args.go,'test','-race','-count=1','-json','./...']),('integration',[args.go,'test','-tags=integration','-race','-count=1','-json','./...']),('vet',[args.go,'vet','-tags=integration','./...']),('build',[args.go,'build','./...']),('modules',[args.go,'mod','verify']),('schema',[sys.executable,str(ROOT/'tools/check_postgres.py')])]:
  before=time.monotonic();r=run(cmd);(out/(label+'.stdout')).write_text(r.stdout);(out/(label+'.stderr')).write_text(r.stderr);report['commands'].append({'name':label,'argv':cmd,'exit_code':r.returncode,'seconds':round(time.monotonic()-before,3)});print(label,'exit',r.returncode,flush=True)
  if r.returncode:raise RuntimeError(label+' failed; inspect saved logs')
  if label=='integration':
   records=[json.loads(l) for l in r.stdout.splitlines()];assert any(x.get('Test','').startswith('TestIntegrationSender') and x.get('Action')=='pass' for x in records);assert not any(x.get('Test','').startswith('TestIntegration') and x.get('Action')=='skip' for x in records)
 assert all(hashlib.sha256((ROOT/p).read_bytes()).hexdigest()==h for p,h in report['source_hashes'].items())
 report['state']='PASSED'
except Exception as e:
 report['state']='FAILED';report['error']=str(e);print(str(e),flush=True)
finally:
 if started:report['owned_container_removed']=run(['docker','rm','--force',name]).returncode==0
 report['finished_utc']=time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime());(out/'commands.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
raise SystemExit(0 if report.get('state')=='PASSED' and report.get('owned_container_removed') else 1)
