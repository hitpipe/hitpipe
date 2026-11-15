#!/usr/bin/env python3
"""HIT-100 four real private owner HTTP operations on isolated PostgreSQL18.7.
Only owned disposable resources and labelled controlled owner dependencies.
"""
import argparse,hashlib,json,os,pathlib,uuid
import verify_preparation
class Run(verify_preparation.Run):
 def __init__(self,output,go):
  super().__init__(output,go);self.name='hit100-'+uuid.uuid4().hex;self.script_sha256=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
 def command(self,label,args,*a,**kw):
  args=['hitpipe.task=HIT-100' if x in ['hitpipe.task=HIT-76','hitpipe.task=HIT-78','hitpipe.task=HIT-79'] else x for x in args]
  if label=='postgres-storage-tests':
   args[2:2]=['-e','HIT100_CAPTURE_FILE=/tmp/hit100-captures.jsonl'];args=['-test.timeout=180s' if x=='-test.timeout=60s' else x for x in args]
  try:return super().command(label,args,*a,**kw)
  finally:
   if self.records:self.records[-1]['environment']={k:os.environ.get(k,'<unset>') for k in ['PATH','GOTOOLCHAIN','GOFLAGS','GOEXPERIMENT','CGO_ENABLED','GOMAXPROCS','DOCKER_HOST','PYTHONDONTWRITEBYTECODE']};self.save()
 def checks(self):
  super().checks();module=str(pathlib.Path(__file__).resolve().parents[2]);self.command('go-build',[self.go,'-C',module,'build','./...']);r=self.command('draft-api-captures',['docker','exec',self.container,'cat','/tmp/hit100-captures.jsonl']);(self.output/'api-captures.jsonl').write_bytes(r.stdout)
 def main(self):
  super().main();p=self.output/'summary.json';s=json.loads(p.read_text());s.update(task='HIT-100',scope='Local four draft/readiness/rehearsal HTTPS TLS1.3 mTLS owner operations, actual PG18.7 app-role RLS, durable owner files and full command replay. Host Go1.27.2 race/vet/build; separate LinuxCGO0 DB binary, not Linux race. Controlled session/authority/owner-binding ports, no live IAM/BFF/providers or activation qualification.',local_verification_complete=True,project_os_handoff_ready='NOT_EVALUATED_BY_RUNNER');p.write_text(json.dumps(s,indent=2)+'\n');contexts=[];commands=[];keys=['cwd','environment','source_sha','tree_status_at_start','runner_sha256']
  for x in self.records:
   c={k:x[k] for k in keys if k in x}
   if c not in contexts:contexts.append(c)
   commands.append({'context':contexts.index(c),**{k:v for k,v in x.items() if k not in keys}})
  assert [{**contexts[x['context']],**{k:v for k,v in x.items() if k!='context'}} for x in commands]==self.records
  (self.output/'review-commands.json').write_text(json.dumps({'format':'exact commands with deduplicated contexts; merge context into each command','original_sha256':hashlib.sha256((self.output/'commands.json').read_bytes()).hexdigest(),'lossless_round_trip_verified':True,'contexts':contexts,'commands':commands},indent=2)+'\n');print(json.dumps(s))
if __name__=='__main__':
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--output',type=pathlib.Path,required=True);p.add_argument('--go',type=pathlib.Path,required=True);a=p.parse_args();Run(a.output.resolve(),a.go).main()
