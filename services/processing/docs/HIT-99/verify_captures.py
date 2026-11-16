"""Independent exact-OAS validation of actual observation and owner read captures."""
import argparse,datetime,hashlib,json,re,uuid
p=argparse.ArgumentParser();p.add_argument('--oas',required=True);p.add_argument('--captures',required=True);a=p.parse_args();raw=open(a.oas,'rb').read();assert hashlib.sha256(raw).hexdigest()=='a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e';schemas=json.loads(raw)['components']['schemas']
def valid(x,s):
 assert set(s)<={'$ref','anyOf','type','additionalProperties','properties','required','items','maxItems','minItems','enum','const','format','pattern','minLength','maxLength','minimum','maximum','description','title','uniqueItems'}
 if '$ref' in s:return valid(x,schemas[s['$ref'].split('/')[-1]])
 if 'anyOf' in s:return any(valid(x,y) for y in s['anyOf'])
 if s['type']=='null':return x is None
 if x is None:return False
 if 'const' in s and (type(x)!=type(s['const']) or x!=s['const']):return False
 if 'enum' in s and x not in s['enum']:return False
 t=s['type']
 if t=='object':return isinstance(x,dict) and all(k in x for k in s.get('required',[])) and all(k in s['properties'] and valid(v,s['properties'][k]) for k,v in x.items())
 if t=='array':return isinstance(x,list) and s.get('minItems',0)<=len(x)<=s.get('maxItems',len(x)) and all(valid(v,s['items']) for v in x)
 if t=='integer':return isinstance(x,int) and not isinstance(x,bool) and s.get('minimum',x)<=x<=s.get('maximum',x)
 if t=='boolean':return isinstance(x,bool)
 if t=='string':
  if not isinstance(x,str) or not s.get('minLength',0)<=len(x)<=s.get('maxLength',len(x)):return False
  if 'pattern' in s and re.search(s['pattern'],x) is None:return False
  if s.get('format')=='uuid':
   try:uuid.UUID(x)
   except ValueError:return False
  if s.get('format')=='date-time':
   try:
    if datetime.datetime.fromisoformat(x).tzinfo is None:return False
   except ValueError:return False
  if s.get('format')=='uri':return bool(re.match(r'^[a-zA-Z][a-zA-Z0-9+.-]*:',x)) and not any(c.isspace() for c in x)
  return True
 raise AssertionError(t)
negative=0
def sensitivity(x,s):
 global negative
 if '$ref' in s:s=schemas[s['$ref'].split('/')[-1]]
 assert valid(x,s),(x,s)
 if 'anyOf' in s:
  sensitivity(x,next(v for v in s['anyOf'] if valid(x,v)));return
 if s['type']=='null':variants=[False,0,'',{}]
 else:variants=[None]
 if s['type']=='object':
  for k in s.get('required',[]):
   y=dict(x);del y[k];variants.append(y)
  variants.append({**x,'UNEXPECTED':0})
  for k,v in x.items():sensitivity(v,s['properties'][k])
 elif s['type']=='array':
  for v in x:sensitivity(v,s['items'])
  if s.get('minItems',0)>0:variants.append([])
  if 'maxItems' in s and x:variants.append([x[0]]*(s['maxItems']+1))
 elif s['type']=='integer':
  variants.extend([False,1.5])
  if 'minimum' in s:variants.append(s['minimum']-1)
  if 'maximum' in s:variants.append(s['maximum']+1)
 elif s['type']=='boolean':variants.append(0)
 elif s['type']=='string':
  if s.get('minLength',0)>0:variants.append('')
  if 'maxLength' in s:variants.append('x'*(s['maxLength']+1))
  if 'enum' in s or 'const' in s:variants.append('UNRECOGNIZED')
 for y in variants:assert not valid(y,s),(y,s);negative+=1
rows=[json.loads(l) for l in open(a.captures)];operations=set();knowledge=set();states=set();dispositions=set();details=[]
for row in rows:
 operations.add(row['operation']);sensitivity(row['response'],schemas[row['schema']])
 if row['operation']=='applyTaskObservation' and row['status']==202:
  sensitivity(row['request'],schemas['TaskObservation']);assert row['response']['message_id']==row['request']['message_id'];knowledge.add(row['request']['knowledge']);dispositions.add(row['response']['disposition'])
 if row['schema']=='CaseDetail':
  details.append(row['response']);state=row['response']['case']['state'];states.add(state)
  if state!='CONFIRMED':assert row['response']['case']['target_url'] is None
assert operations=={'applyTaskObservation','listCases','getCase','getCaseHistory'} and knowledge=={'RESULT_VERIFIED','TARGET_ID_OBSERVED','POSSIBLE_EFFECT','RESULT_MISMATCH','INCOMPLETE','KNOWN_NO_EFFECT'} and dispositions=={'APPLIED','HELD'}
assert any(d['case']['state']=='CONFIRMED' and d['observation_complete'] and d['case']['target_url']=='https://app.clickup.com/t/86synthetic99' for d in details) and any(d['case']['state']=='UNKNOWN' and not d['observation_complete'] for d in details)
print(json.dumps({'result':'PASS','actual_captures':len(rows),'operations':sorted(operations),'knowledge_inputs':sorted(knowledge),'durable_receipt_dispositions':sorted(dispositions),'observed_case_states':sorted(states),'independent_negative_schema_sensitivity_cases':negative,'oas_sha256':hashlib.sha256(raw).hexdigest(),'scope':'actual local PG18.7 Processing HTTPS/mTLS and HIT95 read API; controlled Delivery/authority, not live IAM/Delivery/ClickUp or fetched foreign evidence'},ensure_ascii=False))
