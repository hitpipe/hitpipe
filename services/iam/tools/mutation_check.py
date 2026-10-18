#!/usr/bin/env python3
"""HIT-21: compile and test isolated behavioral mutants, always restoring source.

This is an experiment, not production code generation. It requires a committed
clean checkout and exact Go from the wrapper. Each mutant is applied separately
in a disposable Git worktree/temporary branch. Logs and digests are retained.
"""
import argparse
from datetime import datetime, timezone
import difflib
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import uuid

ROOT = Path(__file__).resolve().parents[3]
DOMAIN = 'services/iam/internal/domain/sessionpolicy/policy.go'
APP = 'services/iam/internal/application/authenticatesession/service.go'
MUTATIONS = [
    ('M01', DOMAIN, 'if !now.Before(idle) {', 'if now.After(idle) {', 'DK-RULE-02: idle equality must expire'),
    ('M02', DOMAIN, 'if !now.Before(absolute) {', 'if now.After(absolute) {', 'DK-RULE-02: absolute equality and precedence'),
    ('M03', DOMAIN, 'if snapshot.Revoked {', 'if false {', 'DK-FR-01: revoked cannot be valid'),
    ('M04', DOMAIN, 'return deny(ReasonUnknown)', 'return Decision{State: StateValid, Reason: ReasonValid}', 'DK-FR-01: unknown must fail closed'),
    ('M05', DOMAIN, 'absolute.Sub(created) > MaxAbsoluteLifetime', 'absolute.Sub(created) > MaxAbsoluteLifetime*2', 'DK-RULE-01: absolute lifetime cap'),
    ('M06', APP, 'return unavailable("lookup_unavailable")', 'return Result{State: "valid", Reason: "session_valid"}', 'DK-FR-02: lookup error must not allow'),
    ('M07', DOMAIN, 'return d.State == StateValid && d.Reason == ReasonValid', 'return d.State == StateValid', 'DK-RULE-02: IsValid requires complete positive decision'),
]

def stamp(): return datetime.now(timezone.utc).isoformat()
def sha(data): return hashlib.sha256(data).hexdigest()

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', default=str(ROOT / '.pr12-build/iam/mutations'))
    parser.add_argument('--only', nargs='+', choices=[m[0] for m in MUTATIONS])
    args = parser.parse_args()
    out = Path(args.output).resolve()
    out.mkdir(parents=True, exist_ok=True)
    def git(*argv): return subprocess.check_output(['git', *argv], cwd=ROOT, text=True).strip()
    if git('status', '--porcelain'):
        raise SystemExit('Commit source before experiment; dirty input is not reproducible.')
    source = git('rev-parse', 'HEAD')
    record = {'source_sha':source, 'started_at':stamp(), 'input_dirty':False,
              'go_version':subprocess.check_output(['go','version'], text=True).strip(),
              'environment':{k:os.environ.get(k,'') for k in ('GOTOOLCHAIN','GOMAXPROCS','GOFLAGS','PR12_RACE')},
              'mutations':[], 'scope':'Synthetic IAM slice; not real authority, independent human review or acceptance.'}
    branch = 'temp/pr12-dk-mutations-' + uuid.uuid4().hex[:12]
    record['temporary_branch'] = branch
    def run(label, command, cwd):
        start=stamp()
        result=subprocess.run(command,cwd=cwd,text=True,capture_output=True,timeout=240)
        body=result.stdout+result.stderr
        log=out/(label+'.log');log.write_text(body)
        checks=[]
        for line in body.splitlines():
            try: event=json.loads(line)
            except (ValueError, TypeError): continue
            if event.get('Action')=='fail' and event.get('Test'):checks.append(event['Test'])
        return {'command':command,'cwd':str(cwd),'started_at':start,'completed_at':stamp(),
                'exit_code':result.returncode,'log':log.name,'sha256':sha(log.read_bytes()),'failed_tests':sorted(set(checks))}
    check=['python3','tools/pr12/workspace.py','check','iam','--require']
    with tempfile.TemporaryDirectory(prefix='dk-mutations-',dir=ROOT/'.pr12-build') as tmp:
        tree=Path(tmp)/'checkout'
        subprocess.run(['git','worktree','add','-b',branch,str(tree),source],cwd=ROOT,check=True,capture_output=True)
        try:
            record['baseline']=run('baseline',check,tree)
            if record['baseline']['exit_code']!=0:raise RuntimeError('Baseline not green')
            for identity,path,old,new,rule in MUTATIONS:
                if args.only and identity not in args.only:continue
                target=tree/path;original=target.read_text()
                if original.count(old)!=1:raise RuntimeError('Mutation anchor is not unique: '+identity)
                changed=original.replace(old,new,1)
                patch=''.join(difflib.unified_diff(original.splitlines(True),changed.splitlines(True),fromfile=path,tofile=path))
                (out/(identity+'.patch')).write_text(patch)
                item={'id':identity,'rule':rule,'file':path,'before_sha256':sha(original.encode()),
                      'mutated_sha256':sha(changed.encode()),'patch':identity+'.patch','patch_sha256':sha(patch.encode())}
                try:
                    target.write_text(changed)
                    item['build']=run(identity+'-build',['go','build','./...'],tree/'services/iam')
                    if item['build']['exit_code']!=0:
                        item['result']='INVALID_NOT_COMPILED'
                    else:
                        # This is the same command as the ordinary CI gate.
                        item['test']=run(identity+'-check',check,tree)
                        item['result']='KILLED' if item['test']['exit_code']!=0 and item['test']['failed_tests'] else 'SURVIVED' if item['test']['exit_code']==0 else 'INVALID_ENVIRONMENT'
                finally:
                    target.write_text(original)
                    item['restored_sha256']=sha(target.read_bytes())
                    item['restored']=target.read_text()==original
                record['mutations'].append(item)
                print(identity,item['result'],flush=True)
            record['final_clean']=run('final-clean',check,tree)
            record['restored_worktree_clean']=not subprocess.check_output(['git','status','--porcelain'],cwd=tree,text=True).strip()
        finally:
            subprocess.run(['git','worktree','remove','--force',str(tree)],cwd=ROOT,check=True,capture_output=True)
            subprocess.run(['git','branch','-D',branch],cwd=ROOT,check=True,capture_output=True)
            record['completed_at']=stamp()
            (out/'protocol.json').write_text(json.dumps(record,ensure_ascii=False,indent=2)+'\n')
    if record['final_clean']['exit_code']!=0 or not record['restored_worktree_clean'] or any(m['result']!='KILLED' or not m['restored'] for m in record['mutations']):
        raise SystemExit(1)
    print('All selected compiling behavioral mutants killed; restored final gate PASS.')

if __name__=='__main__':main()
