"""锁定dev入口执行配置命令，保留原始stdout/stderr/退出码，不运行Go或数据库。"""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess
import time

ROOT=Path(__file__).resolve().parents[3]
OUT=Path(__file__).resolve().parent
COMMANDS={
    'candidate':['.work/equipment-growth-20261030/config/author_configuration.py'],
    'candidate-2':['.work/equipment-growth-20261030/config/author_configuration.py'],
    'candidate-3':['.work/equipment-growth-20261030/config/author_configuration.py'],
    'apply':['.work/equipment-growth-20261030/config/author_configuration.py','--apply'],
    'export-check':['tools/config/export_configs.py','--check'],
    'export':['tools/config/export_configs.py','--version','dev-equipment-foundation-v0.22'],
    'pet-generate':['tools/resources/prepare_pet_runtime.py'],
    'summon-generate':['tools/resources/prepare_summon_candidates.py'],
    'fixture-generate':['tools/resources/prepare_first_loop.py'],
    'pet-check':['tools/resources/prepare_pet_runtime.py','--check'],
    'summon-check':['tools/resources/prepare_summon_candidates.py','--check'],
    'fixture-check':['tools/resources/prepare_first_loop.py','--check'],
    'sync-bindings':['tools/config/sync_bindings.py'],
    'verify-bindings':['tools/config/verify_bindings.mjs'],
    'python-full':['-m','unittest','discover','-s','tools/config','-p','test_*.py','-v'],
    'equipment-narrow':['-m','unittest','discover','-s','tools/config','-p','test_equipment_foundation.py','-v'],
}
parser=argparse.ArgumentParser();parser.add_argument('checks',nargs='+',choices=COMMANDS);args=parser.parse_args()
for name in args.checks:
    report=OUT/(name+'.json')
    if report.exists():raise SystemExit('不覆盖原始命令证据：'+str(report))
    program='node' if name=='verify-bindings' else 'python3'
    command=['powershell','-NoProfile','-ExecutionPolicy','Bypass','-File',str(ROOT/'tools/dev.ps1'),program,*COMMANDS[name]]
    start=datetime.now(timezone.utc).isoformat();begin=time.monotonic();result=subprocess.run(command,cwd=ROOT,capture_output=True)
    stdout,stderr=OUT/(name+'.stdout.txt'),OUT/(name+'.stderr.txt');stdout.write_bytes(result.stdout);stderr.write_bytes(result.stderr)
    value={'command':['.\\tools\\dev.ps1',program,*COMMANDS[name]],'actual_command':command,'started_at_utc':start,
        'finished_at_utc':datetime.now(timezone.utc).isoformat(),'seconds':round(time.monotonic()-begin,3),'exit_code':result.returncode,
        'stdout':stdout.name,'stderr':stderr.name,'stdout_sha256':hashlib.sha256(result.stdout).hexdigest(),'stderr_sha256':hashlib.sha256(result.stderr).hexdigest()}
    report.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n');print(json.dumps(value,ensure_ascii=False),flush=True)
    if result.returncode:
        print((result.stderr or result.stdout).decode('utf-8',errors='replace')[-9000:]);raise SystemExit(result.returncode)
