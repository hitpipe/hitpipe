#!/usr/bin/env python3
"""HIT-99 real private mTLS receiver, owner read projection and PG18.7 tests.
Controlled authority/Delivery only; no provider access or production wiring.
"""
import argparse,hashlib,json,os,pathlib,uuid
import verify_case_reads
class Run(verify_case_reads.Run):
 def __init__(self,output,go,oas):
  super().__init__(output,go,oas);self.name='hit99-'+uuid.uuid4().hex;self.script_sha256=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
 def command(self,label,args,*a,**kw):
  args=['hitpipe.task=HIT-99' if x in ['hitpipe.task=HIT-76','hitpipe.task=HIT-78','hitpipe.task=HIT-79','hitpipe.task=HIT-95'] else x for x in args]
  if label=='postgres-storage-tests':args[2:2]=['-e','HIT99_CAPTURE_FILE=/tmp/hit99-captures.jsonl'];args=['-test.timeout=180s' if x=='-test.timeout=60s' else x for x in args]
  return super().command(label,args,*a,**kw)
 def checks(self):
  super().checks();self.command('go-build',[self.go,'-C',str(pathlib.Path(__file__).resolve().parents[2]),'build','./...']);r=self.command('observation-api-captures',['docker','exec',self.container,'cat','/tmp/hit99-captures.jsonl']);(self.output/'observation-captures.jsonl').write_bytes(r.stdout)
 def main(self):
  super().main();p=self.output/'summary.json';s=json.loads(p.read_text());s.update(task='HIT-99',contract_revision=1,scope='Local Processing applyTaskObservation via actual private HTTPS TLS1.3 mTLS, real owned PG18.7 app-role RLS, durable history, HIT95 existing read APIs. Controlled authority/current epoch/shared actual-start and Delivery observations only; no live IAM/Delivery/ClickUp/provider proof retrieval. Host Go1.27.2 darwin/arm64 race/vet/build plus separate linux/arm64 CGO0 actual DB suite, not Linux race.');p.write_text(json.dumps(s,indent=2)+'\n');contexts=[];commands=[];keys=['cwd','environment','source_sha','tree_status_at_start','runner_sha256']
  for row in self.records:
   context={k:row[k] for k in keys if k in row}
   if context not in contexts:contexts.append(context)
   commands.append({'context':contexts.index(context),**{k:v for k,v in row.items() if k not in keys}})
  assert [{**contexts[c['context']],**{k:v for k,v in c.items() if k!='context'}} for c in commands]==self.records
  (self.output/'review-commands.json').write_text(json.dumps({'format':'Exact commands; merge referenced context into each command.','original_sha256':hashlib.sha256((self.output/'commands.json').read_bytes()).hexdigest(),'lossless_round_trip_verified':True,'contexts':contexts,'commands':commands},indent=2)+'\n');print(json.dumps(s))
if __name__=='__main__':
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--output',type=pathlib.Path,required=True);p.add_argument('--go',type=pathlib.Path,required=True);p.add_argument('--oas',type=pathlib.Path,required=True);a=p.parse_args();Run(a.output.resolve(),a.go,a.oas).main()
