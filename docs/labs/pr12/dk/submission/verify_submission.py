#!/usr/bin/env python3
"""Verify the actual report artifacts, typography and source-text preservation.

Uses only the standard library; does not attest interviews or student actions.
"""
from pathlib import Path
from zipfile import ZipFile
import xml.etree.ElementTree as ET
import re,json,hashlib,unicodedata

HERE=Path(__file__).resolve().parent
NS={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main','pic':'http://schemas.openxmlformats.org/drawingml/2006/picture'}
def q(s):return '{'+NS['w']+'}'+s
def text(n):return ''.join(x.text or ''for x in n.findall('.//w:t',NS))
def packed(s):return re.sub(r'\s+','',unicodedata.normalize('NFKC',s).replace('\u00ad','')).casefold()
def display(s):
    s=re.sub(r'\[([^\]]+)\]\(https?://[^)]+\)',r'\1',s)
    return s.replace('**','').replace('`','')
def cell_fields(line):
    result=[];buf='';code=False
    for c in line.strip().strip('|'):
        if c=='`':code=not code
        if c=='|'and not code:result.append(buf.strip());buf=''
        else:buf+=c
    result.append(buf.strip());return result

def main():
    captures=json.loads((HERE/'capture-manifest.json').read_text())
    # Captures retain their original bytes; source views never replace raw logs.
    root=HERE.parents[4]
    for identity,capture in captures.items():
        if capture.get('package_html'):
            assert hashlib.sha256((HERE/capture['package_html']).read_bytes()).hexdigest()==capture['view_html_sha256']
        for source in capture.get('sources',[]):
            target=HERE/source['package_path'] if source.get('package_path') else root/source['repository_path'] if source.get('repository_path') else None
            if target is not None:assert hashlib.sha256(target.read_bytes()).hexdigest()==source['sha256'],(identity,str(target))
    artifacts=json.loads((HERE/'artifact-manifest.json').read_text())
    archive=HERE/artifacts['archive']['file']
    assert hashlib.sha256(archive.read_bytes()).hexdigest()==artifacts['archive']['sha256']
    with ZipFile(archive)as z:
        assert len(z.namelist())==4 and z.testzip()is None
        for item in artifacts['files']:
            assert hashlib.sha256(z.read(item['archive_name'])).hexdigest()==item['sha256']
            assert hashlib.sha256((HERE/item['file']).read_bytes()).hexdigest()==item['sha256']
    visual=json.loads((HERE/'visual-validation.json').read_text())
    assert visual['status']=='PASS_AUTHOR_LAYOUT_CHECK' and not visual['independent_reapproval_claimed']
    for item in visual['reports']:
        assert hashlib.sha256((HERE/item['file']).read_bytes()).hexdigest()==item['docx_sha256']
        assert hashlib.sha256((root/item['pdf_path']).read_bytes()).hexdigest()==item['pdf_sha256']
        assert item['visually_checked_pages']==list(range(1,item['pages']+1))
    out={'status':'PASS','submission_ready':False,'external_evidence':'Interview, personal clarity and opposition not attested; delegated peer check explicitly disclosed.','reports':[]}
    for path in sorted(HERE.glob('*-dk.docx')):
        name=path.stem.removesuffix('-dk');checks=[]
        with ZipFile(path)as z:
            assert z.testzip()is None
            for member in z.namelist():
                if member.endswith(('.xml','.rels')):ET.fromstring(z.read(member))
            styles=ET.fromstring(z.read('word/styles.xml'))
            defaults=[st for st in styles.findall('w:style',NS) if st.get(q('default'))=='1' and st.get(q('type'))=='paragraph']
            assert len(defaults)==1
            fonts=defaults[0].find('w:rPr/w:rFonts',NS)
            assert fonts is not None and fonts.get(q('ascii'))=='Times New Roman'
            for st in styles.findall('w:style',NS):
                label=st.find('w:name',NS)
                if label is not None and re.fullmatch(r'heading[ _]?[123]',label.get(q('val'),'').casefold()):
                    italic=st.find('w:rPr/w:i',NS);assert italic is not None and italic.get(q('val'),'true')not in ['0','false']
            root=ET.fromstring(z.read('word/document.xml'));nodes=list(root.find('w:body',NS))
            start=next(i for i,n in enumerate(nodes)if text(n).strip()=='ВВЕДЕНИЕ')
            before=packed(''.join(text(n)for n in nodes[:start]))
            with ZipFile(HERE.parent/'reports'/path.name)as original:
                original_nodes=list(ET.fromstring(original.read('word/document.xml')).find('w:body',NS))
                cut=next(i for i,n in enumerate(original_nodes)if text(n).strip()in ['СОДЕРЖАНИЕ','ВВЕДЕНИЕ'])
                expected=packed(''.join(text(n)for n in original_nodes[:cut]))
                assert before.startswith(expected),(path.name,'title page content changed')
            content=nodes[start:];alltext='\n'.join(text(n)for n in content);plain=packed(alltext)
            assert '[местодляскриншота'not in plain
            assert not re.search(r'(?i)Codex|агент|NOT_CONDUCTED|NOT_PERFORMED|SUBMISSION_READY|HUMAN_[A-Z_]+|это шаблон|на текущий момент.{0,20}не готов',alltext),(path.name,'operational prose leaked into report')
            core=ET.fromstring(z.read('docProps/core.xml'))
            assert core.find('{http://purl.org/dc/elements/1.1/}creator').text=='Дмитрий Карачебан'
            assert 'codex' not in z.read('docProps/core.xml').decode().casefold()
            body_tables=[n for n in content if n.tag==q('tbl')]
            for table in body_tables:
                rows=table.findall('w:tr',NS)
                assert rows[0].find('w:trPr/w:tblHeader',NS)is not None
                assert all(row.find('w:trPr/w:cantSplit',NS)is not None and row.find('w:trPr/w:cantSplit',NS).get(q('val'),'true')not in ['0','false'] for row in rows)
                table_borders=table.find('w:tblPr/w:tblBorders',NS)
                for cell in table.findall('w:tr/w:tc',NS):
                    assert table_borders is not None or cell.find('w:tcPr/w:tcBorders',NS)is not None
            # Source requirements/cells must survive Markdown conversion intact.
            missing=[]
            regular=[];inside_code=False
            for line in (HERE/(path.stem+'.md')).read_text().splitlines():
                s=line.strip()
                if s.startswith('```'):inside_code=not inside_code;continue
                if not s or s.startswith(('# ','<!--')):continue
                if not inside_code and not s.startswith(('#','|')):regular.append(packed(display(re.sub(r'^-\s+','',s))))
                if s.startswith('|'):
                    fragments=cell_fields(s)
                    if all(re.fullmatch(r':?-+:?',x)for x in fragments):continue
                else:fragments=[re.sub(r'^#{2,4}\s+|^-\s+','',s)]
                for fragment in fragments:
                    v=packed(display(fragment))
                    if v and v not in plain:missing.append(fragment)
            assert not missing,(path.name,'missing source text',missing[:3])
            # Verify prose indentation and justification in the produced file.
            prose_count=0
            for n in content:
                if n.tag!=q('p') or packed(text(n)) not in regular:continue
                ind=n.find('w:pPr/w:ind',NS);jc=n.find('w:pPr/w:jc',NS)
                assert ind is not None and ind.get(q('firstLine'))=='709',(path.name,'first line indent',text(n)[:50])
                assert jc is not None and jc.get(q('val'))=='both',(path.name,'justification')
                prose_count+=1
            assert prose_count>4,(path.name,'prose checks absent')
            # Verify actual glyph-bearing runs and paragraph line spacing.
            for n in content:
                for sz in n.findall('.//w:rPr/w:sz',NS):assert sz.get(q('val'))=='28',(path.name,'non-14-point text')
                for sp in n.findall('.//w:pPr/w:spacing',NS):
                    if sp.get(q('line')):assert sp.get(q('line'))=='360',(path.name,'line spacing')
            captions=[]
            for n in content:
                if n.tag!=q('p'):continue
                s=text(n)
                if re.match(r'^Рисунок \d+ - ',s):
                    captions.append(int(re.match(r'^Рисунок (\d+)',s)[1]))
                    jc=n.find('w:pPr/w:jc',NS);assert jc is not None and jc.get(q('val'))=='center'
            assert list(dict.fromkeys(captions))==sorted(set(captions)),(path.name,'figure order')
            expected_images=[i for ident,v in captures.items()if ident.startswith(name)for i in v.get('images',[])]
            pics=root.findall('.//pic:pic',NS);assert len(pics)==len(expected_images)
            hashes={hashlib.sha256(z.read(n)).hexdigest()for n in z.namelist()if n.startswith('word/media/')}
            assert {i['sha256']for i in expected_images}<=hashes
            out['reports'].append({'file':path.name,'sha256':hashlib.sha256(path.read_bytes()).hexdigest(),'actual_captures':len(pics),'tables_with_borders_and_repeated_headers':len(body_tables),'all_rows_unsplit':True,'body_font_family':'Times New Roman','body_font_size_pt':14,'section_headings_italic':True,'line_spacing':1.5,'first_line_indent_cm':1.25,'prose_justified':True,'source_text_preserved':True,'title_content_unchanged':True,'caption_numbers_in_appearance_order':list(dict.fromkeys(captions))})
    (HERE/'quality-audit.json').write_text(json.dumps(out,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(out,ensure_ascii=False,indent=2))

if __name__=='__main__':main()
