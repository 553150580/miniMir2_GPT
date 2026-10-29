"""记录本阶段纯Python/生成检查；不运行Go、DB或preview同步。"""
from pathlib import Path
import datetime
import hashlib
import json
import subprocess
import sys
import time

ROOT=Path(__file__).resolve().parents[2]
phase=sys.argv[1]
BASE=Path(__file__).resolve().parent/'checks'/phase
BASE.mkdir(parents=True,exist_ok=True)
FILES=['tools/resources/prepare_first_loop.py','tools/resources/prepare_pet_runtime.py','tools/resources/prepare_summon_candidates.py',
    'tools/config/test_summon_candidates.py','tools/config/test_pet_runtime.py','tools/config/test_teleport.py','tools/config/test_safe_zone_binding.py',
    'tools/config/test_export.py','tools/config/export_configs.py','tools/config/edit_workbook.py',
    'tools/config/test_source_map_projection.py',
    'config/schema/catalog.json','outputs/development-prep-20261026/玩法配置.xlsx','outputs/development-prep-20261026/架构配置.xlsx']
sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
snapshot=lambda:{name:sha(ROOT/name) for name in FILES}
before=snapshot()
(BASE/'before.json').write_text(json.dumps(before,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
commands=[('python',['python3','-m','unittest','discover','-s','tools/config','-v'])]
if phase=='final':
    commands=[(kind+'-'+action,['python3','tools/resources/'+tool]+(['--check'] if action=='check' else []))
        for action in ['generate','check'] for kind,tool in [('pet','prepare_pet_runtime.py'),('summon','prepare_summon_candidates.py'),('fixture','prepare_first_loop.py')]]+commands
records=[]
for name,args in commands:
    start=time.monotonic();began=datetime.datetime.now(datetime.timezone.utc).isoformat()
    result=subprocess.run(['powershell','-NoProfile','-ExecutionPolicy','Bypass','-File',str(ROOT/'tools/dev.ps1'),*args],cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
    (BASE/(name+'.txt')).write_text(result.stdout.decode('utf-8',errors='replace'),encoding='utf-8',newline='\n')
    record={'command':'.\\tools\\dev.ps1 '+' '.join(args),'exit_code':result.returncode,'started_at_utc':began,'elapsed_seconds':round(time.monotonic()-start,3),'output':name+'.txt'}
    records.append(record)
    (BASE/'commands.json').write_text(json.dumps(records,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
    print(json.dumps(record,ensure_ascii=False),flush=True)
    if result.returncode:sys.exit(result.returncode)
after=snapshot()
assert before==after
artifacts={name:sha(ROOT/name) for name in ['config/development/first-loop.json','config/development/pet-runtime-candidates.json','config/development/summon-candidates.json','client/assets/resources/development/gameplay.json']}
summary={'passed':True,'inputs_stable':True,'input_count':len(before),'inputs':after,'artifact_sha256':artifacts,'commands':records,'preview_sync_performed':False,'go_or_db_executed':False}
(BASE/'after.json').write_text(json.dumps(after,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
(BASE/'summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
print(json.dumps(summary,ensure_ascii=False))
