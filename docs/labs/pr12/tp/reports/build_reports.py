from pathlib import Path
import copy,json,re,zipfile,hashlib
from lxml import etree
from docx import Document
from docx.oxml import OxmlElement
from docx.oxml.ns import qn
from docx.shared import Pt
from docx.enum.table import WD_TABLE_ALIGNMENT
from docx.opc.constants import RELATIONSHIP_TYPE as RT
here=Path(__file__).resolve().parent
R=here/'hitpipe' if (here/'hitpipe').is_dir() else next(p for p in here.parents if (p/'docs/labs/pr12/manifest.json').is_file())
B=R/'docs/labs/pr12';P=B/'tp';O=P/'reports'
spec=json.loads((O/'report-content.json').read_text());fig=json.loads((P/'evidence/screenshots/instructions.json').read_text())
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
 # Rebuild editable body text as real hyperlink runs; preserve paragraph geometry.
 p.clear();pos=0
 for m in re.finditer(r'https?://[^\s;]+',text):
  url=m.group().rstrip('.,)');end=m.start()+len(url)
  p.add_run(text[pos:m.start()])
  rel=p.part.relate_to(url,RT.HYPERLINK,is_external=True);h=OxmlElement('w:hyperlink');h.set(qn('r:id'),rel)
  r=OxmlElement('w:r');pr=OxmlElement('w:rPr');style=OxmlElement('w:rStyle');style.set(qn('w:val'),'Hyperlink');pr.append(style);r.append(pr)
  t=OxmlElement('w:t');t.text=url;r.append(t);h.append(r);p._p.append(h);pos=end
 p.add_run(text[pos:])

def all_paragraphs(d):
 yield from d.paragraphs
 for t in d.tables:
  for row in t.rows:
   for c in row.cells:yield from c.paragraphs

