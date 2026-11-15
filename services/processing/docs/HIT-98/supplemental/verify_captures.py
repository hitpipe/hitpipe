"""Independent stdlib OAS check of actual controlled request/receipt artifacts."""
import argparse,datetime,hashlib,json,re,uuid
p=argparse.ArgumentParser();p.add_argument('--oas',required=True);p.add_argument('--captures',required=True);a=p.parse_args();raw=open(a.oas,'rb').read();assert hashlib.sha256(raw).hexdigest()=='a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e';schemas=json.loads(raw)['components']['schemas']
def valid(x,s):
 if '$ref' in s:return valid(x,schemas[s['$ref'].split('/')[-1]])
 if x is None:return False
 if 'const' in s and x!=s['const']:return False
 if 'enum' in s and x not in s['enum']:return False
 typ=s['type']
 if typ=='object':return isinstance(x,dict) and all(k in x for k in s.get('required',[])) and all(k in s['properties'] and valid(v,s['properties'][k]) for k,v in x.items())
 if typ=='array':return isinstance(x,list) and len(x)<=s.get('maxItems',len(x)) and all(valid(v,s['items']) for v in x)
 if typ=='integer':return isinstance(x,int) and not isinstance(x,bool) and s.get('minimum',x)<=x<=s.get('maximum',x)
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
rows=[json.loads(l) for l in open(a.captures)];pairs=0;negative=0;dispositions=set()
for row in rows:
 if row['scenario']!='controlled_delivery_durable_receipt':continue
 value=row['value']
 for key,schema in [('request','DeliveryIntent'),('receipt','Receipt')]:
  body=value[key];assert valid(body,schemas[schema]),(key,body)
  for req in schemas[schema]['required']:
   changed=dict(body);del changed[req];assert not valid(changed,schemas[schema]);negative+=1
  changed=dict(body);changed['UNEXPECTED']=0;assert not valid(changed,schemas[schema]);negative+=1
 assert value['request']['message_id']==value['receipt']['message_id'];pairs+=1;dispositions.add(value['receipt']['disposition'])
assert pairs==8 and dispositions=={'APPLIED','DUPLICATE','HELD','CONFLICT'}
print(json.dumps({'records':len(rows),'actual_request_receipt_pairs':pairs,'oas_schemas':['DeliveryIntent','ContextField','Receipt'],'required_extra_sensitivity_cases':negative,'dispositions':sorted(dispositions),'result':'PASS','qualification':'controlled Delivery peer only; not live IAM/Delivery/ClickUp'},ensure_ascii=False))
