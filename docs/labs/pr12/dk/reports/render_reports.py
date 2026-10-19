#!/usr/bin/env python3
"""Update real Writer indexes, persist their DOCX results and export QA PDFs.

Requires LibreOffice Writer, its matching Python UNO bridge and Poppler.
This renders report pages for layout QA; it never fills screenshot slots.
Run after build_reports.py; rendering is not a human academic acceptance.
"""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import uuid

import uno
from com.sun.star.beans import PropertyValue

import build_reports


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def properties(**values):
    return tuple(PropertyValue(Name=name, Value=value) for name, value in values.items())


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--soffice',default='soffice')
    parser.add_argument('--pdfinfo',default='pdfinfo')
    parser.add_argument('--pdftotext',default='pdftotext')
    parser.add_argument('--output',type=Path,required=True)
    args=parser.parse_args()
    output=args.output.resolve();output.mkdir(parents=True,exist_ok=True)
    started=datetime.now(timezone.utc).isoformat()
    # UNO's native bootstrap may set process-local C environment entries which
    # are not reflected in os.environ; do not leak those into another bootstrap.
    child_env=dict(os.environ)
    versions={name:subprocess.run(command,capture_output=True,text=True,check=True,env=child_env).stdout.strip()
              for name,command in [('libreoffice',[args.soffice,'--version'])]}
    result={'started_at_utc':started,'argv':sys.argv,'cwd':str(Path.cwd()),
            'source_base_sha':subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(),
            'environment':{k:os.environ.get(k) for k in ['LANG','LC_ALL','FONTCONFIG_FILE','SAL_USE_VCLPLUGIN','PYTHONPATH','LD_LIBRARY_PATH','URE_BOOTSTRAP','UNO_TYPES']},
            'tool_versions':versions,'reports':[],'visual_inspection':'PENDING',
            'student_screenshots_generated':False,'submission_ready':False}
    with tempfile.TemporaryDirectory(prefix='dk-writer-') as temporary:
        pipe='dk_'+uuid.uuid4().hex
        command=[args.soffice,'--headless','--nologo','--nodefault','--norestore','--nofirststartwizard',
                 '-env:UserInstallation='+Path(temporary).as_uri(),
                 '--accept=pipe,name='+pipe+';urp;StarOffice.ServiceManager']
        result['writer_command']=command
        with (output/'writer.log').open('w') as log:
            process=subprocess.Popen(command,stdout=log,stderr=subprocess.STDOUT,env=child_env)
            desktop=None;document=None
            try:
                local=uno.getComponentContext()
                resolver=local.ServiceManager.createInstanceWithContext('com.sun.star.bridge.UnoUrlResolver',local)
                for attempt in range(120):
                    try:
                        context=resolver.resolve('uno:pipe,name='+pipe+';urp;StarOffice.ComponentContext')
                        break
                    except Exception:
                        if process.poll() is not None:raise RuntimeError('Writer exited; inspect writer.log')
                        time.sleep(0.25)
                else:raise RuntimeError('Writer UNO connection timed out')
                desktop=context.ServiceManager.createInstanceWithContext('com.sun.star.frame.Desktop',context)
                for path in sorted(build_reports.HERE.glob('*-dk.docx')):
                    before=sha(path)
                    load=properties(Hidden=True,ReadOnly=False,Silent=True,UpdateDocMode=0,MacroExecutionMode=0)
                    document=desktop.loadComponentFromURL(path.as_uri(),'_blank',0,load)
                    if document is None:raise RuntimeError('Writer could not open '+str(path))
                    indexes=document.getDocumentIndexes()
                    expected=1 if path.name.startswith('0302') else 0
                    if expected and indexes.Count==0:
                        # Writer does not import an empty Word fldSimple TOC as
                        # an index. Replace its explicit seed with a real one.
                        search=document.createSearchDescriptor()
                        search.SearchString='[ОГЛАВЛЕНИЕ: ОБНОВИТЬ ЧЕРЕЗ render_reports.py]'
                        anchor=document.findFirst(search)
                        if anchor is None:raise RuntimeError('TOC seed missing: '+path.name)
                        anchor.String=''
                        index=document.createInstance('com.sun.star.text.ContentIndex')
                        index.CreateFromOutline=True;index.Level=3;index.Title=''
                        anchor.getText().insertTextContent(anchor,index,False)
                        indexes=document.getDocumentIndexes()
                    if indexes.Count!=expected:raise RuntimeError(f'{path.name}: expected {expected} TOC, got {indexes.Count}')
                    for _ in range(2):
                        document.refresh()
                        for index in range(indexes.Count):indexes.getByIndex(index).update()
                    if indexes.Count:
                        saved=Path(temporary)/(path.stem+'-updated.docx')
                        document.storeAsURL(saved.as_uri(),properties(FilterName='Office Open XML Text',Overwrite=True))
                        document.close(True)
                        path.write_bytes(saved.read_bytes())
                        document=desktop.loadComponentFromURL(path.as_uri(),'_blank',0,load)
                    pdf=output/(path.stem+'.pdf')
                    document.storeToURL(pdf.as_uri(),properties(FilterName='writer_pdf_Export',Overwrite=True))
                    document.close(True)
                    info=subprocess.run([args.pdfinfo,str(pdf)],capture_output=True,text=True,check=True)
                    (output/(path.stem+'-pdfinfo.txt')).write_text(info.stdout)
                    subprocess.run([args.pdftotext,'-layout',str(pdf),str(output/(path.stem+'.txt'))],check=True)
                    result['reports'].append({'file':path.name,'sha256_before_index_update':before,
                        'sha256':sha(path),'pdf':pdf.name,'pdf_sha256':sha(pdf),
                        'toc_indexes_updated':expected,'pdfinfo_exit_code':info.returncode,
                        'pages':int(next(l.split(':')[1].strip() for l in info.stdout.splitlines() if l.startswith('Pages:')))})
                    print(path.name,result['reports'][-1]['pages'],'pages',flush=True)
            finally:
                if document is not None:
                    try:document.close(True)
                    except Exception:pass
                if desktop is not None:desktop.terminate()
                try:process.wait(timeout=15)
                except subprocess.TimeoutExpired:process.terminate();process.wait(timeout=10)
    records=[]
    for item in result['reports']:
        path=build_reports.HERE/item['file']
        records.append({'file':path.name,'template':path.name.replace('-dk.docx','-report-template.docx'),
                        'bytes':path.stat().st_size,'sha256':sha(path)})
    structural=build_reports.validate(records)
    structural['rendering']='DOCX_TO_PDF_COMPLETED: see visual-validation.json for the separate inspection verdict and exact hashes; revalidate after real screenshots.'
    (build_reports.HERE/'validation.json').write_text(json.dumps(structural,ensure_ascii=False,indent=2)+'\n')
    result['completed_at_utc']=datetime.now(timezone.utc).isoformat();result['exit_code']=0
    (output/'render-run.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':main()
