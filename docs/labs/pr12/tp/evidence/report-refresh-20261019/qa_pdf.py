import pdfplumber, json,re,hashlib,datetime,zipfile,collections
from pathlib import Path
from docx import Document
R=Path('/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe');P=R/'docs/labs/pr12/tp';E=R.parent/'hit32-report-evidence';D=P/'evidence/report-refresh-20261019';spec=json.loads((P/'reports/report-content.json').read_text());records=[]
norm=lambda s:''.join(re.findall('[0-9a-zа-яё]+',s.casefold()))
for lab in spec:
 path=E/{'0104-pz01':'render-final-v3','0104-pz02':'render-final-v4','0302-pz01':'render-final-v6','0302-pz02':'render-final-v6'}[lab]/lab/(lab+'-tp.pdf');dpath=P/'reports'/(lab+'-tp.docx');doc=Document(dpath);body=''.join(t.text or '' for t in doc.element.body.xpath('.//w:t'))
 with pdfplumber.open(path) as pdf:
  pages=[];outside=[]
  for n,p in enumerate(pdf.pages,1):
   txt=p.extract_text() or '';chars=p.chars
   bad=[c for c in chars if c['x0'] < -0.5 or c['x1'] > p.width+0.5 or c['top'] < -0.5 or c['bottom'] > p.height+0.5];outside.extend((n,c.get('text')) for c in bad)
   assert len(txt.strip())>10,(lab,'empty page',n)
   pages.append({'page':n,'characters':len(txt),'normalized_text':norm(txt),'footer_page_number':txt.strip().splitlines()[-1]})
  assert not outside,(lab,'text outside page',outside)
  for label,number in spec[lab].get('toc_pages',{}).items():
   needle=norm(re.sub(r'^\d+(?:\.\d+)*\s*','',label));assert needle in pages[number-1]['normalized_text'],(lab,'TOC mismatch',label,number)
  actual=''.join(p['normalized_text'] for p in pages);missing=collections.Counter(norm(body))-collections.Counter(actual)
  # Repeated table headers/page numbers add text in PDF. Character accounting is a sanity check,
  # never used alone as proof of complete round-trip retention.
  assert not missing,(lab,'character deficit',dict(missing))
  records.append({'lab':lab,'docx_sha256':hashlib.sha256(dpath.read_bytes()).hexdigest(),'pdf_sha256':hashlib.sha256(path.read_bytes()).hexdigest(),'pages':len(pages),'text_outside_page':outside,'empty_pages':0,'character_deficit':dict(missing),'toc_pages':spec[lab].get('toc_pages',{}),'footer_values':[p['footer_page_number'] for p in pages],'check':'PDF_TEXT_GEOMETRY_AND_TOC_PASS','visual_inspection':'SEPARATE_MANUAL_PAGE_RECORD'})
(D/'pdf-integrity.json').write_text(json.dumps({'at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'tool':'Bundled Python pdfplumber; rendered with bundled LibreOffice/Poppler via documents/render_docx.py','records':records},ensure_ascii=False,indent=2)+'\n');print([(r['lab'],r['pages'],r['check']) for r in records])
