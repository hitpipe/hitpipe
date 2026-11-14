#!/usr/bin/env python3
"""HIT-78 private storage qualification, reusing isolated HIT-76 PG lifecycle.
No existing database/DSN is accepted. Not an activation-boundary qualification.
"""
import argparse
import hashlib
import json
import pathlib
import tempfile
import uuid
import verify

class Run(verify.Run):
 def __init__(self, output, go):
  super().__init__(output)
  self.name='hit78-'+uuid.uuid4().hex
  self.db='hit78'
  self.go=str(go.resolve())
  self.script_sha256=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
 def command(self,label,args,*a,**kw):
  args=['hitpipe.task=HIT-78' if x=='hitpipe.task=HIT-76' else x for x in args]
  return super().command(label,args,*a,**kw)
 def checks(self):
  # Preserve the full existing migration/RLS/CAS/rollback qualification.
  super().checks()
  self.command('go-version',[self.go,'version'])
  module=str(verify.MODULE)
  self.command('go-race',[self.go,'-C',module,'test','-race','-count=1','-v','./...'])
  self.command('go-vet',[self.go,'-C',module,'vet','./...'])
  arch=self.command('container-arch',['docker','exec',self.container,'uname','-m']).stdout.decode().strip()
  target={'aarch64':'arm64','x86_64':'amd64'}[arch]
  with tempfile.TemporaryDirectory(prefix='hit78-test-') as tmp:
   binary=pathlib.Path(tmp)/'storage.test'
   self.command('compile-linux-test',['env','GOOS=linux','GOARCH='+target,'CGO_ENABLED=0',self.go,'-C',module,'test','-c','-o',str(binary),'./internal/adapters/postgres'])
   (self.output/'test-binary.json').write_text(json.dumps({'sha256':hashlib.sha256(binary.read_bytes()).hexdigest(),'GOOS':'linux','GOARCH':target,'CGO_ENABLED':'0'},indent=2)+'\n')
   self.command('copy-linux-test',['docker','cp',str(binary),self.container+':/tmp/storage.test'])
   self.command('postgres-storage-tests',['docker','exec','-e','HIT78_ISOLATED_PG=1',self.container,'/tmp/storage.test','-test.v','-test.count=1','-test.timeout=60s'])
 def main(self):
  super().main()
  p=self.output/'summary.json';s=json.loads(p.read_text())
  s['scope']='HIT-76 migration regression plus HIT-78 PRIVATE storage on real isolated PostgreSQL; host Go race/vet. Linux test binary CGO=0, no race there. No public admission, authority/binding or activation-boundary qualification.'
  s['task']='HIT-78'
  p.write_text(json.dumps(s,indent=2)+'\n')

if __name__=='__main__':
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--output',required=True,type=pathlib.Path)
 p.add_argument('--go',required=True,type=pathlib.Path)
 a=p.parse_args();Run(a.output.resolve(),a.go).main()
