#!/usr/bin/env python3
"""Generate exact Draft API DTO/schema subset from accepted OAS, without changing OAS."""
import argparse,hashlib,json,pathlib,subprocess
p=argparse.ArgumentParser();p.add_argument('--oas',type=pathlib.Path,required=True);p.add_argument('--gofmt',required=True);a=p.parse_args();raw=a.oas.read_bytes();assert hashlib.sha256(raw).hexdigest()=='e10177793312cd80224c461113ccab3fdc3f8af82fe0d08ce77513bcd5a2e31e';o=json.loads(raw);names=['CommandMeta','DraftConfig','DraftWrite','Error','ReadinessCheck','ScenarioExample','RehearsalRequest','RehearsalResult','ActivationCommand','Operation','CanonicalAcceptedConfig','ActivationPreview','Operations','LifecycleCommand','ResumeSnapshot'];module=pathlib.Path(__file__).resolve().parents[2];lines=['// Code generated from exact amended OAS aa380880c43d8045a851d3c939d0cd0a879b7484 (HP-CCR-ACTIVATION-20261116); DO NOT EDIT.','package drafts','import "time"']
def typ(s):
 if 'anyOf' in s:
  assert len(s['anyOf'])==2 and s['anyOf'][1]=={'type':'null'}
  return '*'+typ(s['anyOf'][0])
 if '$ref' in s:return s['$ref'].split('/')[-1]
 if s['type']=='array':return '[]'+typ(s['items'])
 if s.get('format')=='date-time':return 'time.Time'
 return {'string':'string','integer':'int64','boolean':'bool'}[s['type']]
for name in names:
 s=o['components']['schemas'][name];lines+=['type '+name+' struct {']
 for k,v in s['properties'].items():
  go=''.join(x.upper() if x in ['id','url','csrf'] else x.title() for x in k.split('_'));t=typ(v);optional=k not in s.get('required',[])
  lines+=[go+' '+('*' if optional else '')+t+' `json:"'+k+(',omitempty' if optional else '')+'"`']
 lines+=['}']
f=module/'internal/application/drafts/wire_generated.go';f.write_text('\n'.join(lines)+'\n');subprocess.run([a.gofmt,'-w',str(f)],check=True);(module/'internal/adapters/draftjson/schemas.json').write_text(json.dumps({k:o['components']['schemas'][k] for k in names},ensure_ascii=False,indent=2)+'\n')
