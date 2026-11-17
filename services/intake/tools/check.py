#!/usr/bin/env python3
"""Repeatable current Intake checks; no server, provider, SQL or academic gate."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import time

parser = argparse.ArgumentParser()
parser.add_argument('--go', default='go')
parser.add_argument('--output', type=Path)
a = parser.parse_args()
module = Path(__file__).resolve().parents[1]
root = module.parents[1]
out = (a.output or module / 'evidence/HIT-74').resolve()
out.mkdir(parents=True, exist_ok=True)
env = dict(os.environ, GOTOOLCHAIN='local')
def capture(cmd, cwd=module):
    return subprocess.run(cmd, cwd=cwd, env=env, capture_output=True, text=True)
version = capture([a.go, 'version'])
if version.returncode or not version.stdout.startswith('go version go1.27.2 '):
    raise SystemExit('Go 1.27.2 is required; no silent toolchain substitution')
files = sorted(p for p in module.rglob('*') if p.is_file() and (p.suffix == '.go' or p.name in ('go.mod', 'go.sum', 'normalized_signal.schema.json')))
manifest = {str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest() for p in files}
report = dict(started_utc=datetime.datetime.now(datetime.UTC).isoformat(),
              source_sha=capture(['git', 'rev-parse', 'HEAD'], root).stdout.strip(),
              dirty=capture(['git', 'status', '--porcelain'], root).stdout.splitlines(),
              go=version.stdout.strip(), host=platform.platform(), source_sha256=manifest,
              scope='synthetic local module only; no durable receipt, HTTP ACK, external service or independent review', commands=[])
commands = [('race', [a.go, 'test', '-race', '-count=1', '-json', './...']),
            ('vet', [a.go, 'vet', './...']), ('build', [a.go, 'build', './...']),
            ('modules', [a.go, 'mod', 'verify'])]
failed = False
for name, cmd in commands:
    started = time.monotonic()
    r = capture(cmd)
    (out / (name + '.stdout')).write_text(r.stdout)
    (out / (name + '.stderr')).write_text(r.stderr)
    report['commands'].append(dict(name=name, argv=cmd, cwd=str(module), exit_code=r.returncode, seconds=round(time.monotonic() - started, 3)))
    failed |= r.returncode != 0
    print(name, 'exit', r.returncode, flush=True)
report['finished_utc'] = datetime.datetime.now(datetime.UTC).isoformat()
(out / 'commands.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
raise SystemExit(1 if failed else 0)
