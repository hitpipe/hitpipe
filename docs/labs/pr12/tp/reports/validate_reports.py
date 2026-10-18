from pathlib import Path
import hashlib,json,zipfile,xml.etree.ElementTree as ET,datetime,sys
base=Path(__file__).resolve().parent
R=base/'hitpipe' if (base/'hitpipe').is_dir() else next(p for p in base.parents if (p/'docs/labs/pr12/manifest.json').is_file())
P=R/'docs/labs/pr12/tp';reports=P/'reports';ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
expected={'0104-pz01':7,'0104-pz02':6,'0302-pz01':3,'0302-pz02':5};rows=[]
for lab,count in expected.items():
 path=reports/(lab+'-tp.docx');source=R/'docs/labs/pr12/templates'/(lab+'-report-template.docx')
 with zipfile.ZipFile(path) as z,zipfile.ZipFile(source) as original:
  assert z.testzip() is None
  for n in z.namelist():
   if n.endswith('.xml'):ET.fromstring(z.read(n))
  doc=ET.fromstring(z.read('word/document.xml'));texts=[x.text or '' for x in doc.findall('.//w:t',ns)];text='\n'.join(texts)
  slots=sum('[ВСТАВИТЬ РЕАЛЬНОЕ ИЗОБРАЖЕНИЕ ВРУЧНУЮ:' in t for t in texts)
  assert slots==count,(lab,slots)
  assert 'TEMPLATE_UNFILLED' not in text
  for token in ['{{.ImportPath}}','{{join .Imports " "}}']:text=text.replace(token,'GO_TEMPLATE_LITERAL')
  assert '{{' not in text,lab+' unresolved fields'
  assert 'CONTENT_READY_IMAGES_PENDING' in text
  assert not any(n.startswith('word/media/') for n in z.namelist())
  changed=[n for n in original.namelist() if n not in z.namelist() or original.read(n)!=z.read(n)]
  assert set(changed)<=set(['word/document.xml','word/_rels/document.xml.rels','docProps/core.xml','word/settings.xml'])
  tables=doc.findall('.//w:body/w:tbl',ns)
  for table in tables[3:]:
   for row in table.findall('w:tr',ns):
    for cell in row.findall('w:tc',ns):assert ''.join(cell.itertext()).strip(),lab+' blank body table cell'
  rels=ET.fromstring(z.read('word/_rels/document.xml.rels'));ids={x.attrib['Id'] for x in rels}
  for h in doc.findall('.//w:hyperlink',ns):assert h.attrib['{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id'] in ids
  rows.append({'file':path.name,'sha256':hashlib.sha256(path.read_bytes()).hexdigest(),'figures':slots,'tables':len(tables),'changed_parts':changed,'status':'STRUCTURAL_PASS'})
assert sum(x['figures'] for x in rows)==21
assert len(json.loads((P/'criteria-crosswalk.json').read_text())['criteria'])==29
figures=json.loads((P/'evidence/screenshots/instructions.json').read_text());assert len(figures)==21
required={'source','state','cwd','command','steps','must_be_visible','target_file','caption','insert_at','redact','verify_before_insert'}
for row in figures:assert all(row.get(k) for k in required)
record={'observed_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'argv':sys.argv,'cwd':str(Path.cwd()),'exit':0,'python':sys.version,'docs':rows,'total_image_placeholders':21,'generated_or_captured_images':0,'criteria':29,'visual_qa':'NOT_PERFORMED_USER_PROHIBITS_IMAGE_GENERATION','status':'PASS_STRUCTURAL_ONLY'}
print(json.dumps(record,ensure_ascii=False,indent=2))
