from pathlib import Path
import copy,json,re,zipfile,hashlib,argparse
from lxml import etree
from docx import Document
from docx.oxml import OxmlElement
from docx.oxml.ns import qn
from docx.shared import Pt
from docx.enum.table import WD_TABLE_ALIGNMENT
from docx.enum.text import WD_ALIGN_PARAGRAPH
from docx.opc.constants import RELATIONSHIP_TYPE as RT
here=Path(__file__).resolve().parent
R=here/'hitpipe' if (here/'hitpipe').is_dir() else next(p for p in here.parents if (p/'docs/labs/pr12/manifest.json').is_file())
B=R/'docs/labs/pr12';P=B/'tp';O=P/'reports'
spec=json.loads((O/'report-content.json').read_text());fig=json.loads((P/'evidence/screenshots/instructions.json').read_text())
parser=argparse.ArgumentParser();parser.add_argument('--lab',choices=tuple(spec));selected=parser.parse_args().lab
previous={r['lab']:r for r in json.loads((O/'report-manifest.json').read_text())} if selected else {}
manifest=[]
def replace_para(p,text):
 # Preserve paragraph formatting/style; replace the editable text slot.
 p.text=text

def fill_table(t,rows):
 sample=copy.deepcopy(t.rows[-1]._tr)
 for row in list(t.rows)[1:]:t._tbl.remove(row._tr)
 for data in rows:
  t._tbl.append(copy.deepcopy(sample));row=t.rows[-1]
  if len(data)!=len(row.cells):raise ValueError((len(data),len(row.cells),data))
  for c,v in zip(row.cells,data):
   style=c.paragraphs[0].style
   c.text=str(v)
   for p in c.paragraphs:p.style=style
  prop=row._tr.get_or_add_trPr()
  if prop.find(qn('w:cantSplit')) is None:prop.append(OxmlElement('w:cantSplit'))
 h=t.rows[0]._tr.get_or_add_trPr()
 if h.find(qn('w:tblHeader')) is None:h.append(OxmlElement('w:tblHeader'))

def add_hyperlinks(p):
 text=p.text
 if not re.search(r'https?://',text):return
 prototype=copy.deepcopy(p.runs[0]._r.rPr) if p.runs and p.runs[0]._r.rPr is not None else None
 def add_text(value):
  run=p.add_run(value)
  if prototype is not None:run._r.insert(0,copy.deepcopy(prototype))
 # Rebuild editable body text as real hyperlink runs; preserve paragraph geometry.
 p.clear();pos=0
 for m in re.finditer(r'https?://[^\s;]+',text):
  url=m.group().rstrip('.,)');end=m.start()+len(url)
  add_text(text[pos:m.start()])
  rel=p.part.relate_to(url,RT.HYPERLINK,is_external=True);h=OxmlElement('w:hyperlink');h.set(qn('r:id'),rel)
  r=OxmlElement('w:r');pr=copy.deepcopy(prototype) if prototype is not None else OxmlElement('w:rPr')
  oldstyle=pr.find(qn('w:rStyle'))
  if oldstyle is not None:pr.remove(oldstyle)
  style=OxmlElement('w:rStyle');style.set(qn('w:val'),'Hyperlink');pr.insert(0,style);r.append(pr)
  t=OxmlElement('w:t');t.text=url;r.append(t);h.append(r);p._p.append(h);pos=end
 add_text(text[pos:])

def all_paragraphs(d):
 yield from d.paragraphs
 for t in d.tables:
  for row in t.rows:
   for c in row.cells:yield from c.paragraphs

def readable_table(t):
 # Template Normal has a first-line indent inappropriate for narrow cells.
 for ri,row in enumerate(t.rows):
  for cell in row.cells:
   for p in cell.paragraphs:
    p.alignment=WD_ALIGN_PARAGRAPH.LEFT
    p.paragraph_format.first_line_indent=Pt(0)
    p.paragraph_format.line_spacing=1
    p.paragraph_format.space_after=Pt(3)
    for run in p.runs:
     run.font.size=Pt(12)
     if ri==0:run.bold=True

