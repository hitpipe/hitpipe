"""Validate actual captured responses against the exact accepted OAS subset.
Stdlib only; unsupported schema keywords fail rather than silently being ignored.
Not a claim of implementing a general JSON Schema validator.
"""
import datetime
import hashlib
import json
import pathlib
import re
import sys
import uuid
import urllib.parse

raw=pathlib.Path(sys.argv[1]).read_bytes()
assert hashlib.sha256(raw).hexdigest()=='a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e','wrong accepted OAS'
oas=json.loads(raw)

def check(value,s,path='$'):
 allowed={'$ref','anyOf','type','additionalProperties','properties','required','items','maxItems','minItems','enum','format','pattern','minLength','maxLength','minimum','maximum','description','title'}
 assert set(s)<=allowed,(path,'unsupported schema keyword',set(s)-allowed)
 if '$ref' in s:
  assert s['$ref'].startswith('#/components/schemas/')
  return check(value,oas['components']['schemas'][s['$ref'].split('/')[-1]],path)
 if 'anyOf' in s:
  for v in s['anyOf']:
   try:check(value,v,path);return
   except AssertionError:pass
  raise AssertionError((path,'no anyOf variant'))
 kind=s.get('type')
 if kind=='object':
  assert isinstance(value,dict),(path,'object required')
  assert set(s.get('required',[]))<=set(value),(path,'missing required')
  if s.get('additionalProperties') is False:assert set(value)<=set(s['properties']),(path,'unknown fields')
  for k,v in value.items():check(v,s['properties'][k],path+'.'+k)
 elif kind=='array':
  assert isinstance(value,list),(path,'array required')
  assert s.get('minItems',0)<=len(value)<=s.get('maxItems',10**9),(path,'array bound')
  for i,v in enumerate(value):check(v,s['items'],path+f'[{i}]')
 elif kind=='string':
  assert isinstance(value,str),(path,'string required')
  assert s.get('minLength',0)<=len(value)<=s.get('maxLength',10**9),(path,'length')
  if 'pattern' in s:assert re.search(s['pattern'],value),(path,'pattern')
  if s.get('format')=='uuid':
   assert re.fullmatch(r'[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}',value);uuid.UUID(value)
  elif s.get('format')=='date-time':
   assert re.fullmatch(r'\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})',value),(path,'RFC3339')
   assert datetime.datetime.fromisoformat(value.replace('Z','+00:00')).tzinfo is not None
  elif s.get('format')=='uri':assert urllib.parse.urlsplit(value).scheme and not any(c.isspace() for c in value)
  else:assert 'format' not in s,(path,'unsupported string format')
 elif kind=='integer':
  assert isinstance(value,(int,float)) and not isinstance(value,bool) and int(value)==value,(path,'integer required')
  assert s.get('minimum',-10**99)<=value<=s.get('maximum',10**99),(path,'integer bound')
 elif kind=='boolean':assert isinstance(value,bool),(path,'boolean required')
 elif kind=='null':assert value is None,(path,'null required')
 else:raise AssertionError((path,'unsupported type',kind))
 if 'enum' in s:assert value in s['enum'],(path,'enum')

observations=json.loads(pathlib.Path(sys.argv[2]).read_text())
for row in observations:
 check(row['body'],oas['components']['schemas'][row['schema']])
 # Negative sensitivity check: a validator accepting a missing mandatory field
 # or a nullable omission is not evidence that the real response meets OAS.
 bad=dict(row['body']);del bad[oas['components']['schemas'][row['schema']]['required'][0]]
 try:check(bad,oas['components']['schemas'][row['schema']])
 except AssertionError:pass
 else:raise AssertionError('validator did not detect removed required field')
 print(row['operation'],row['status'],row['schema'],'exact OAS PASS')
assert any(r['schema']=='History' and r['status']==200 for r in observations)
print('Nonempty Cases/CaseDetail200 not claimed: missing approved display sources.')
