#!/usr/bin/env python3
"""Verify the committed report package without rewriting it or needing DOCX libs."""
import hashlib
import json
from pathlib import Path
import re
from urllib.parse import urlparse
from zipfile import ZipFile
import xml.etree.ElementTree as E

HERE=Path(__file__).resolve().parent
BASE=HERE.parent
ROOT=HERE.parents[4]
NS={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}

def verify():
    source=json.loads((ROOT/'docs/labs/pr12/manifest.json').read_text())['criteria_crosswalk']['dk']
    cross=json.loads((BASE/'criteria-crosswalk.json').read_text())
    assert len(cross['criteria'])==29
    assert {r['id'] for r in cross['criteria']}==set(source)
    assert not cross['submission_ready']
    for row in cross['criteria']:
        assert row['actual_status'] in {'PASS_AGENT_CHECKED','HUMAN_INTERVIEW_PENDING','HUMAN_REVIEW_MERGE_PENDING','EXTERNAL_OPPOSITION_PENDING','HUMAN_CLARITY_PENDING','HUMAN_PEER_PENDING','PASS_DELEGATED_TECHNICAL_REVIEW_MERGED','DELEGATED_CHECK_PASS_PERSONAL_ATTESTATION_PENDING'}
        for path in row['evidence_paths']:assert (BASE/path).is_file(),path
    if any(r['actual_status']=='PASS_DELEGATED_TECHNICAL_REVIEW_MERGED' for r in cross['criteria']):
        merged=json.loads((BASE/'submission/integration-readback.json').read_text())
        assert {r['number'] for r in merged}==set(range(12,20)) and all(r['merged'] for r in merged)
    if any(r['actual_status']=='DELEGATED_CHECK_PASS_PERSONAL_ATTESTATION_PENDING' for r in cross['criteria']):
        peer=json.loads((BASE/'submission/evidence/peer-technical/commands.json').read_text())
        assert len(peer)==4 and all(r['exit_code']==0 for r in peer)
    figures=json.loads((HERE/'figures.json').read_text());assert len(figures)==21
    expected={'0302-pz01':3,'0302-pz02':5,'0104-pz01':7,'0104-pz02':6}
    validation=json.loads((HERE/'validation.json').read_text())
    all_ids=[];links=0
    for report in validation['reports']:
        path=HERE/report['file'];assert hashlib.sha256(path.read_bytes()).hexdigest()==report['sha256'],path
        with ZipFile(path) as z:
            assert z.testzip() is None
            for n in z.namelist():
                if n.endswith(('.xml','.rels')):E.fromstring(z.read(n))
            assert not any(n.startswith('word/media/') for n in z.namelist())
            tree=E.fromstring(z.read('word/document.xml'))
            # Three retained cover tables precede the generated report tables.
            # Verify pagination rules independently of the generator/renderer.
            for table in tree.findall('.//w:tbl',NS)[3:]:
                rows=table.findall('w:tr',NS)
                assert rows[0].find('w:trPr/w:tblHeader',NS) is not None
                for row in rows:
                    flag=row.find('w:trPr/w:cantSplit',NS)
                    assert flag is not None and flag.get('{'+NS['w']+'}val','true') not in ('false','0')
            text='\n'.join(t.text or '' for t in tree.findall('.//w:t',NS))
            assert '{{' not in text and 'TEMPLATE_UNFILLED' not in text
            assert 'Д. Карачебан' in text and 'ВПИСАТЬ ГРУППУ' in text
            assert 'CONTENT_READY_IMAGES_PENDING' in text
            ids=re.findall(r'\[МЕСТО ДЛЯ СКРИНШОТА: ([a-z0-9-]+)\]',text)
            name=report['file'].removesuffix('-dk.docx');assert len(ids)==expected[name]
            for identity in ids:
                f=figures[identity]
                for field in ['caption','insert_location','state','navigation','source_sha','cwd','command','must_be_visible','target_file','redaction']:assert f[field]
                assert re.fullmatch('[0-9a-f]{40}',f['source_sha'])
                assert f['target_file'] in text and f['caption'] in text and f['source_sha'] in text
            all_ids+=ids
            for n in z.namelist():
                if not n.endswith('.rels'):continue
                for relation in E.fromstring(z.read(n)):
                    if relation.get('TargetMode')=='External':
                        target=relation.get('Target');assert urlparse(target).scheme in ('http','https');links+=1
    assert len(all_ids)==len(set(all_ids))==21 and set(all_ids)==set(figures)
    index=json.loads((BASE/'evidence/index.json').read_text())
    for record in index['entries']:
        path=ROOT/record['path'];assert path.is_file(),path
        assert path.stat().st_size==record['bytes'],path
        assert hashlib.sha256(path.read_bytes()).hexdigest()==record['sha256'],path
    # Historical metrics may describe the current package only while the owned
    # module inputs still match. Later code changes must refresh the evidence.
    checked=json.loads((BASE/'evidence/c3-final-check.json').read_text())
    actual=[{'path':str(p.relative_to(ROOT)),'sha256':hashlib.sha256(p.read_bytes()).hexdigest()}
            for p in sorted((ROOT/'services/iam').rglob('*')) if p.is_file() and '__pycache__' not in p.parts]
    assert actual==checked['module_files'],'Current IAM module differs from the reported checked inputs'
    for p in BASE.rglob('*.md'):
        for target in re.findall(r'\]\(([^)]+)\)',p.read_text()):
            if target.startswith(('http:','https:','#')):continue
            assert (p.parent/target.split('#')[0]).is_file(),(p,target)
    submission=json.loads((BASE/'submission-record.json').read_text())
    assert not submission['submission_ready'] and all(not tag['created'] and tag['commit'] is None for tag in submission['tags'])
    ci=json.loads((BASE/'evidence/ci-c3/iam/mutations-ci/protocol.json').read_text())
    assert ci['source_sha']=='2cd7c032e8ede98c75181599b440e97657bff197'
    assert len(ci['mutations'])==7 and all(m['result']=='KILLED' and m['build']['exit_code']==0 and m['restored'] for m in ci['mutations'])
    assert ci['final_clean']['exit_code']==0 and ci['restored_worktree_clean']
    initial=json.loads((BASE/'evidence/mutations-initial/protocol.json').read_text())
    assert len(initial['mutations'])==7 and sum(m['result']=='SURVIVED' for m in initial['mutations'])==1
    rendering='NOT_RUN: no current visual-validation.json'
    visual_path=HERE/'visual-validation.json'
    if visual_path.exists():
        visual=json.loads(visual_path.read_text())
        assert visual['status']=='PASS_AUTHOR_LAYOUT_CHECK'
        assert not visual['independent_reapproval_claimed'] and not visual['student_personal_gates_attested']
        assert {r['file'] for r in visual['reports']}=={r['file'] for r in validation['reports']}
        for report in visual['reports']:
            assert hashlib.sha256((HERE/report['file']).read_bytes()).hexdigest()==report['docx_sha256']
            assert hashlib.sha256((ROOT/report['pdf_path']).read_bytes()).hexdigest()==report['pdf_sha256']
            assert report['visually_checked_pages']==list(range(1,report['pages']+1))
        rendering='PASS_AUTHOR_LAYOUT_CHECK; real LibreOffice/Poppler pages, exact DOCX/PDF hashes verified'
    return {'status':'PASS','reports':4,'mandatory_results':29,'unique_screenshot_placeholders':21,'verified_evidence_files':len(index['entries']),'external_docx_links':links,'submitted_or_human_accepted':False,'rendering':rendering}

if __name__=='__main__':
    result=verify()
    current=BASE/'submission/verify_submission.py'
    if current.is_file():
        import subprocess,sys
        subprocess.run([sys.executable,str(current)],check=True)
        result['current_illustrated_package']='PASS; external academic evidence still pending'
    print(json.dumps(result,ensure_ascii=False,indent=2))
