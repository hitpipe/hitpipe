import sys,os,json
sys.path.insert(0,'/tmp');from hitpipe_dk_review_runner import *
go='/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin';env={'PATH':go+os.pathsep+os.environ['PATH'],'GOTOOLCHAIN':'local','GOMAXPROCS':'2','GOFLAGS':'-p=1','CGO_ENABLED':'1'}
(E/'check-env.json').write_text(json.dumps(env,indent=2)+'\n')
x,out,_=run('pinned-go-version',['go','version'],env=env);assert x['exit']==0 and b'go1.27.2' in out
commands=[('verify-package',['python3','docs/labs/pr12/dk/reports/verify_package.py'],C,{}),('validate',['python3','tools/pr12/workspace.py','validate'],C,{}),('wrapper-tests',['python3','tools/pr12/test_workspace.py'],C,{}),('iam-check-race',['python3','tools/pr12/workspace.py','check','iam','--require'],C,{'PR12_RACE':'1'}),('iam-smoke',['python3','tools/pr12/workspace.py','smoke','iam','--require'],C,{}),('iam-inspect',['python3','tools/pr12/workspace.py','inspect','iam','--mode','release','--require'],C,{}),('adapter-contract',['go','test','./internal/adapters/fixtures/sessionlookup','-run','TestSameConsumerContractWithBothAdapters','-v','-count=1'],C/'services/iam',{}),('imports',['go','list','-json','./...'],C/'services/iam',{}),('godoc-domain',['go','doc','./internal/domain/sessionpolicy'],C/'services/iam',{}),('godoc-application',['go','doc','./internal/application/authenticatesession'],C/'services/iam',{}),('cross-coverage',['go','test','-count=1','-coverpkg=./...','-coverprofile='+str(E/'iam-cross.cover'),'./...'],C/'services/iam',{})]
for name,argv,cwd,extra in commands:
 run(name,argv,cwd=cwd,env={**env,**extra})
# Mutation input must remain clean; outputs stay outside source checkout.
run('mutation-check',['python3','services/iam/tools/mutation_check.py','--output',str(E/'mutations')],env=env,timeout=2400)
run('source-clean-after-checks',['git','status','--porcelain=v1'],env=env)
print('Required checks finished',flush=True)