def link_outline(d, original, pages):
 # Static navigation with page numbers verified against the current PDF render.
 # Human image insertion changes pagination, so the final handoff requires refresh.
 normal=lambda t:re.sub(r'^\d+(?:\.\d+)*\s*','',t).strip().casefold()
 for p in original:
  if not p.style.name.lower().startswith('toc'):continue
  label=re.sub(r'\t\d+$','',p.text)
  target=next((q for q in d.paragraphs if not q.style.name.lower().startswith('toc') and normal(q.text)==normal(label)),None)
  if target is None:raise ValueError('Missing outline target: '+label)
  anchor='tp_section_'+str(original.index(p));bid=str(500+original.index(p))
  start=OxmlElement('w:bookmarkStart');start.set(qn('w:id'),bid);start.set(qn('w:name'),anchor)
  end=OxmlElement('w:bookmarkEnd');end.set(qn('w:id'),bid)
  target._p.insert(0,start);target._p.append(end)
  p.clear();h=OxmlElement('w:hyperlink');h.set(qn('w:anchor'),anchor)
  r=OxmlElement('w:r');text=OxmlElement('w:t');text.text=label;r.append(text);h.append(r);p._p.append(h)
  if label in pages:p.add_run('\t'+str(pages[label]))

for lab,item in spec.items():
 if selected and lab!=selected:continue
 source=B/'templates'/f'{lab}-report-template.docx';d=Document(source);original=list(d.paragraphs)
 for i,text in item['paragraph_updates'].items():replace_para(original[int(i)],text)
 replace_para(original[1],'КАФЕДРА № (не предоставлена)')
 replace_para(original[13],'Санкт-Петербург\n2026')
 # Student name only; signature and teacher grade remain untouched and empty.
 d.tables[2].cell(0,0).text='гр. №\nне указана';d.tables[2].cell(0,5).text='Тимофей Патрушев'
 for ci in [0,5]:
  for p in d.tables[2].cell(0,ci).paragraphs:
   p.paragraph_format.first_line_indent=Pt(0)
   for run in p.runs:run.font.size=Pt(12 if ci==0 else 14)
 for i,rows in item['table_rows'].items():fill_table(d.tables[int(i)],rows)
 for i,labels in item.get('table_headers',{}).items():
  for cell,label in zip(d.tables[int(i)].rows[0].cells,labels):cell.text=label
 for t in d.tables[3:]:readable_table(t)
 slots=[(i,p) for i,p in enumerate(original) if '[ВСТАВИТЬ РЕАЛЬНОЕ ИЗОБРАЖЕНИЕ ВРУЧНУЮ:' in p.text]
 own=[r for r in fig if r['lab']==lab];assert len(slots)==len(own)
 for (index,p),row in zip(slots,own):
  p.text='[ВСТАВИТЬ РЕАЛЬНОЕ ИЗОБРАЖЕНИЕ ВРУЧНУЮ: '+row['target_file'].split('/')[-1]+']'
  original[index+1].text='SOURCE: '+row['source']+'\nSTATE: '+row['state']+'\nCWD: '+row['cwd']+'\nCOMMAND: '+row['command']+'\nSTEPS: '+row['steps']
  original[index+2].text='MUST BE VISIBLE: '+row['must_be_visible']+'\nREDACT: '+row['redact']+'\nVERIFY: '+row['verify_before_insert']
  original[index+3].text='TARGET: '+row['target_file']+'\nCAPTION: '+row['caption']+'\nINSERT AT: '+row['insert_at']
  for instruction in original[index:index+4]:
   instruction.alignment=WD_ALIGN_PARAGRAPH.LEFT
   instruction.paragraph_format.first_line_indent=Pt(0)
   instruction.paragraph_format.line_spacing=1
   instruction.paragraph_format.space_after=Pt(6)
   for run in instruction.runs:run.font.size=Pt(10)
 for p in original:
  if p.text.startswith('Каждый слот заполнить'):
   p.text='Ниже точные инструкции человеку. Изображения не создавались и не снимались агентом. Источники, actual status и версии сохранены; ещё не состоявшиеся gates выполняются до снимка. Placeholder не подтверждает PASS.'
  if p.style.name.lower().startswith('toc'):
   p.text=re.sub(r'\t\d+$','',p.text)
 # Clearly separated support uses actual source-derived paragraphs, without raw Markdown tables.
 for title,files in item['appendices']:
  d.add_heading(title,level=1)
  d.add_paragraph('Отдельное приложение соответствующей методички. Полный самостоятельный экспорт '+('0104-pz01-tp.docx' if lab=='0302-pz01' else '0104-pz02-tp.docx')+' прилагается; согласие преподавателя на объединённую сдачу не предполагается.')
  for text in item['appendix_paragraphs']:d.add_paragraph(text)
 for pp in list(d.paragraphs):
  if pp.text.startswith('Окончательный report SHA'):
   pp._p.getparent().remove(pp._p)
  elif pp.text.startswith('Если отчёт включён'):
   pp.paragraph_format.keep_with_next=True
 current=list(d.paragraphs)
 for i,pp in enumerate(current[:-1]):
  if i>14 and not pp.text.strip() and current[i+1].text.startswith('СПИСОК ИСПОЛЬЗОВАННЫХ МАТЕРИАЛОВ') and not pp._p.xpath('.//w:drawing | .//w:fldChar'):
   pp._p.getparent().remove(pp._p)
 # Hyperlink actual URLs; DO NOT generate or capture images.
 link_outline(d,original,item.get('toc_pages',{}))
 for p in all_paragraphs(d):add_hyperlinks(p)
 settings=d.settings.element
 update=settings.find(qn('w:updateFields'))
 if update is None:update=OxmlElement('w:updateFields');settings.append(update)
 update.set(qn('w:val'),'true')
 d.core_properties.author='Тимофей Патрушев';d.core_properties.title={'0104-pz01':'Инструментальный контур Processing','0104-pz02':'Модульная архитектура Processing','0302-pz01':'Требования Processing и регламент изменений','0302-pz02':'Проверки финансовой допустимости Processing'}[lab]
 d.core_properties.subject='MANUAL_LAB_PR12 / HIT-12 / '+lab
 target=O/f'{lab}-tp.docx';d.save(target)
 with zipfile.ZipFile(target) as z:
  assert z.testzip() is None
  for n in z.namelist():
   if n.endswith('.xml'):etree.fromstring(z.read(n))
  assert not any(n.startswith('word/media/') for n in z.namelist())
 # XML extraction includes hyperlink text, table cells and every body story.
 check=Document(target);texts=[]
 for p in check.element.body.iter(qn('w:t')):
  if p.text:texts.append(p.text)
 full='\n'.join(texts)
 assert '{{' not in full.replace('{{.ImportPath}}','').replace('{{join .Imports " "}}',''),lab+' unresolved token'
 assert 'TEMPLATE_UNFILLED' not in full
 assert full.count('[ВСТАВИТЬ РЕАЛЬНОЕ ИЗОБРАЖЕНИЕ ВРУЧНУЮ:')==len(own)
 # Editable source module preserves ordered paragraphs and tables in XML body order.
 md='# '+d.core_properties.title+'\n\n'
 for child in check.element.body:
  if child.tag==qn('w:p'):
   vals=child.xpath('.//w:t');t=''.join(x.text or '' for x in vals)
   if t:md+=t+'\n\n'
  elif child.tag==qn('w:tbl'):
   for row in child.xpath('./w:tr'):
    cells=[''.join(x.text or '' for x in c.xpath('.//w:t')).replace('\n',' / ') for c in row.xpath('./w:tc')]
    md+='| '+' | '.join(cells)+' |\n'
   md+='\n'
 (O/f'{lab}-tp.md').write_text(md)
 with zipfile.ZipFile(source) as a,zipfile.ZipFile(target) as b:
  changed=[n for n in a.namelist() if n not in b.namelist() or a.read(n)!=b.read(n)]
 manifest.append({'lab':lab,'file':target.name,'sha256':hashlib.sha256(target.read_bytes()).hexdigest(),'template_sha256':hashlib.sha256(source.read_bytes()).hexdigest(),'figures':len(own),'tables':len(check.tables),'paragraphs':len(check.paragraphs),'sections':len(check.sections),'hyperlinks':len([r for r in check.part.rels.values() if r.reltype==RT.HYPERLINK]),'changed_package_parts':changed,'text_state':'CONTENT_READY_IMAGES_PENDING','human_gates':'PENDING','visual_qa':'PENDING_CURRENT_RENDER','accepted_code_sha':'ec0dcba7ca42ea59909489cef9650c6edff12f35','outline_pages':item.get('toc_pages',{})})
if selected:
 previous.update({row['lab']:row for row in manifest});manifest=[previous[lab] for lab in spec]
(O/'report-manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2))
print(json.dumps(manifest,ensure_ascii=False))
