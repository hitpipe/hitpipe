#!/usr/bin/env python3
"""Run one compiled behavioural mutant at a time in a disposable Git worktree."""
import argparse,datetime,hashlib,json,os,pathlib,subprocess,tempfile
ROOT=pathlib.Path(__file__).resolve().parents[4]
REL=pathlib.Path('services/processing/internal/domain/eligibility/eligibility.go')
MUTANTS=[
 ('M01','HP-RULE-006/refund','*f.AmountRefunded == 0','*f.AmountRefunded >= 0'),
 ('M02','HP-BP-002/T/004/capture','*f.AmountCaptured == *f.AmountTotal','*f.AmountCaptured >= 0'),
 ('M03','HP-RULE-006/quantity','*f.Quantity == 1','*f.Quantity >= 1'),
 ('M04','HP-RULE-006/activation','!f.SessionCreatedAt.Before(p.ActivationAt)','f.SessionCreatedAt.After(p.ActivationAt)'),
 ('M05','HP-RULE-006/account','f.AccountID != p.AccountID','false'),
 ('M06','HP-RULE-006/dispute','!*f.Disputed, "DISPUTE"','*f.Disputed, "DISPUTE"'),
]
def digest(b):return hashlib.sha256(b).hexdigest()
def run(argv,cwd,folder,label,sha):
 start=datetime.datetime.now(datetime.timezone.utc).isoformat()
 p=subprocess.run(argv,cwd=cwd,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=240)
 path=folder/(label+'.log');path.write_text(p.stdout)
 return {'argv':argv,'cwd':str(cwd),'source_sha':sha,'started_at_utc':start,'finished_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'exit':p.returncode,'log':path.name,'sha256':digest(path.read_bytes())}
def main():
 parser=argparse.ArgumentParser();parser.add_argument('--output',required=True);parser.add_argument('--natural-defects',type=int,default=0);parser.add_argument('--natural-defect-task',action='append',default=[]);args=parser.parse_args()
 if args.natural_defects<0 or (args.natural_defects and not args.natural_defect_task):raise ValueError('natural defect count requires actual task references')
 output=pathlib.Path(args.output).resolve();output.mkdir(parents=True,exist_ok=True)
 sha=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()
 records=[];gate=None
 with tempfile.TemporaryDirectory(prefix='hit29-mutations-') as temp:
  wt=pathlib.Path(temp)/'checkout'
  subprocess.run(['git','worktree','add','--detach',str(wt),sha],cwd=ROOT,check=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  try:
   module=wt/'services/processing';file=wt/REL;original=file.read_text()
   baseline=run(['go','test','-count=1','-v','./...'],module,output,'baseline',sha)
   if baseline['exit']!=0:raise RuntimeError('baseline not green')
   for mid,rule,old,new in MUTANTS:
    if original.count(old)!=1:raise RuntimeError('mutation preimage not unique '+mid)
    try:
     file.write_text(original.replace(old,new))
     patch=subprocess.check_output(['git','diff','--',str(REL)],cwd=wt)
     (output/(mid+'.patch')).write_bytes(patch)
     build=run(['go','build','./...'],module,output,mid+'-build',sha)
     test=run(['go','test','-count=1','-v','./...'],module,output,mid+'-test',sha)
     failures=[line.strip() for line in (output/test['log']).read_text().splitlines() if '--- FAIL:' in line]
     outcome='INVALID' if build['exit'] else ('KILLED' if test['exit'] and failures else 'SURVIVED')
     records.append({'id':mid,'rule':rule,'patch':mid+'.patch','patch_sha256':digest(patch),'build':build,'test':test,'outcome':outcome,'failing_tests':failures,'retest':'NOT_NEEDED' if outcome=='KILLED' else 'REQUIRED','source_file_sha256':digest(file.read_bytes())})
     if mid=='M01':
      gate=run(['python3','tools/pr12/workspace.py','check','processing','--require'],wt,output,'negative-gate',sha)
      if gate['exit']==0:raise RuntimeError('gate accepted refund mutant')
    finally:
     file.write_text(original)
     subprocess.run(['git','diff','--exit-code'],cwd=wt,check=True,stdout=subprocess.PIPE)
   restored=run(['go','test','-count=1','-v','./...'],module,output,'restored',sha)
   if restored['exit']!=0:raise RuntimeError('restoration not green')
   clean=not subprocess.check_output(['git','status','--porcelain'],cwd=wt,text=True).strip()
   summary={'baseline_sha':sha,'tool_versions':{'go':subprocess.check_output(['go','version'],text=True).strip(),'python':subprocess.check_output(['python3','--version'],text=True).strip()},'baseline':baseline,'mutants':records,'seeded':len(records),'compiled':sum(x['build']['exit']==0 for x in records),'detected':sum(x['outcome']=='KILLED' for x in records),'missed':sum(x['outcome']=='SURVIVED' for x in records),'invalid':sum(x['outcome']=='INVALID' for x in records),'natural_defects':args.natural_defects,'natural_defect_task_links':args.natural_defect_task,'natural_count_boundary':'Natural count is supplied from separately recorded genuine failing tests; mutation runner does not discover or estimate natural defects.','negative_gate':gate,'restored':restored,'clean':clean,'hidden_suite':'NOT_PROVIDED_NOT_RUN','limitations':'Synthetic offline domain and consumer only; no human review, provider or full-system correctness claim.'}
   (output/'summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2))
   print(json.dumps({k:summary[k] for k in ['baseline_sha','seeded','compiled','detected','missed','invalid','clean']}))
   if not clean or any(x['outcome']!='KILLED' for x in records):raise RuntimeError('inspect INVALID/SURVIVED protocol; strengthen tests and preserve retest')
  finally:subprocess.run(['git','worktree','remove','--force',str(wt)],cwd=ROOT,check=True)
if __name__=='__main__':main()
