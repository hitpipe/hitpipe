#!/usr/bin/env python3
"""HIT-104 private Intake HTTPS/mTLS admission on a new owned PG18.7.
Reuses HIT76/78/79/95/99 qualification. No existing DB or provider access.
"""
import argparse,hashlib,json,pathlib,uuid
import verify_observations
class Run(verify_observations.Run):
 def __init__(self,output,go,oas):
  super().__init__(output,go,oas)
  self.name='hit104-'+uuid.uuid4().hex
  self.script_sha256=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
 def command(self,label,args,*a,**kw):
  args=['hitpipe.task=HIT-104' if x.startswith('hitpipe.task=HIT-') else x for x in args]
  if label=='postgres-storage-tests':args[2:2]=['-e','HIT104_CAPTURE_FILE=/tmp/hit104-captures.jsonl']
  return super().command(label,args,*a,**kw)
 def checks(self):
  super().checks()
  r=self.command('intake-http-captures',['docker','exec',self.container,'cat','/tmp/hit104-captures.jsonl'])
  (self.output/'intake-captures.jsonl').write_bytes(r.stdout)
  self.command('python-version',['python3','--version'])
  self.command('exact-intake-oas-validation',['python3',str(pathlib.Path(__file__).with_name('verify_intake_schema.py')),'--oas',str(self.oas),'--captures',str(self.output/'intake-captures.jsonl')])
 def main(self):
  super().main()
  p=self.output/'summary.json';s=json.loads(p.read_text());s.update(task='HIT-104',contract_revision=1,scope='Actual receiveNormalizedSignal over existing Processing TLS1.3/mTLS listener with Intake-only peer, controlled trusted context/current authority/shared actual-start, actual PG18.7 admission/inbox/case/safety/content and original HIT95 read handler; all predecessor tests retained. Host Go1.27.2 darwin/arm64 race/vet/build; separate Linux CGO0 actual DB tests, not Linux race. No live IAM/Intake sender/provider, scheduler or deployment qualification.');p.write_text(json.dumps(s,indent=2)+'\n');print(json.dumps(s))
if __name__=='__main__':
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--output',type=pathlib.Path,required=True);p.add_argument('--go',type=pathlib.Path,required=True);p.add_argument('--oas',type=pathlib.Path,required=True);a=p.parse_args();Run(a.output.resolve(),a.go,a.oas).main()
