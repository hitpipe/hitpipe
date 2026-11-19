#!/usr/bin/env python3
"""Generate only Go representations of the exact extracted OAS schemas."""
import json,pathlib,argparse,subprocess
p=argparse.ArgumentParser();p.add_argument('--check',action='store_true');p.add_argument('--gofmt',default='gofmt');a=p.parse_args()
root=pathlib.Path(__file__).resolve().parents[1]/'internal/domain/sourcefacts'
s=json.loads((root/'schema.json').read_text());names={'SourceReadRequest':'Request','SourceFacts':'Facts'}
def camel(s):return ''.join({'id':'ID'}.get(p,p.title()) for p in s.split('_'))
lines=['// Code generated from schema.json (pinned MVP OAS); DO NOT EDIT.','package sourcefacts','import "time"']
for name,go in names.items():
 lines+=['type '+go+' struct {']
 for field,r in s[name]['properties'].items():
  nullable='anyOf' in r
  if nullable:r=r['anyOf'][0]
  typ={'boolean':'bool','integer':'int64','string':'string'}[r['type']]
  if r.get('format')=='date-time':typ='time.Time'
  lines+=[' '+camel(field)+' '+('*' if nullable else '')+typ+' `json:"'+field+'"`']
 lines+=['}']
b=subprocess.run([a.gofmt],input='\n'.join(lines)+'\n',text=True,capture_output=True,check=True).stdout
if a.check:
 assert (root/'types_generated.go').read_text()==b, 'generated types drift'
else: (root/'types_generated.go').write_text(b)
