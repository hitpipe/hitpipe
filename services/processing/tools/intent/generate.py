#!/usr/bin/env python3
"""Generate only DeliveryIntent/ContextField DTOs from the exact accepted OAS."""
import argparse
import hashlib
import json
import pathlib
import subprocess

SHA='a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e'
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--oas',type=pathlib.Path,required=True);p.add_argument('--gofmt',required=True);a=p.parse_args()
raw=a.oas.read_bytes()
if hashlib.sha256(raw).hexdigest()!=SHA:raise SystemExit('unaccepted OAS bytes')
schema=json.loads(raw)
lines=['// Code generated from pinned mvp-openapi.json fedbe40bb597e25f66aeb1172af3ff7459c81b40; DO NOT EDIT.','package preparecase','import "time"']
for name in ['ContextField','DeliveryIntent']:
 lines.append('type '+name+' struct {')
 for key,s in schema['components']['schemas'][name]['properties'].items():
  typ={'string':'string','integer':'int64','array':'[]ContextField'}[s['type']]
  if s.get('format')=='date-time':typ='time.Time'
  go=''.join(x[:1].upper()+x[1:] for x in key.split('_'))
  lines.append(go+' '+typ+' `json:"'+key+'"`')
 lines.append('}')
out=pathlib.Path(__file__).resolve().parents[2]/'internal/application/preparecase/wire_generated.go'
out.write_text('\n'.join(lines)+'\n');subprocess.run([a.gofmt,'-w',str(out)],check=True)
