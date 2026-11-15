"""Independent pinned OAS validation of actual HIT-100 private HTTPS captures."""
import argparse,datetime,hashlib,json,re,uuid,copy
p=argparse.ArgumentParser();p.add_argument('--oas',required=True);p.add_argument('--captures',required=True);a=p.parse_args();raw=open(a.oas,'rb').read();assert hashlib.sha256(raw).hexdigest()=='a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e';schemas=json.loads(raw)['components']['schemas']
def valid(x,s):
 if '$ref' in s:return valid(x,schemas[s['$ref'].split('/')[-1]])
 if x is None:return False
 if 'const' in s and (type(x)!=type(s['const']) or x!=s['const']):return False
 if 'enum' in s and x not in s['enum']:return False
 typ=s['type']
 if typ=='object':return isinstance(x,dict) and all(k in x for k in s.get('required',[])) and all(k in s['properties'] and valid(v,s['properties'][k]) for k,v in x.items())
 if typ=='array':return isinstance(x,list) and s.get('minItems',0)<=len(x)<=s.get('maxItems',len(x)) and (not s.get('uniqueItems') or len({json.dumps(v,sort_keys=True) for v in x})==len(x)) and all(valid(v,s['items']) for v in x)
 if typ=='integer':return isinstance(x,int) and not isinstance(x,bool) and s.get('minimum',x)<=x<=s.get('maximum',x)
 if typ=='boolean':return isinstance(x,bool)
 if typ=='string':
  if not isinstance(x,str) or not s.get('minLength',0)<=len(x)<=s.get('maxLength',len(x)):return False
  if 'pattern' in s and re.search(s['pattern'],x) is None:return False
  if s.get('format')=='uuid':
   try:uuid.UUID(x)
   except ValueError:return False
  if s.get('format')=='date-time':
   try:
    if datetime.datetime.fromisoformat(x).tzinfo is None:return False
   except ValueError:return False
  return True
 raise AssertionError('unsupported schema type '+typ)
negative=0
def sensitivity(body,s):
 global negative
 if '$ref' in s:s=schemas[s['$ref'].split('/')[-1]]
 assert valid(body,s),(s,body)
 variants=[None]
 if s['type']=='object':
  for k in s.get('required',[]):
   x=dict(body);del x[k];variants.append(x)
  variants.append({**body,'UNEXPECTED':0})
  for k,v in body.items():
   sensitivity(v,s['properties'][k]);variants.append({**body,k:None})
 elif s['type']=='array':
  for v in body:sensitivity(v,s['items'])
  if 'minItems' in s and s['minItems']>0:variants.append([])
  if 'maxItems' in s and body:variants.append([body[0]]*(s['maxItems']+1))
 elif s['type']=='boolean':variants.append(0)
 elif s['type']=='integer':
  variants+=[False,1.5]
  if 'minimum' in s:variants.append(s['minimum']-1)
  if 'maximum' in s:variants.append(s['maximum']+1)
 elif s['type']=='string':
  if s.get('minLength',0)>0:variants.append('')
  if 'maxLength' in s:variants.append('x'*(s['maxLength']+1))
  if 'enum' in s:variants.append('UNRECOGNIZED')
 for x in variants:assert not valid(x,s),(s,x);negative+=1
mapping={'saveDraft':('DraftWrite','DraftConfig'),'getDraft':(None,'DraftConfig'),'checkDraftReadiness':('CommandMeta','ReadinessCheck'),'rehearseDraft':('RehearsalRequest','RehearsalResult')}
rows=[json.loads(l) for l in open(a.captures)];operations=set();outcomes=set();positive=negative_checks=0
for row in rows:
 req,res=mapping[row['operation']];status=row['status'];operations.add(row['operation'])
 sensitivity(row['response'],schemas[res if status<300 else 'Error'])
 if status<300:
  if req:sensitivity(row['request'],schemas[req])
  if res=='ReadinessCheck':
   c=row['response'];positive+=c['passed'];negative_checks+=not c['passed'];assert (c['passed'] and not c['issues']) or (not c['passed'] and c['issues'])
  if res=='RehearsalResult':outcomes.update(row['response']['results']);assert row['response']['live_requests_made']==0
assert len(operations)==4 and outcomes=={'WOULD_CONFIRM','WOULD_WAIT','WOULD_EXCLUDE','WOULD_HOLD_UNKNOWN'} and positive and negative_checks
print(json.dumps({'result':'PASS','records':len(rows),'operations':sorted(operations),'synthetic_outcomes':sorted(outcomes),'positive_readiness_captures':positive,'honest_negative_readiness_captures':negative_checks,'independent_negative_sensitivity_cases':negative,'oas_sha256':hashlib.sha256(raw).hexdigest(),'qualification':'actual local PG18.7 private mTLS captures; controlled authority/binding ports, no live provider or activation'},ensure_ascii=False))
