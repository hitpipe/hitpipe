"""Reproduce the nine review failures against the exact submitted Delivery files.

Uses test overlays only. A nonzero test exit is expected here and is not a passing
product test suite. After an author correction, port the oracles into regression
tests; this script deliberately rejects a changed original input set.
"""
from pathlib import Path
import argparse
import datetime
import hashlib
import json
import os
import shutil
import subprocess


def hashes(repo):
    return {p.relative_to(repo).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
            for p in sorted((repo / 'services/delivery').rglob('*')) if p.is_file()}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo', required=True, type=Path)
    parser.add_argument('--out', required=True, type=Path)
    args = parser.parse_args()
    repo, out = args.repo.resolve(), args.out.resolve()
    here = Path(__file__).resolve().parent
    expected = json.loads((here / 'expected-input-hashes.json').read_text())
    before = hashes(repo)
    if before != expected:
        raise SystemExit('Input differs from reviewed original; use the recorded SHA in a separate worktree.')
    if not shutil.which('go'):
        raise SystemExit('Go is not on PATH; add the pinned Go 1.27.2 bin directory first.')
    go = subprocess.check_output(['go', 'version'], text=True).strip()
    if ' go1.27.2 ' not in go:
        raise SystemExit('Use the exact pinned Go 1.27.2 toolchain.')
    out.mkdir(parents=True, exist_ok=True)
    spec = {
        'internal/domain/readback/readback_test.go': 'domain.go.txt',
        'internal/application/verifytarget/verifytarget_test.go': 'application.go.txt',
        'internal/adapters/fixtures/targetreader/file_reader_test.go': 'adapter.go.txt',
    }
    mapping = {str(repo / 'services/delivery' / relative): str(here / 'probes' / name)
               for relative, name in spec.items()}
    overlay = out / 'overlay.json'
    overlay.write_text(json.dumps({'Replace': mapping}, indent=2) + '\n')
    argv = ['go', 'test', '-count=1', '-json', '-overlay=' + str(overlay),
            '-run', '^TestReview', './internal/...']
    env = os.environ.copy()
    env['GOTOOLCHAIN'] = 'local'
    started = datetime.datetime.now(datetime.timezone.utc).isoformat()
    result = subprocess.run(argv, cwd=repo / 'services/delivery', env=env,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=180)
    log = out / 'contract-probes.log'
    log.write_bytes(result.stdout)
    events = [json.loads(line) for line in result.stdout.decode(errors='replace').splitlines()
              if line.startswith('{')]
    ran = [e['Test'] for e in events if e.get('Action') == 'run' and e.get('Test')]
    failed = [e['Test'] for e in events if e.get('Action') == 'fail' and e.get('Test')]
    after = hashes(repo)
    record = {'reviewed_source_sha': '0a6f2ef3a5cd6db30ff4cef5efbefd185fc47b00',
              'checkout_head': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=repo, text=True).strip(),
              'started_at': started, 'toolchain': go, 'argv': argv, 'cwd': str(repo / 'services/delivery'),
              'exit_code': result.returncode, 'ran': ran, 'failed': failed,
              'source_unchanged': before == after,
              'log_sha256': hashlib.sha256(result.stdout).hexdigest(),
              'meaning': 'Observed original defects; these tests are not a successful acceptance.'}
    (out / 'result.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(record, ensure_ascii=False, indent=2))
    if result.returncode != 1 or len(ran) != 9 or len(failed) != 9 or before != after:
        raise SystemExit('Reproduction did not match recorded result; inspect log, do not claim PASS.')


if __name__ == '__main__':
    main()
