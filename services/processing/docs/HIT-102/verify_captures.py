"""Independently validate real CCR2 captures against the exact amended OAS."""
import argparse,datetime,hashlib,json,re,uuid
p=argparse.ArgumentParser();p.add_argument('--oas',required=True);p.add_argument('--activation-captures',required=True);p.add_argument('--draft-captures',required=True);p.add_argument('--pipeline');a=p.parse_args();raw=open(a.oas,'rb').read();assert hashlib.sha256(raw).hexdigest()=='e10177793312cd80224c461113ccab3fdc3f8af82fe0d08ce77513bcd5a2e31e';schemas=json.loads(raw)['components']['schemas']
def valid(v,s):
 allowed={'$ref','type','description','additionalProperties','properties','required','minLength','maxLength','format','pattern','minimum','maximum','const','enum','items','maxItems','minItems','uniqueItems','anyOf'}
 assert not set(s)-allowed,('unsupported schema keywords',set(s)-allowed)
 if '$ref'in s:return valid(v,schemas[s['$ref'].rsplit('/',1)[1]])
 if 'anyOf'in s:return any(valid(v,x)for x in s['anyOf'])
 t=s['type']
 if t=='null':return v is None
 if v is None:return False
 if 'const'in s and (type(v)is not type(s['const'])or v!=s['const']):return False
 if 'enum'in s and v not in s['enum']:return False
 if t=='object':return isinstance(v,dict)and all(k in v for k in s.get('required',[]))and all(k in s['properties']and valid(x,s['properties'][k])for k,x in v.items())
 if t=='array':return isinstance(v,list)and s.get('minItems',0)<=len(v)<=s.get('maxItems',len(v))and (not s.get('uniqueItems')or len({json.dumps(x,sort_keys=True,ensure_ascii=False)for x in v})==len(v))and all(valid(x,s['items'])for x in v)
 if t=='boolean':return type(v)is bool
 if t=='integer':return type(v)is int and s.get('minimum',v)<=v<=s.get('maximum',v)
 if t=='string':
  if not isinstance(v,str)or not s.get('minLength',0)<=len(v)<=s.get('maxLength',len(v)):return False
  if 'pattern'in s and re.search(s['pattern'],v)is None:return False
  if s.get('format')=='uuid':
   try:uuid.UUID(v)
   except ValueError:return False
  elif s.get('format')=='date-time':
   try:
    if datetime.datetime.fromisoformat(v).tzinfo is None:return False
   except ValueError:return False
  elif 'format'in s:raise AssertionError('unsupported format '+s['format'])
  return True
 raise AssertionError('unsupported type '+t)
def canonical_digest(config):
 # The exact canonical schema has ASCII keys and only strings/string arrays;
 # canonical JSON here equals RFC8785 without claiming a general number codec.
 assert len(config)==11 and all(k.isascii()for k in config)
 assert all(isinstance(v,str)or isinstance(v,list)and all(isinstance(x,str)for x in v)for v in config.values())
 b=json.dumps(config,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()
 return 'sha256:'+hashlib.sha256(b).hexdigest()
def preview(check):
 assert check['passed']==(check['activation_preview']is not None)
 if check['passed']:
  p=check['activation_preview'];assert p['accepted_config_digest']==canonical_digest(p['canonical_config'])
  assert not any(k in p for k in ['BindingProof','session','epoch','principal_id'])
  assert check['config_digest']!=p['accepted_config_digest']
rows=[json.loads(l)for l in open(a.activation_captures)];assert rows;counts={}
for r in rows:
 status=r['status'];counts[str(status)]=counts.get(str(status),0)+1
 assert valid(r['response'],schemas['Operation'if status==201 else 'Error']),(r['scenario'],r['response'])
 if status==201:assert valid(r['request'],schemas['ActivationCommand'])and r['request']['accepted_config_digest']==r['response']['accepted_config_digest']
 assert not(status>=400 and 'operation_id'in r['response'])
mapping={'saveDraft':('DraftWrite','DraftConfig'),'getDraft':(None,'DraftConfig'),'checkDraftReadiness':('CommandMeta','ReadinessCheck'),'rehearseDraft':('RehearsalRequest','RehearsalResult')}
drafts=[json.loads(l)for l in open(a.draft_captures)];checks=0;rehearsals=0;requests=0
for r in drafts:
 req,res=mapping[r['operation']];status=r['status'];assert valid(r['response'],schemas[res if status<300 else 'Error']),(r['scenario'],res)
 if status<300:
  if req and r['request']is not None:assert valid(r['request'],schemas[req]);requests+=1
  if res=='ReadinessCheck':preview(r['response']);checks+=1
  if res=='RehearsalResult':assert r['response']['live_requests_made']==0;rehearsals+=1
pipeline='not_requested'
if a.pipeline:
 g=json.load(open(a.pipeline));mapping={'readiness':'ReadinessCheck','rehearsal_request':'RehearsalRequest','rehearsal':'RehearsalResult','activation_request':'ActivationCommand','operation':'Operation','listCases':'Cases','getCase':'CaseDetail','getCaseHistory':'History'}
 for key,schema in mapping.items():assert valid(g[key],schemas[schema]),(key,schema)
 preview(g['readiness']);check=g['readiness'];req=g['activation_request'];rr=g['rehearsal']
 assert g['rehearsal_request']['readiness_check_id']==check['check_id']==req['readiness_check_id']
 assert rr['rehearsal_id']==req['rehearsal_id']and rr['config_digest']==check['config_digest']
 final=check['activation_preview']['accepted_config_digest'];assert final==req['accepted_config_digest']==g['operation']['accepted_config_digest']==g['sql_canonical_digest']
 assert len(g['listCases']['items'])==1 and len(g['getCaseHistory']['items'])>0
 assert g['getCase']['case']['config_digest']==final
 pipeline='PASS actual public readiness -> selected rehearsal -> activation201 -> Intake202 -> nonempty list/get/history200; experimental stack, not merged main'
print(json.dumps({'result':'PASS','activation_http_records':len(rows),'status_counts':counts,'draft_http_records':len(drafts),'readiness_checks_validated':checks,'rehearsals_validated':rehearsals,'available_successful_draft_requests_validated':requests,'pipeline':pipeline,'oas_sha256':hashlib.sha256(raw).hexdigest(),'qualification':'Actual private TLS1.3/mTLS owned PG18.7/files. Controlled owners, no live IAM/BFF/provider deployment or human acceptance claim.'},ensure_ascii=False))
