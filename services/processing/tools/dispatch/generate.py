#!/usr/bin/env python3
"""Extract three exact accepted schemas; never regenerate the financial module."""
import argparse,hashlib,json,pathlib
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--oas',type=pathlib.Path,required=True);a=p.parse_args();raw=a.oas.read_bytes()
if hashlib.sha256(raw).hexdigest()!='a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e':raise SystemExit('Unaccepted OAS')
o=json.loads(raw);out=pathlib.Path(__file__).resolve().parents[2]/'internal/adapters/deliveryhttp/schemas.json';out.write_text(json.dumps({k:o['components']['schemas'][k] for k in ['DeliveryIntent','ContextField','Receipt']},ensure_ascii=False,indent=2)+'\n')
