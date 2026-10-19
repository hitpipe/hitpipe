"""Reproduce the three recorded integer-boundary failures using test overlays.

The harness succeeding means the original defects were reproduced, not that the
product passed acceptance. Changed author source requires adapted regressions.
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
            for p in sorted((repo / 'services/intake').rglob('*')) if p.is_file()}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo', required=True, type=Path)
    parser.add_argument('--out', required=True, type=Path)
    args = parser.parse_args()
    here = Path(__file__).resolve().parent
    repo, out = args.repo.resolve(), args.out.resolve()
    expected = json.loads((here / 'expected-input-hashes.json').read_text())
    before = hashes(repo)
    if before != expected:
        raise SystemExit('Input differs from reviewed original; use a separate worktree at the recorded SHA.')
    if not shutil.which('go'):
        raise SystemExit('Add pinned Go 1.27.2 bin directory to PATH first.')
    env = os.environ.copy()
    env['GOTOOLCHAIN'] = 'local'
    version = subprocess.check_output(['go', 'version'], env=env, text=True).strip()
    if ' go1.27.2 ' not in version:
        raise SystemExit('Use exact Go 1.27.2.')
    out.mkdir(parents=True, exist_ok=True)
    original = repo / 'services/intake/internal/domain/stripesignature/verifier_test.go'
    overlay = out / 'overlay.json'
    overlay.write_text(json.dumps({'Replace': {str(original): str(here / 'probes/domain.go.txt')}}, indent=2) + '\n')
    argv = ['go', 'test', '-count=1', '-json', '-overlay=' + str(overlay),
            '-run', '^TestReview', './internal/domain/stripesignature']
    started = datetime.datetime.now(datetime.timezone.utc).isoformat()
    result = subprocess.run(argv, cwd=repo / 'services/intake', env=env,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=180)
    (out / 'probes.log').write_bytes(result.stdout)
    events = [json.loads(line) for line in result.stdout.decode(errors='replace').splitlines()
              if line.startswith('{')]
    failed = [e['Test'] for e in events if e.get('Action') == 'fail' and e.get('Test')]
    passed = [e['Test'] for e in events if e.get('Action') == 'pass' and e.get('Test')]
    after = hashes(repo)
    record = {'reviewed_source_sha': 'b23001983b511999d815fbc7f77063e763516bc2',
              'checkout_head': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=repo, text=True).strip(),
              'started_at': started, 'argv': argv, 'toolchain': version,
              'exit_code': result.returncode, 'failed': failed, 'passed': passed,
              'source_unchanged': before == after,
              'log_sha256': hashlib.sha256(result.stdout).hexdigest(),
              'meaning': 'Original integer-boundary defects reproduced; not a positive acceptance.'}
    (out / 'result.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(record, ensure_ascii=False, indent=2))
    expected_failed = {'TestReviewTimestampParserRejectsOverflow',
                       'TestReviewOverflowHeaderReportsMalformed',
                       'TestReviewAbsoluteDistanceCannotWrap'}
    if result.returncode != 1 or set(failed) != expected_failed or 'TestReviewIndependentHMACVectors' not in passed or before != after:
        raise SystemExit('Result differs from recorded reproduction; inspect log before claiming success.')


if __name__ == '__main__':
    main()
