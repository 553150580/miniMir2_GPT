"""保留实际命令、原始stdout/stderr与退出码；不启动Go或数据库。"""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import subprocess
import sys
import time

ROOT=Path(__file__).resolve().parents[2]
OUT=Path(__file__).resolve().parent

COMMANDS={
    'narrow':['-m','unittest','discover','-s','tools/config','-p','test_level_growth.py','-v'],
    'python-full':['-m','unittest','discover','-s','tools/config','-p','test_*.py','-v'],
    'export-check':['tools/config/export_configs.py','--check'],
    'export':['tools/config/export_configs.py','--version','dev-level-growth-v0.20'],
    'pet-generate':['tools/resources/prepare_pet_runtime.py'],
    'summon-generate':['tools/resources/prepare_summon_candidates.py'],
    'fixture-generate':['tools/resources/prepare_first_loop.py'],
    'pet-check':['tools/resources/prepare_pet_runtime.py','--check'],
    'summon-check':['tools/resources/prepare_summon_candidates.py','--check'],
    'fixture-check':['tools/resources/prepare_first_loop.py','--check'],
}

parser=argparse.ArgumentParser();parser.add_argument('checks',nargs='+',choices=COMMANDS);args=parser.parse_args()
for name in args.checks:
    command=[sys.executable,*COMMANDS[name]];started=datetime.now(timezone.utc).isoformat();begin=time.monotonic()
    r=subprocess.run(command,cwd=ROOT,capture_output=True,encoding='utf-8',errors='strict')
    (OUT/(name+'.stdout.txt')).write_text(r.stdout,encoding='utf-8',newline='\n')
    (OUT/(name+'.stderr.txt')).write_text(r.stderr,encoding='utf-8',newline='\n')
    report={'command':['.\\tools\\dev.ps1','python3',*COMMANDS[name]],'actual_python':sys.executable,'started_at_utc':started,
            'finished_at_utc':datetime.now(timezone.utc).isoformat(),'seconds':round(time.monotonic()-begin,3),'exit_code':r.returncode,
            'stdout':name+'.stdout.txt','stderr':name+'.stderr.txt'}
    (OUT/(name+'.json')).write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
    print(json.dumps(report,ensure_ascii=False))
    if r.returncode:
        print((r.stderr or r.stdout)[-7000:]);raise SystemExit(r.returncode)
