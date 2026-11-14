#!/usr/bin/env python3
"""Qualify HIT-79 against a new owned PostgreSQL18.7; no existing DSN/DB.
Reuses HIT-76 migration/RLS and HIT-78 admission tests, with actual owner files.
"""
import argparse
import hashlib
import json
import pathlib
import uuid
import verify_admission_storage as admission

class Run(admission.Run):
 def __init__(self,output,go):
  super().__init__(output,go)
  self.name='hit79-'+uuid.uuid4().hex
  self.script_sha256=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
 def command(self,label,args,*a,**kw):
  args=['hitpipe.task=HIT-79' if x in ('hitpipe.task=HIT-76','hitpipe.task=HIT-78') else x for x in args]
  return super().command(label,args,*a,**kw)
 def main(self):
  super().main()
  p=self.output/'summary.json';s=json.loads(p.read_text());s['task']='HIT-79'
  s['scope']='HIT-79 local decision/content/outbox; HIT-76 migration/RLS/CAS and HIT-78 admission regressions. PostgreSQL18.7 Linux plus host Go1.27.2 race/vet, separate CGO=0 Linux DB binary without race. Test owner grants/epochs only; controlled HTTPS Intake fixture, no runtime IAM/Intake/Delivery or provider effect.'
  p.write_text(json.dumps(s,indent=2)+'\n')
  print(json.dumps(s))

if __name__=='__main__':
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--output',required=True,type=pathlib.Path);p.add_argument('--go',required=True,type=pathlib.Path);a=p.parse_args();Run(a.output.resolve(),a.go).main()
