#!/usr/bin/env python3
"""HIT-87 reproduction: immutable sources, synthetic local process, no services."""
import argparse, hashlib, io, json, os, pathlib, platform, statistics, subprocess, tarfile, tempfile, time
ROOT=pathlib.Path(__file__).resolve().parents[3]
BASELINE='8acc83de7daa316719004212fe0d76092aecb8f2'
CANDIDATE='7f688aa26b4e96c19e39f906a5ffaa51a7ce3a12'

def summarize(out):
    before=[json.loads((out/f'baseline-round-{i}.stdout').read_text()) for i in range(1,6)]
    after=[json.loads((out/f'optimized-round-{i}.stdout').read_text()) for i in range(1,6)]
    assert len({x['corpus_sha256'] for x in before+after})==1, 'different outcomes or corpus'
    assert all(x['outcomes']==before[0]['outcomes'] for x in before+after)
    rows=[]
    for index,case in enumerate(before[0]['samples']):
        row={'name':case['name'],'accepted':case['accepted'],'bytes':case['bytes'],'metrics':{}}
        for key in ('bytes_per_event','allocs_per_event','p50_ns','p95_ns','events_per_second'):
            pairs=[[r[index][key] for r in [x['samples'] for x in group]] for group in [before,after]]
            stats=[dict(median=statistics.median(x),min=min(x),max=max(x)) for x in pairs]
            row['metrics'][key]={'baseline':stats[0],'optimized':stats[1],'median_change_pct':100*(stats[1]['median']/stats[0]['median']-1)}
        rows.append(row)
    result={'corpus_sha256':before[0]['corpus_sha256'],'cases':len(rows),'rounds':5,'events_per_case_per_phase':before[0]['samples'][0]['n'],'all_outcomes_identical':True,'rows':rows}
    (out/'comparison.json').write_text(json.dumps(result,indent=2)+'\n')
    return result

def main():
    p=argparse.ArgumentParser();p.add_argument('--go',required=True);p.add_argument('--baseline',default=BASELINE);p.add_argument('--candidate',default=CANDIDATE);p.add_argument('--output',type=pathlib.Path,required=True);a=p.parse_args();out=a.output.resolve();out.mkdir(parents=True,exist_ok=False)
    env=dict(os.environ,GOTOOLCHAIN='local',GOMAXPROCS='1');commands=[]
    def run(label,cmd,cwd,source=None):
        start=time.time();r=subprocess.run(cmd,cwd=cwd,env=env,capture_output=True,timeout=240)
        for stream in ('stdout','stderr'):(out/(label+'.'+stream)).write_bytes(getattr(r,stream))
        commands.append(dict(label=label,argv=cmd,source_sha=source,started_utc=time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime(start)),seconds=time.time()-start,exit_code=r.returncode))
        (out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n');print(label,r.returncode,flush=True);r.check_returncode();return r.stdout
    assert run('go-version',[a.go,'version'],ROOT).decode().startswith('go version go1.27.2 ')
    run('go-env',[a.go,'env','-json'],ROOT)
    (out/'host.json').write_text(json.dumps(dict(platform=platform.platform(),machine=platform.machine(),logical_cpu=os.cpu_count(),gomaxprocs=1,scope='synthetic local; shared desktop; no CPU pinning, thermal control, deployed SLO or HTTP deadline qualification'),indent=2)+'\n')
    with tempfile.TemporaryDirectory(prefix='hit87-compare-') as temporary:
        work={};pins={}
        for name,ref in [('baseline',a.baseline),('optimized',a.candidate)]:
            sha=subprocess.check_output(['git','rev-parse',ref+'^{commit}'],cwd=ROOT,text=True).strip();pins[name]=sha
            raw=subprocess.check_output(['git','archive',sha,'services/intake'],cwd=ROOT);folder=pathlib.Path(temporary)/name;folder.mkdir()
            with tarfile.open(fileobj=io.BytesIO(raw)) as t:t.extractall(folder,filter='data')
            cwd=folder/'services/intake';work[name]=cwd
            run(name+'-build',[a.go,'build','-o',str(out/name),'./cmd/webhookbench'],cwd,sha)
            for check,args in [('race',['test','-race','-count=1','-json','./...']),('vet',['vet','./...']),('modules',['mod','verify'])]:run(name+'-'+check,[a.go]+args,cwd,sha)
        (out/'versions.json').write_text(json.dumps(pins,indent=2)+'\n')
        for i in range(1,6):
            for name in (['baseline','optimized'] if i%2 else ['optimized','baseline']):run(f'{name}-round-{i}',[str(out/name),'-n','200'],work[name],pins[name])
        for name in work:
            run(name+'-profile',[str(out/name),'-profile',str(out/name),'-duration','5s'],work[name],pins[name])
            for kind,args in [('cpu',[]),('alloc',['-alloc_space'])]:run(name+'-'+kind,[a.go,'tool','pprof','-top']+args+[str(out/name),str(out/name)+('.cpu.pprof' if kind=='cpu' else '.heap.pprof')],work[name],pins[name])
    summarize(out)
    # Build executables remain local reproduction artifacts; do not commit binaries.
    print('comparison.json saved; originals retained',flush=True)
if __name__=='__main__':main()
