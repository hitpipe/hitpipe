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
rows=[json.loads(l) for l in open(a.captures)];statuses=set()
for row in rows:
 assert row['operation']=='receiveNormalizedSignal' and row['actual_pg187_mtls']
 statuses.add(row['status']);sensitivity(row['response'],schemas[row['schema']])
 if row['status']==202:
  sensitivity(row['request'],schemas['NormalizedSignal'])
  assert row['request']['message_id']==row['response']['message_id']
  assert row['response']['disposition']=='APPLIED' and 'revision' not in row['response']
assert statuses=={202,400,403,404,409,412,413,503},statuses
print(json.dumps({'result':'PASS','actual_http_captures':len(rows),'statuses':sorted(statuses),'negative_schema_sensitivity_checks':negative,'oas_sha256':hashlib.sha256(raw).hexdigest(),'scope':'actual controlled Intake private HTTP/mTLS plus actual owned PG18.7. Not production integration.'},ensure_ascii=False))