for lab,item in spec.items():
 source=B/'templates'/f'{lab}-report-template.docx';d=Document(source);original=list(d.paragraphs)
 for i,text in item['paragraph_updates'].items():replace_para(original[int(i)],text)
 replace_para(original[1],'КАФЕДРА № (не предоставлена)')
 replace_para(original[13],'Санкт-Петербург\n2026')
 # Student name only; signature and teacher grade remain untouched and empty.
 d.tables[2].cell(0,0).text='Студент гр. № (не предоставлена)';d.tables[2].cell(0,5).text='Тимофей Патрушев'
 for i,rows in item['table_rows'].items():fill_table(d.tables[int(i)],rows)
 slots=[(i,p) for i,p in enumerate(original) if '[ВСТАВИТЬ РЕАЛЬНОЕ ИЗОБРАЖЕНИЕ ВРУЧНУЮ:' in p.text]
 own=[r for r in fig if r['lab']==lab];assert len(slots)==len(own)
 for (index,p),row in zip(slots,own):
  p.text='[ВСТАВИТЬ РЕАЛЬНОЕ ИЗОБРАЖЕНИЕ ВРУЧНУЮ: '+row['target_file'].split('/')[-1]+']'
  original[index+1].text='SOURCE: '+row['source']+'\nSTATE: '+row['state']+'\nCWD: '+row['cwd']+'\nCOMMAND: '+row['command']+'\nSTEPS: '+row['steps']
  original[index+2].text='MUST BE VISIBLE: '+row['must_be_visible']+'\nREDACT: '+row['redact']+'\nVERIFY: '+row['verify_before_insert']
  original[index+3].text='TARGET: '+row['target_file']+'\nCAPTION: '+row['caption']+'\nINSERT AT: '+row['insert_at']
 for p in original:
  if p.text.startswith('Каждый слот заполнить'):
   p.text='Ниже точные инструкции человеку. Изображения не создавались и не снимались агентом. Источники, actual status и версии сохранены; ещё не состоявшиеся gates выполняются до снимка. Placeholder не подтверждает PASS.'
  if p.style.name.lower().startswith('toc'):
   p.text=re.sub(r'\t\d+$','',p.text)
 # Clearly separated support uses actual source-derived paragraphs, without raw Markdown tables.
 for title,files in item['appendices']:
  d.add_heading(title,level=1)
  d.add_paragraph('Отдельное приложение соответствующей методички. Полный самостоятельный экспорт '+('0104-pz01-tp.docx' if lab=='0302-pz01' else '0104-pz02-tp.docx')+' прилагается; согласие преподавателя на объединённую сдачу не предполагается.')
  if lab=='0302-pz01':
   parts=[
    'Собственный scaffold HIT-27 — конфигурация/metadata/safe diagnostic и retained pure policy; PR5. Общий LAB-STACK-001 переиспользован по pin. Toolchain: build Go1.27.2; dependencies stdlib/go.mod; tests Go testing; static vet/import guard; dynamic race/diagnostics реально работают. Delve и profiling pprof запланированы для следующей исследовательской работы. GitHub Actions Ubuntu24.04 выполняет named tests/smoke/race/mutations; actual CI URL и SHA в основном тексте.',
    'Путь: source+go.mod → Go compiler → linker с version/commit/dirty/mode → OS loader/system dylibs → fixture → application → domain → result. Mac inspection показал libSystem/libresolv, поэтому полностью статический binary не заявлен. Dev -N -l и release -trimpath сравнивались на source8ae5d67b9eb0: 3948210/3909730 bytes, тот же normal/refund/invalid scenario. Оба binary реально исполнялись вне checkout; fixed input valid→VALID0, refund→INELIGIBLE0, malformed→exit2. Timing не измерялся.',
    'Содержательная CI sensitivity: M01 нарушает refund predicate, компилируется, тот же wrapper возвращает nonzero. После restoration clean green. Реальное независимое clean-room воспроизведение другой студент ещё не выполнил. Полные семь позиций и наблюдения: toolchain.md, execution-path.md, evidence/d1; 0104-pz01 отдельный DOCX содержит семь точных image slots.'
   ]
  else:
   parts=[
    'RULE006 → eligibility.Evaluate → application EvaluatePaidOrder → retained CLI. Публичный контракт определяет nullable inputs, supported schema/evaluator, local typed outputs, precedence, borrowed read-only ownership, owned result strings и отсутствие IO/state в domain. One SourceFactsReader read, no retry; pin/identity mismatch и failures удерживают действие. Same consumer с File и finite double исполнял normal/error/expiry boundary; real Intake authenticity этим не доказана.',
    'Личная запись module-map.md: cmd→application/adapters/domain; sourcefacts→application/input; input→domain; application→domain. A→B означает исходный import; guard/go list подтверждают отсутствие production cycles. Future Intake/transport/storage/authority остаются планом, чужая реализация не приписана. Автор записи Тимофей с Codex; Lead интегрирует её в общую карту.',
    'Два направления сложности: nullable/exact financial data и bounded snapshot interaction. Кандидат уже существует и наблюдается отдельно на valid.json. TP ADR001 сравнивает handler, shared business library и service-local pure policy/consumer; выбран последний. Цена: Query/Snapshot/Request, mapping/copy/lifetime и interface indirection. Проверки — contract DI, pin/unknown/boundary/imports; performance/bottleneck не измерены. module-adr.md и complexity-profile.md содержат полное решение. Взаимные student checks HIT-31 ещё pending; отдельный 0104-pz02 DOCX содержит шесть image slots.'
   ]
  for text in parts:d.add_paragraph(text)
 for pp in list(d.paragraphs):
  if pp.text.startswith('Окончательный report SHA'):
   pp._p.getparent().remove(pp._p)
  elif pp.text.startswith('Если отчёт включён'):
   pp.paragraph_format.keep_with_next=True
 # Hyperlink actual URLs; DO NOT generate or capture images.
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
 manifest.append({'lab':lab,'file':target.name,'sha256':hashlib.sha256(target.read_bytes()).hexdigest(),'template_sha256':hashlib.sha256(source.read_bytes()).hexdigest(),'figures':len(own),'tables':len(check.tables),'paragraphs':len(check.paragraphs),'sections':len(check.sections),'hyperlinks':len([r for r in check.part.rels.values() if r.reltype==RT.HYPERLINK]),'changed_package_parts':changed,'text_state':'CONTENT_READY_IMAGES_PENDING','human_gates':'PENDING','visual_qa':'NOT_PERFORMED_NO_IMAGE_GENERATION_BY_USER_REQUEST'})
(O/'report-manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2))
print(json.dumps(manifest,ensure_ascii=False))
