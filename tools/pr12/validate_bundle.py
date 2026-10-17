#!/usr/bin/env python3
"""Validate portable planning assets, never assert student or human completion."""
import hashlib
import json
from pathlib import Path
import sys
import zipfile
import xml.etree.ElementTree as ET


def validate(root):
    base=(root/'docs/labs/pr12').resolve()
    checked=0
    for index in ('source-files.json','export-files.json'):
        rows=json.loads((base/index).read_text())['files']
        assert rows, 'Empty source/export inventory'
        seen=set()
        for row in rows:
            p=(base/row['path']).resolve()
            assert p.is_relative_to(base), 'Inventory path escapes package'
            assert row['path'] not in seen, 'Duplicate inventory path'
            seen.add(row['path'])
            assert p.is_file(), 'Missing package file: '+row['path']
            assert hashlib.sha256(p.read_bytes()).hexdigest()==row['sha256'], 'Package drift: '+row['path']
            checked+=1
    manifest=json.loads((base/'manifest.json').read_text())
    assert manifest['mode']=='MANUAL_LAB_PR12'
    assert len(manifest['features'])==5
    tasks=manifest['tasks'];assert len(tasks)==41
    by_local={t['local_id']:t for t in tasks};assert len(by_local)==41
    keys={t['key'] for t in tasks};assert len(keys)==41
    for f in manifest['features']:
        own=[t for t in tasks if t['person_code']==f['code']]
        assert len(own)==8, 'Expected eight personal tasks'
        assert {t['role'] for t in own}=={'R1','R2','C1','C2','C3','D1','H1','REPORT'}
        assert len(f['code_tasks'])==3
        assert len(manifest['criteria_crosswalk'][f['code']])==29
        for links in manifest['criteria_crosswalk'][f['code']].values():
            assert links and all(x in by_local for x in links)
        for t in own:
            assert t['parent_key']==f['feature_key']
            assert all(k in keys for k in t['dependency_keys'])
            export=json.loads((base/'tasks'/f"{t['key']}.json").read_text())
            assert export['key']==t['key'] and export['parent_key']==f['feature_key']
            assert export['mode']=='MANUAL_LAB_PR12'
        prompt=(base/f['agent_prompt']).read_text()
        assert f['feature_key'] in prompt and 'FEATURE_KEY' not in prompt
    # Actual DAG, including shared infrastructure. Human gates can remain pending.
    by_key={t['key']:t for t in tasks};done=set();active=set()
    def visit(key):
        assert key not in active, 'Dependency cycle'
        if key in done:return
        active.add(key)
        for parent in by_key[key]['dependency_keys']:visit(parent)
        active.remove(key);done.add(key)
    for key in by_key:visit(key)
    forms=list((base/'templates').glob('*-report-template.docx'));assert len(forms)==4
    for form in forms:
        with zipfile.ZipFile(form) as z:
            assert z.testzip() is None
            ET.fromstring(z.read('word/document.xml'))
            assert not any(x.startswith('word/media/') for x in z.namelist()), 'Template must not include screenshots'
    print(json.dumps({'status':'PORTABLE_PACKAGE_VALIDATION_PASS','verified_files':checked,
        'features':5,'tasks':41,'criteria_links':145,'templates':4,
        'student_work':'NOT_ASSESSED','human_gates':'NOT_ASSESSED'}))


if __name__=='__main__':
    try:validate(Path(__file__).resolve().parents[2])
    except (AssertionError,KeyError,ValueError,OSError,zipfile.BadZipFile) as exc:
        sys.exit('PACKAGE_ERROR: '+str(exc))
