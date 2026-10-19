#!/usr/bin/env python3
"""Manual lab tooling. Missing modules are NOT_IMPLEMENTED, never student PASS."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
CFG = json.loads((ROOT / 'tools/pr12/workspace-config.json').read_text())


def die(message):
    raise SystemExit('ERROR: ' + message)


def execute(args, cwd=ROOT, capture=False, env=None, timeout=180):
    print('+ ' + ' '.join(map(str, args)), flush=True)
    try:
        result = subprocess.run(list(map(str, args)), cwd=cwd, text=True,
                                capture_output=capture, env=env, check=False, timeout=timeout)
    except subprocess.TimeoutExpired:
        die('command exceeded bounded timeout of ' + str(timeout) + ' seconds')
    if result.returncode:
        if capture:
            print(result.stdout)
            print(result.stderr, file=sys.stderr)
        die('command failed with exit ' + str(result.returncode))
    return result.stdout if capture else ''


def directory(component):
    return ROOT / ('services/' + component if CFG['kind'] == 'backend'
                   else 'src/features/case-status')


def implemented(component, required=False):
    folder = directory(component)
    manifest = folder / 'lab-component.json'
    if not manifest.is_file():
        files = [p for p in folder.rglob('*') if p.is_file()] if folder.exists() else []
        if required or files:
            die(f'{component}: module files exist or component is required, but lab-component.json is missing')
        print(json.dumps({'component': component, 'status': 'NOT_IMPLEMENTED',
                          'student_checks': 'NOT_RUN', 'reason': 'No module has been submitted.'}))
        return None
    data = json.loads(manifest.read_text())
    if data.get('schema_version') != 1 or data.get('component') != component:
        die('component manifest identity/version mismatch')
    for key in ('positive_fixture', 'negative_fixture', 'positive_output_contains', 'expected_test_names'):
        if not data.get(key):
            die('component manifest must declare ' + key)
    if not isinstance(data['expected_test_names'], list) or not all(isinstance(x, str) and x.strip() for x in data['expected_test_names']):
        die('expected_test_names must be an explicit array of nonempty names')
    if not isinstance(data['positive_output_contains'], list) or not all(isinstance(x, str) and x.strip() for x in data['positive_output_contains']):
        die('positive_output_contains must be an explicit array of requirement-derived markers')
    if len(set(data['expected_test_names'])) < 3:
        die('declare at least three named normal/error/boundary tests')
    for key in ('positive_fixture', 'negative_fixture'):
        target = (folder / data[key]).resolve()
        if not target.is_relative_to(folder.resolve()) or not target.is_file():
            die('fixture must exist inside component: ' + key)
    return data


def check_versions():
    if CFG['kind'] == 'backend':
        version = execute(['go', 'version'], capture=True)
        if not re.search(r'\bgo' + re.escape(CFG['go_version']) + r'\b', version):
            die('expected exact Go ' + CFG['go_version'] + '; found ' + version.strip())
    else:
        if execute(['node', '--version'], capture=True).strip() != 'v' + CFG['node_version']:
            die('use exact Node ' + CFG['node_version'])
        if execute(['npm', '--version'], capture=True).strip() != CFG['npm_version']:
            die('use exact npm ' + CFG['npm_version'])


def validate():
    docs = ROOT / 'docs/labs/pr12'
    for name in ('AGENT_LAB_WORKFLOW.md', 'MASTER_PROMPT_TEMPLATE.md', 'IMPLEMENTATION_CONVENTION.md', 'context-lock.json'):
        if not (docs / name).is_file():
            die('missing required documentation: ' + name)
    upstream = docs / 'upstream/stack'
    inventory = json.loads((upstream / 'inventory.json').read_text())
    verified = 0
    for record in inventory['files']:
        local = upstream / record['path'].removeprefix('formal/stack/')
        if local.is_file():
            if hashlib.sha256(local.read_bytes()).hexdigest() != record['sha256']:
                die('frozen upstream source drift: ' + record['path'])
            verified += 1
    if verified < 5:
        die('upstream stack context incomplete')
    if CFG['kind'] == 'frontend':
        for name in ('package.json', 'package-lock.json'):
            if (ROOT / 'tooling/pr12/frontend' / name).read_bytes() != (upstream / 'frontend' / name).read_bytes():
                die('active isolated frontend lock differs from frozen source: ' + name)
    for relative in ('AGENTS.md', 'GEMINI.md', '.hermes.md', '.agents/rules/project-os.md'):
        path = ROOT / relative
        if path.exists() and 'MANUAL_LAB_PR12' not in path.read_text():
            die('manual loader route absent: ' + relative)
    execute([sys.executable, ROOT / 'tools/pr12/validate_bundle.py'])
    print(json.dumps({'status': 'BOOTSTRAP_VALIDATION_PASS', 'verified_frozen_files': verified,
                      'student_implementation': 'NOT_ASSESSED', 'human_gates': 'NOT_ASSESSED'}))


def go_imports(text):
    """Read canonical Go import declarations, ignoring comments and ordinary strings.

    This narrow guard is not a Go parser/security sandbox. Dot aliases, raw import
    literals and escaped paths are deliberately rejected; go vet/build remains
    responsible for the full language grammar. Accepting only canonical paths
    prevents unsupported legal syntax from silently bypassing layer checks.
    """
    token_re = re.compile(r'//[^\n]*|/\*.*?\*/|"(?:\\.|[^"\\])*"|`[^`]*`|\'(?:\\.|[^\'\\])*\'|[A-Za-z_][A-Za-z_0-9]*|[^\s]', re.DOTALL)
    tokens = [m.group() for m in token_re.finditer(text)
              if not m.group().startswith(('//', '/*'))]
    imports = []
    index = 0
    def path_at(position):
        if position >= len(tokens):
            die('unsupported or incomplete Go import declaration')
        value = tokens[position]
        if not value.startswith('"') or not value.endswith('"') or '\\' in value:
            die('unsupported Go import syntax: use canonical double-quoted paths without escapes or raw literals')
        path = value[1:-1]
        if not path or not re.fullmatch(r'[A-Za-z0-9._~+/\-]+', path):
            die('unsupported Go import path syntax')
        imports.append(path)
        return position + 1
    def specification(position):
        if position >= len(tokens):
            die('incomplete Go import declaration')
        if tokens[position] == '.':
            die('unsupported Go dot import; use an explicit named package import')
        if not tokens[position].startswith(('"', '`')):
            if not re.fullmatch(r'[A-Za-z_][A-Za-z_0-9]*', tokens[position]):
                die('unsupported Go import alias')
            position += 1
        return path_at(position)
    while index < len(tokens):
        if tokens[index] != 'import':
            index += 1
            continue
        index += 1
        if index < len(tokens) and tokens[index] == '(':
            index += 1
            while index < len(tokens) and tokens[index] != ')':
                if tokens[index] == ';':
                    index += 1
                else:
                    index = specification(index)
            if index >= len(tokens):
                die('unterminated Go import group')
            index += 1
        else:
            index = specification(index)
    return imports


def import_rules(component):
    folder = directory(component)
    errors = []
    module_file = folder / 'go.mod'
    module_match = re.search(r'(?m)^module\s+(\S+)', module_file.read_text()) if module_file.is_file() else None
    module_prefix = module_match.group(1) + '/' if module_match else None
    for path in folder.rglob('*.go'):
        if path.name.endswith('_test.go'):
            continue
        text = path.read_text()
        # Reject unsupported declaration syntax rather than silently skipping it.
        imports = go_imports(text)
        rel = path.relative_to(folder).as_posix()
        for imported in imports:
            if rel.startswith('internal/domain/') and (imported.startswith(('net/', 'database/', 'github.com/', 'go.opentelemetry.io/')) or imported in ('os', 'os/exec') or '/internal/application/' in imported or '/internal/adapters/' in imported or '/internal/transport/' in imported):
                errors.append(f'{rel}: forbidden domain import {imported}')
            if rel.startswith('internal/application/') and ('/internal/adapters/' in imported or '/internal/transport/' in imported or imported.startswith(('database/', 'net/http')) or (imported.startswith('github.com/') and not (module_prefix and imported.startswith(module_prefix)))):
                errors.append(f'{rel}: concrete dependency in application {imported}')
            if any('/services/' + other + '/internal/' in imported for other in CFG['components'] if other != component):
                errors.append(f'{rel}: foreign service internal import {imported}')
    if errors:
        die('\n'.join(errors))


def go_check(component, data):
    folder = directory(component)
    mod = folder / 'go.mod'
    if not mod.is_file() or not re.search(r'(?m)^go\s+' + re.escape(CFG['go_version']) + r'\s*$', mod.read_text()):
        die('service go.mod must pin exact Go ' + CFG['go_version'])
    import_rules(component)
    execute(['go', 'vet', './...'], cwd=folder)
    output = execute(['go', 'test', '-count=1', '-json', '-coverprofile=' + str(ROOT / '.pr12-build' / (component + '.cover')), './...'], cwd=folder, capture=True)
    events = [json.loads(line) for line in output.splitlines() if line.startswith('{')]
    ran = {e['Test'] for e in events if e.get('Action') == 'run' and e.get('Test')}
    passed = {e['Test'] for e in events if e.get('Action') == 'pass' and e.get('Test')}
    required = set(data['expected_test_names'])
    if not required <= ran or not required <= passed:
        die('declared named tests did not actually run and pass: ' + str(sorted(required - (ran & passed))))
    print(output, end='')
    if os.environ.get('PR12_RACE') == '1':
        execute(['go', 'test', '-race', '-count=1', './...'], cwd=folder)
    else:
        print('Race check NOT_RUN; use PR12_RACE=1 where supported and report scope honestly.')


def frontend_build(mode='dev'):
    tool = ROOT / 'tooling/pr12/frontend'
    tsc = tool / 'node_modules/typescript/bin/tsc'
    if not tsc.is_file():
        die('install isolated exact cohort: cd tooling/pr12/frontend && npm ci --strict-peer-deps')
    commit = execute(['git', 'rev-parse', 'HEAD'], capture=True).strip()
    dirty = bool(execute(['git', 'status', '--porcelain'], capture=True).strip())
    metadata = {'version': 'pr12', 'commit': commit, 'dirty': dirty, 'buildMode': mode}
    generated = ROOT / 'tools/pr12/component/build-info.ts'
    generated.parent.mkdir(parents=True, exist_ok=True)
    generated.write_text('// GENERATED by workspace.py; not business code.\nexport const buildInfo = ' + json.dumps(metadata) + ' as const;\n')
    output_tree = ROOT / '.pr12-build/frontend'
    if output_tree.is_dir():
        shutil.rmtree(output_tree)
    flags = ['--sourceMap', 'true', '--removeComments', 'false'] if mode == 'dev' else ['--sourceMap', 'false', '--removeComments', 'true']
    execute(['node', tsc, '--project', tool / 'tsconfig.json', *flags], cwd=ROOT)
    # Node resolves packages from generated output ancestry; put only an ephemeral local link here.
    link = ROOT / '.pr12-build/frontend/node_modules'
    if not link.exists():
        link.parent.mkdir(parents=True, exist_ok=True)
        link.symlink_to(tool / 'node_modules', target_is_directory=True)
    (ROOT / '.pr12-build/frontend/package.json').write_text('{"type":"module"}\n')


def frontend_check(data):
    frontend_build()
    paths = sorted((ROOT / '.pr12-build/frontend/src/features/case-status').rglob('*.test.js'))
    paths += sorted((ROOT / '.pr12-build/frontend/src/features/case-status').rglob('*.test.mjs'))
    if not paths:
        die('no compiled native Node tests found; zero tests cannot pass')
    test_env = os.environ.copy()
    test_env['NODE_V8_COVERAGE'] = str(ROOT / '.pr12-build/frontend-coverage')
    result = execute(['node', '--test', '--test-timeout=30000', '--experimental-test-coverage', '--test-reporter=tap', *paths], capture=True, env=test_env)
    verify_node_results(result, data['expected_test_names'])
    print(result, end='')


def verify_node_results(result, expected_names):
    if not re.search(r'# tests [1-9][0-9]*\b', result) or not re.search(r'# pass [1-9][0-9]*\b', result):
        die('native Node test runner did not report nonzero passing tests')
    for name in expected_names:
        matches = [line for line in result.splitlines() if re.search(r'^\s*ok\s+\d+\s+-\s+' + re.escape(name) + r'(?:\s+#.*)?\s*$', line)]
        if not any(not re.search(r'#\s*(?:SKIP|TODO)\b', line, re.IGNORECASE) for line in matches):
            die('declared named test did not actually pass (missing/skipped/todo): ' + name)


def build(component, data, mode):
    out = ROOT / '.pr12-build' / component / mode
    out.mkdir(parents=True, exist_ok=True)
    if CFG['kind'] == 'backend':
        package = './cmd/' + component + '-contractcheck'
        if not (directory(component) / package.removeprefix('./')).is_dir():
            die('C1 must add the actual diagnostic entrypoint: ' + package)
        commit = execute(['git', 'rev-parse', 'HEAD'], capture=True).strip()
        dirty = bool(execute(['git', 'status', '--porcelain'], capture=True).strip())
        flags = f'-X main.version=pr12 -X main.commit={commit} -X main.dirty={str(dirty).lower()} -X main.buildMode={mode}'
        argv = ['go', 'build', '-ldflags', flags]
        argv += ['-gcflags=all=-N -l'] if mode == 'dev' else ['-trimpath']
        target = out / (component + '-contractcheck')
        execute([*argv, '-o', target, package], cwd=directory(component))
        return [str(target)]
    frontend_build(mode)
    target = ROOT / '.pr12-build/frontend/tools/pr12/case-contractcheck.js'
    if not target.is_file():
        die('C1 must add tools/pr12/case-contractcheck.ts')
    return ['node', *(['--enable-source-maps'] if mode == 'dev' else []), str(target)]


def smoke(component, data):
    # This validates declared fixture outcomes, not whole service readiness or external truth.
    for mode in ('dev', 'release'):
        command = build(component, data, mode)
        folder = directory(component)
        with tempfile.TemporaryDirectory(prefix='pr12-outside-checkout-') as tmp:
            dist = Path(tmp)
            if CFG['kind'] == 'backend':
                copied = dist / Path(command[0]).name
                shutil.copy2(command[0], copied)
                command = [str(copied)]
            else:
                # Pure diagnostic distribution must declare and copy its actual runtime tree.
                shutil.copytree(ROOT / '.pr12-build/frontend', dist / 'runtime', symlinks=False,
                                ignore=shutil.ignore_patterns('node_modules'))
                if data.get('runtime_dependencies'):
                    die('dependencyful JS distribution needs explicit packaging Task; wrapper only handles declared dependency-free diagnostic core')
                command = ['node', *(['--enable-source-maps'] if mode == 'dev' else []), str(dist / 'runtime/tools/pr12/case-contractcheck.js')]
            for key in ('positive_fixture', 'negative_fixture'):
                shutil.copy2(folder / data[key], dist / (key + '.json'))
            version = execute([*command, '--version'], cwd=dist, capture=True, timeout=30)
            if not execute(['git', 'rev-parse', '--short', 'HEAD'], capture=True).strip() in version:
                die('--version must identify the source SHA, not only the mode')
            good = execute([*command, '--fixture', dist / 'positive_fixture.json', '--format', 'json'], cwd=dist, capture=True, timeout=30)
            for expected in data['positive_output_contains']:
                if expected not in good:
                    die('positive diagnostic output missing expected marker: ' + expected)
            for args in (['--fixture', str(dist / 'negative_fixture.json'), '--format', 'json'], ['--fixture', str(dist / 'positive_fixture.json'), '--format', 'invalid']):
                try:
                    result = subprocess.run([*command, *args], cwd=dist, capture_output=True, text=True, timeout=30)
                except subprocess.TimeoutExpired:
                    die('negative diagnostic scenario timed out instead of returning a bounded error')
                if result.returncode == 0:
                    die('negative fixture/invalid format incorrectly returned exit 0')
            print(json.dumps({'component': component, 'mode': mode, 'diagnostic_smoke': 'PASS', 'scope': 'declared synthetic fixtures only'}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['validate', 'status', 'check', 'build', 'smoke', 'inspect'])
    parser.add_argument('component', nargs='?')
    parser.add_argument('--mode', choices=['dev', 'release'], default='dev')
    parser.add_argument('--require', action='store_true', help='fail when the selected student component is absent')
    args = parser.parse_args()
    if args.action == 'validate':
        validate(); return
    components = [args.component] if args.component else CFG['components']
    if any(c not in CFG['components'] for c in components):
        die('unknown component')
    (ROOT / '.pr12-build').mkdir(exist_ok=True)
    for component in components:
        data = implemented(component, args.require)
        if not data or args.action == 'status':
            if data:
                print(json.dumps({'component': component, 'status': 'IMPLEMENTATION_DECLARED_NOT_CHECKED'}))
            continue
        check_versions()
        if args.action == 'check':
            if CFG['kind'] == 'backend': go_check(component, data)
            else: frontend_check(data)
        elif args.action == 'build':
            print('Artifact command: ' + ' '.join(build(component, data, args.mode)))
        elif args.action == 'smoke':
            smoke(component, data)
        else:
            command = build(component, data, args.mode)
            if CFG['kind'] == 'backend':
                execute(['go', 'version', '-m', command[0]])
                if shutil.which('readelf'): execute(['readelf', '-h', command[0]])
                else: print('readelf NOT_AVAILABLE; install binutils for detailed ELF inspection.')
            else:
                execute(['node', '--check', command[-1]])
                print('Inspect generated JS, source maps and exact npm ls; no native ELF claim.')


if __name__ == '__main__':
    main()
