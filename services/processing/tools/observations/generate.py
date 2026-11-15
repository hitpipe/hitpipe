#!/usr/bin/env python3
"""Generate the exact pinned observation wire subset; never edit formal OAS."""
import argparse,hashlib,json,pathlib,subprocess
p=argparse.ArgumentParser();p.add_argument('--oas',type=pathlib.Path,required=True);p.add_argument('--gofmt',required=True);a=p.parse_args();raw=a.oas.read_bytes();assert hashlib.sha256(raw).hexdigest()=='a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e';schemas=json.loads(raw)['components']['schemas'];names=['TaskObservation','Receipt','Error'];module=pathlib.Path(__file__).resolve().parents[2]
def typ(s):
 if 'anyOf' in s:return '*'+typ(next(x for x in s['anyOf'] if x.get('type')!='null'))
 if '$ref' in s:return s['$ref'].split('/')[-1]
 if s.get('format')=='date-time':return 'time.Time'
 return {'string':'string','boolean':'bool','integer':'int64'}[s['type']]
lines=['// Code generated from OAS fedbe40bb597e25f66aeb1172af3ff7459c81b40; DO NOT EDIT.','package observations','import "time"']
for name in names:
 s=schemas[name];lines+=['type '+name+' struct {']
 for k,v in s['properties'].items():
  field=''.join(x.upper() if x in ['id','url','csrf'] else x.title() for x in k.split('_'));t=typ(v);optional=k not in s['required'];lines+=[field+' '+('*' if optional else '')+t+' `json:"'+k+(',omitempty' if optional else '')+'"`']
 lines+=['}']
f=module/'internal/application/observations/wire_generated.go';f.write_text('\n'.join(lines)+'\n');subprocess.run([a.gofmt,'-w',str(f)],check=True);(module/'internal/adapters/observationjson/schemas.json').write_text(json.dumps({k:schemas[k] for k in names},ensure_ascii=False,indent=2)+'\n')
