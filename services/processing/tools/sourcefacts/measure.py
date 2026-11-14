"""HIT-80 reproducible local Decode experiment. Run from services/processing.

Usage: python3 tools/sourcefacts/measure.py --out /absolute/output --label baseline
Requires pinned go on PATH; outputs are new files and existing directories refused.
Profiles are diagnostics, never mixed with unprofiled timing samples.
"""
import argparse
import datetime
import hashlib
import json
import os
import pathlib
import platform
import subprocess
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--out', required=True)
parser.add_argument('--label', required=True)
args = parser.parse_args()
out = pathlib.Path(args.out).resolve()
out.mkdir(parents=True, exist_ok=False)
cwd = pathlib.Path.cwd()
env = os.environ.copy()
env.update(GOTOOLCHAIN='local', GOMAXPROCS='1', CGO_ENABLED='1')
commands = []


def run(name, argv, extra=None):
    started = datetime.datetime.now(datetime.timezone.utc).isoformat()
    at = time.monotonic()
    with (out / (name + '.stdout')).open('wb') as stdout, (out / (name + '.stderr')).open('wb') as stderr:
        result = subprocess.run(argv, cwd=cwd, env=env | (extra or {}), stdout=stdout, stderr=stderr)
    commands.append(dict(name=name, argv=argv, cwd=str(cwd), env={k: (env | (extra or {}))[k] for k in ['GOTOOLCHAIN', 'GOMAXPROCS', 'CGO_ENABLED', 'PATH'] + list(extra or {})},
                         started_at=started, seconds=time.monotonic()-at, exit_code=result.returncode,
                         stdout=name+'.stdout', stderr=name+'.stderr'))
    (out / 'commands.json').write_text(json.dumps(commands, indent=2)+'\n')
    if result.returncode:
        raise SystemExit(f'{name} failed: {result.returncode}; see {out}')


def info(argv):
    return subprocess.check_output(argv, cwd=cwd, env=env, text=True).strip()


version = info(['go', 'version'])
assert 'go1.27.2 ' in version, version
metadata = dict(label=args.label, go=version, go_env=json.loads(info(['go','env','-json','GOOS','GOARCH','GOVERSION','CGO_ENABLED','GOFLAGS','GOEXPERIMENT'])),
                platform=platform.platform(), uname=list(platform.uname()), source_sha=info(['git','rev-parse','HEAD']),
                tracked_status=info(['git','status','--porcelain','--untracked-files=no']),
                cpu=info(['sysctl','-n','machdep.cpu.brand_string']) if platform.system()=='Darwin' else platform.processor(),
                logical_cpu_count=os.cpu_count(), note='Local full Decode microbenchmark; no end-to-end or provider integration claim. One OS process, GOMAXPROCS=1, no race/profile instrumentation in timing run. No samples discarded.')
assert metadata['tracked_status']=='', 'commit the harness/code before measurement'
(out/'environment.json').write_text(json.dumps(metadata,indent=2)+'\n')
pkg='./internal/adapters/intake/sourcefacts'
run('corpus', ['go','test','-count=1','-v','-run','^TestDecodeDifferentialCorpus$',pkg], {'HIT80_CORPUS_OUTPUT':str(out/'corpus.json')})
run('bench', ['go','test','-run','^$','-bench','^BenchmarkDecode$','-benchmem','-count=10','-benchtime=100ms','-cpu=1',pkg])
run('cpu-profile', ['go','test','-run','^$','-bench','^BenchmarkDecode/valid$','-benchtime=3s','-count=1','-cpu=1','-cpuprofile',str(out/'cpu.pprof'),'-memprofile',str(out/'alloc.pprof'),'-o',str(out/'sourcefacts.test'),pkg])
run('cpu-top', ['go','tool','pprof','-top','-nodecount=40',str(out/'sourcefacts.test'),str(out/'cpu.pprof')])
run('cpu-cumulative', ['go','tool','pprof','-top','-cum','-nodecount=40',str(out/'sourcefacts.test'),str(out/'cpu.pprof')])
run('alloc-top', ['go','tool','pprof','-top','-alloc_space','-nodecount=40',str(out/'sourcefacts.test'),str(out/'alloc.pprof')])
run('alloc-cumulative', ['go','tool','pprof','-top','-cum','-alloc_space','-nodecount=40',str(out/'sourcefacts.test'),str(out/'alloc.pprof')])
run('race', ['go','test','-race','-count=1','-v','./...'])
run('vet', ['go','vet','./...'])
digests={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(out.iterdir()) if p.is_file()}
(out/'digests.json').write_text(json.dumps(digests,indent=2)+'\n')
print(json.dumps({'label':args.label,'source_sha':metadata['source_sha'],'commands':len(commands),'result':'PASS','out':str(out)}))
