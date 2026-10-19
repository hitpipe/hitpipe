import sys,json,re,os
sys.path.insert(0,'/tmp');from hitpipe_dk_review_runner import *
P=E/'probes';P.mkdir(exist_ok=True);env=json.loads((E/'check-env.json').read_text());binary=C/'.pr12-build/iam/release/iam-contractcheck';base=(C/'services/iam/testdata/valid.json').read_text();cases=[('canonical',base,0,'session_valid')]
for key,bad in [('version','VERSION'),('session','SESSION'),('clock','CLOCK'),('revoked','REVOKED'),('found','FOUND'),('trusted','TRUSTED')]:
 pattern=r'"'+key+r'"\s*:\s*';m=re.search(pattern,base)
 if key in ['session','clock']:v='{}'
 elif key=='version':v='1'
 else:v='false' if key=='revoked' else 'true'
 for order in ['before','after']:
  changed=base[:m.start()]+('"'+bad+'":'+v+',' if order=='before' else '')+base[m.start():]
  if order=='after':changed=base[:m.start()]+'"'+bad+'":'+v+','+base[m.start():] # both insertions are subsequently put in reverse order by replacing canonical lexical key
  if order=='after':changed=changed.replace('"'+bad+'":'+v+',','',1).replace('"'+key+'"','"'+bad+'"',1);changed=changed[:m.start()]+'"'+key+'":'+v+','+changed[m.start():]
  cases.append((key+'-'+order,changed,2,None))
cases += [('duplicate-revoked',base.replace('"revoked": false','"revoked":true,"revoked":false'),2,None),('trailing',base+'{}',2,None),('null-flag',base.replace('"revoked": false','"revoked":null'),2,None),('unknown-key',base.replace('"version": 1','"version":1,"alien":false'),2,None),('numeric-float-version',base.replace('"version": 1','"version":1.0'),2,None),('numeric-large-version',base.replace('"version": 1','"version":9007199254740993'),2,None),('oversize',base+' '*65536,2,None),('idle-equality',base.replace('07:59:59','08:00:00'),0,'idle_expired')]
# Derive equality explicitly from the parsed fixture, independent of its original clock time.
parsed=json.loads(base);parsed['clock']['now']=parsed['session']['idle_expires_at'];cases[-1]=('idle-equality',json.dumps(parsed),0,'idle_expired')
res=[]
for name,data,want,reason in cases:
 p=P/(name+'.json');p.write_text(data);rec,out,err=run('probe-cli-'+name,[str(binary),'--fixture',str(p),'--format','json'],env=env);okay=rec['exit']==want and (reason is None or json.loads(out)['reason']==reason);res.append({'id':name,'expected_exit':want,'expected_reason':reason,'actual_exit':rec['exit'],'result':'PASS' if okay else 'FAIL','fixture':str(p.relative_to(E)),'sha256':sha(p)})
 assert okay,(name,want,rec['exit'],out,err)
(E/'probes-result.json').write_text(json.dumps(res,indent=2)+'\n');print(len(res),'raw-input probes passed')
