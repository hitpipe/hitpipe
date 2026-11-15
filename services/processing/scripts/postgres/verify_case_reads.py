#!/usr/bin/env python3
"""HIT-95 revision3 read-path/admission qualification on owned PG18.7 + real mTLS.
Local owner API only; no production IAM/BFF/runtime qualification.
"""
import argparse
import hashlib
import json
import pathlib
import os
import uuid
import sys
sys.dont_write_bytecode=True
import verify_preparation as preparation

class Run(preparation.Run):
 def __init__(self,output,go,oas):
  super().__init__(output,go)
  self.name='hit95-'+uuid.uuid4().hex
  self.oas=oas.resolve()
  self.script_sha256=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
 def command(self,label,args,*a,**kw):
  args=['hitpipe.task=HIT-95' if x in ('hitpipe.task=HIT-76','hitpipe.task=HIT-78','hitpipe.task=HIT-79') else x for x in args]
  if label=='postgres-storage-tests':
   args[2:2]=['-e','HIT95_SCHEMA_CAPTURE=/tmp/hit95-schema.json']
  result=super().command(label,args,*a,**kw)
  self.records[-1]['environment']={k:os.environ.get(k,'<unset>') for k in ['PATH','GOTOOLCHAIN','GOMAXPROCS','CGO_ENABLED','GOFLAGS','GOEXPERIMENT','DOCKER_HOST','PYTHONDONTWRITEBYTECODE']}
  self.save()
  return result
 def checks(self):
  self.command('go-env',[self.go,'env','-json','GOOS','GOARCH','GOVERSION','CGO_ENABLED','GOFLAGS','GOEXPERIMENT'])
  super().checks()
  result=self.command('read-api-capture',['docker','exec',self.container,'cat','/tmp/hit95-schema.json'])
  (self.output/'api-capture.json').write_bytes(result.stdout)
  self.command('exact-oas-response-validation',['python3',str(pathlib.Path(__file__).with_name('verify_read_schema.py')),str(self.oas),str(self.output/'api-capture.json')])
 def main(self):
  super().main()
  p=self.output/'summary.json';s=json.loads(p.read_text());s['task']='HIT-95'
  s['scope']='HIT-95 CCR revision3: nonempty listCases/getCase/getCaseHistory, approved label/display policy, real admission durable HISTORY and expiry/minimal reads, controlled TLS1.3 BFF mTLS/fresh IAM and epoch ports, real PG18.7 app-role RLS/scoped readonly reads/content. Host Go1.27.2 race/vet; separate Linux CGO0 DB binary, no Linux race or live IAM/BFF/runtime qualification claim.'
  s['contract_revision']=3
  s['local_verification_complete']=True
  s['project_os_handoff_ready']='NOT_EVALUATED_BY_THIS_RUNNER'
  p.write_text(json.dumps(s,indent=2)+'\n');print(json.dumps(s))

if __name__=='__main__':
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--output',required=True,type=pathlib.Path);p.add_argument('--go',required=True,type=pathlib.Path);p.add_argument('--oas',required=True,type=pathlib.Path);a=p.parse_args();Run(a.output.resolve(),a.go,a.oas).main()
