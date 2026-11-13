from pathlib import Path
import hashlib,json,zipfile,xml.etree.ElementTree as ET,datetime,sys,re
base=Path(__file__).resolve().parent
R=base/'hitpipe' if (base/'hitpipe').is_dir() else next(p for p in base.parents if (p/'docs/labs/pr12/manifest.json').is_file())
P=R/'docs/labs/pr12/tp';reports=P/'reports';ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
expected={'0104-pz01':7,'0104-pz02':6,'0302-pz01':3,'0302-pz02':5};rows=[]
spec=json.loads((reports/'report-content.json').read_text())
norm=lambda text:re.sub(r'\s+','',text)
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
  paragraphs=[''.join(x.text or '' for x in p.findall('.//w:t',ns)) for p in doc.findall('.//w:body/w:p',ns)]
  full=norm('\n'.join(paragraphs))
  for value in spec[lab]['paragraph_updates'].values():
   if value.startswith('Окончательный report SHA'):continue
   assert norm(value) in full,(lab,'lost source paragraph',value[:80])
  for index,values in spec[lab].get('table_headers',{}).items():
   cells=tables[int(index)].findall('w:tr',ns)[0].findall('w:tc',ns)
   assert [norm(''.join(x.text or '' for x in c.findall('.//w:t',ns))) for c in cells]==[norm(str(v)) for v in values],(lab,'lost table header',index)
  for index,data in spec[lab]['table_rows'].items():
   actual=tables[int(index)].findall('w:tr',ns)[1:]
   assert len(actual)==len(data),(lab,'lost table row',index)
   for row,values in zip(actual,data):
    cells=row.findall('w:tc',ns)
    assert len(cells)==len(values)
    for cell,value in zip(cells,values):
     assert norm(''.join(x.text or '' for x in cell.findall('.//w:t',ns)))==norm(str(value)),(lab,'changed table cell',index,value)
  for value in spec[lab].get('appendix_paragraphs',[]):assert norm(value) in full,(lab,'lost support paragraph')
  olddoc=ET.fromstring(original.read('word/document.xml'))
  oldtables=olddoc.findall('.//w:body/w:tbl',ns)
  for index in [0,1]:assert ET.tostring(tables[index])==ET.tostring(oldtables[index]),(lab,'teacher form changed')
  for tag in ['pgSz','pgMar']:
   assert [ET.tostring(x) for x in doc.findall('.//w:sectPr/w:'+tag,ns)]==[ET.tostring(x) for x in olddoc.findall('.//w:sectPr/w:'+tag,ns)],(lab,'page geometry changed')
  for table in tables[3:]:
   for row in table.findall('w:tr',ns):
    for cell in row.findall('w:tc',ns):assert ''.join(cell.itertext()).strip(),lab+' blank body table cell'
  rels=ET.fromstring(z.read('word/_rels/document.xml.rels'));ids={x.attrib['Id'] for x in rels}
  bookmarks={x.attrib['{'+ns['w']+'}name'] for x in doc.findall('.//w:bookmarkStart',ns)}
  for h in doc.findall('.//w:hyperlink',ns):
   anchor=h.get('{'+ns['w']+'}anchor')
   if anchor:assert anchor in bookmarks,(lab,'broken internal outline')
   else:assert h.attrib['{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id'] in ids
  for rel in rels:
   if rel.get('Type','').endswith('/hyperlink'):assert rel.get('Target','').startswith(('https://','http://'))
  rows.append({'file':path.name,'sha256':hashlib.sha256(path.read_bytes()).hexdigest(),'figures':slots,'tables':len(tables),'changed_parts':changed,'status':'STRUCTURAL_PASS'})
assert sum(x['figures'] for x in rows)==21
assert len(json.loads((P/'criteria-crosswalk.json').read_text())['criteria'])==29
figures=json.loads((P/'evidence/screenshots/instructions.json').read_text());assert len(figures)==21
required={'source','state','cwd','command','steps','must_be_visible','target_file','caption','insert_at','redact','verify_before_insert'}
for row in figures:assert all(row.get(k) for k in required)
record={'observed_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'argv':sys.argv,'cwd':str(Path.cwd()),'exit':0,'python':sys.version,'docs':rows,'total_image_placeholders':21,'report_screenshots_created_or_captured':0,'criteria':29,'visual_qa':'SEPARATE_RENDER_AND_PAGE_INSPECTION_RECORD_REQUIRED','status':'PASS_STRUCTURAL_AND_SOURCE_TEXT_TABLES'}
print(json.dumps(record,ensure_ascii=False,indent=2))
