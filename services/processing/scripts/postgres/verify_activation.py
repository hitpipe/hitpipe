#!/usr/bin/env python3
"""HIT-102 explicit activation on owned PG18.7/files/private HTTPS with controlled owners."""
import argparse,hashlib,json,pathlib,uuid
import verify_drafts
class Run(verify_drafts.Run):
 def __init__(self,output,go):
  super().__init__(output,go);self.name='hit102-'+uuid.uuid4().hex;self.script_sha256=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
 def command(self,label,args,*a,**kw):
  args=['hitpipe.task=HIT-102' if x.startswith('hitpipe.task=HIT-') else x for x in args]
  if label=='postgres-storage-tests':args[2:2]=['-e','HIT102_CAPTURE_FILE=/tmp/hit102-captures.jsonl']
  return super().command(label,args,*a,**kw)
 def checks(self):
  super().checks();r=self.command('activation-api-captures',['docker','exec',self.container,'cat','/tmp/hit102-captures.jsonl']);(self.output/'activation-api-captures.jsonl').write_bytes(r.stdout)
 def main(self):
  super().main();p=self.output/'summary.json';s=json.loads(p.read_text());s.update(task='HIT-102',scope='Explicit activation using existing drafts/content/commands; private HTTPS TLS1.3 mTLS, actual PG18.7 app-role/RLS/atomicity/SINGLE/replay. Host Go1.27.2 race/vet/build and separate LinuxCGO0 DB binary, not Linux race. Controlled session/authority/owner bindings; no live IAM/BFF/providers or product deployment qualification.',project_os_handoff_ready='NOT_EVALUATED_BY_RUNNER');p.write_text(json.dumps(s,indent=2)+'\n');print(json.dumps(s))
if __name__=='__main__':
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--output',required=True,type=pathlib.Path);p.add_argument('--go',required=True,type=pathlib.Path);a=p.parse_args();Run(a.output.resolve(),a.go).main()
