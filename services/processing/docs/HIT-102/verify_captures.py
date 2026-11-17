"""Independent pinned OAS validation of actual HIT-102 private HTTPS captures."""
import argparse,datetime,hashlib,json,re,uuid,copy
p=argparse.ArgumentParser();p.add_argument('--oas',required=True);p.add_argument('--captures',required=True);a=p.parse_args();raw=open(a.oas,'rb').read();assert hashlib.sha256(raw).hexdigest()=='a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e';schemas=json.loads(raw)['components']['schemas']
def valid(x,s):
 if '$ref' in s:return valid(x,schemas[s['$ref'].split('/')[-1]])
 if "anyOf" in s:return any(valid(x,z) for z in s["anyOf"])
 if s.get("type")=="null":return x is None
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
rows=[json.loads(l) for l in open(a.captures)]
assert rows
statuses={}
for row in rows:
 assert row['operation']=='activateOperation'
 status=row['status'];statuses[status]=statuses.get(status,0)+1
 schema='Operation' if status==201 else 'Error'
 assert valid(row['response'],schemas[schema]),(row['scenario'],schema,row['response'])
 if status==201:assert valid(row['request'],schemas['ActivationCommand'])
 if status>=400:assert 'operation_id' not in row['response'] and 'accepted_config_id' not in row['response']
assert 201 in statuses and all(s in statuses for s in [400,401,403,409,412,413,503])
print(json.dumps({'result':'PASS','actual_http_records':len(rows),'status_counts':statuses,'oas_sha256':hashlib.sha256(raw).hexdigest(),'scope':'Exact accepted OAS validation of recorded actual PG18.7/mTLS activation responses and successful requests. Controlled session/authority/bindings. Not live IAM/BFF/providers or full stack acceptance.'},ensure_ascii=False))
