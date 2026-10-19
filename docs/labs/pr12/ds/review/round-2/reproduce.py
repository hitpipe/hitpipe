#!/usr/bin/env python3
"""Run repeat-review probes without editing production sources or original tests."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--repo', type=Path, default=Path.cwd())
    ap.add_argument('--out', type=Path)
    ap.add_argument('--allow-changed-source', action='store_true', help='Explicit correction recheck: record new source hashes rather than claiming the original source.')
    args = ap.parse_args()
    package = Path(__file__).resolve().parent
    repo = args.repo.resolve()
    service = repo / 'services/delivery'
    if not (service / 'go.mod').is_file():
        ap.error('--repo must point at the hitpipe repository root')
    expected = json.loads((package / 'expected-input-hashes.json').read_text())
    before = {p: hashlib.sha256((repo / p).read_bytes()).hexdigest() for p in expected}
    changed = [p for p in expected if expected[p] != before[p]]
    if changed and not args.allow_changed_source:
        ap.error('Source differs from reviewed9ebe637; use --allow-changed-source only for an explicitly recorded correction recheck: ' + ', '.join(changed))
    out = args.out.resolve() if args.out else Path(tempfile.mkdtemp(prefix='hitpipe-ds-repeat-review-'))
    out.mkdir(parents=True, exist_ok=True)
    specs = [
        ('internal/domain/readback/readback_test.go', 'readback.go.txt'),
        ('internal/application/verifytarget/verifytarget_test.go', 'verifytarget.go.txt'),
        ('internal/adapters/fixtures/targetreader/file_reader_test.go', 'targetreader.go.txt'),
    ]
    replace = {}
    for rel, name in specs:
        original = service / rel
        content = original.read_text(encoding='utf-8')
        if name == 'targetreader.go.txt' and '"strings"' not in content:
            content = content.replace('import (', 'import (\n "strings"', 1)
        target = out / name.replace('.txt', '')
        target.write_text(content + (package / 'probes' / name).read_text(encoding='utf-8'), encoding='utf-8')
        replace[str(original)] = str(target)
    overlay = out / 'overlay.json'
    overlay.write_text(json.dumps({'Replace': replace}, indent=2) + '\n')
    env = os.environ.copy()
    env.setdefault('GOTOOLCHAIN', 'local')
    command = ['go', 'test', '-count=1', '-json', '-overlay=' + str(overlay), '-run', '^TestRepeat', './internal/...']
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=repo, text=True).strip()
    started = datetime.datetime.now(datetime.timezone.utc).isoformat()
    with (out / 'probes.log').open('w', encoding='utf-8') as log:
        result = subprocess.run(command, cwd=service, env=env, stdout=log, stderr=subprocess.STDOUT, timeout=180)
    events = []
    for line in (out / 'probes.log').read_text(encoding='utf-8').splitlines():
        try:
            events.append(json.loads(line))
        except ValueError:
            pass
    failed = [e['Test'] for e in events if e.get('Action') == 'fail' and e.get('Test')]
    leaves = [n for n in failed if not any(other.startswith(n + '/') for other in failed)]
    after = {p: hashlib.sha256((repo / p).read_bytes()).hexdigest() for p in expected}
    record = {'recorded_source': head, 'original_reviewed_source': '9ebe6373467ded83f32939a849bac31905eed99f', 'changed_inputs': changed, 'started_at': started, 'argv': command, 'cwd': str(service), 'exit': result.returncode, 'compiled': bool(events) and not any(e.get('Action') == 'build-fail' for e in events), 'failed_cases': leaves, 'passed': [e['Test'] for e in events if e.get('Action') == 'pass' and e.get('Test')], 'original_inputs_and_tests_unchanged': before == after, 'out': str(out)}
    (out / 'result.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(record, ensure_ascii=False, indent=2))
    return result.returncode if before == after else 2


if __name__ == '__main__':
    sys.exit(main())
