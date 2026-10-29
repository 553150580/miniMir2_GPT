"""真实包装命令、输出与退出码归档；不运行全量Go或覆盖preview_current。"""
from pathlib import Path
import datetime
import hashlib
import json
import subprocess
import sys
import time

ROOT=Path(__file__).resolve().parents[2]
BASE=Path(__file__).resolve().parent/'checks'
FILES=['tools/resources/prepare_pet_runtime.py','tools/resources/prepare_first_loop.py',
       'tools/config/export_configs.py','tools/config/edit_workbook.py','tools/config/test_export.py',
       'tools/config/test_edit_workbook.py','tools/config/test_pet_runtime.py','tools/config/test_teleport.py',
       'tools/config/test_safe_zone_binding.py','config/schema/catalog.json',
       'outputs/development-prep-20261026/玩法配置.xlsx','outputs/development-prep-20261026/架构配置.xlsx']
sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()

def hashes():return {name:sha(ROOT/name) for name in FILES}
def save(name,value):(BASE/name).write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')

def main():
    BASE.mkdir(parents=True,exist_ok=True)
    before=hashes();save('inputs-before.json',before)
    commands=[('candidate-generate',['python3','tools/resources/prepare_pet_runtime.py']),
              ('fixture-generate',['python3','tools/resources/prepare_first_loop.py']),
              ('candidate-check',['python3','tools/resources/prepare_pet_runtime.py','--check']),
              ('fixture-check',['python3','tools/resources/prepare_first_loop.py','--check']),
              ('python-configuration',['python3','-m','unittest','discover','-s','tools/config','-v'])]
    results=[]
    for name,args in commands:
        command=['powershell','-NoProfile','-ExecutionPolicy','Bypass','-File',str(ROOT/'tools/dev.ps1'),*args]
        start=time.monotonic();began=datetime.datetime.now(datetime.timezone.utc).isoformat()
        result=subprocess.run(command,cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
        output=result.stdout.decode('utf-8',errors='replace')
        (BASE/(name+'.txt')).write_text(output,encoding='utf-8',newline='\n')
        record={'command':'.\\tools\\dev.ps1 '+' '.join(args),'started_at_utc':began,'exit_code':result.returncode,
                'elapsed_seconds':round(time.monotonic()-start,3),'output':name+'.txt'}
        results.append(record);save('commands.json',results)
        print(json.dumps(record,ensure_ascii=False),flush=True)
        if result.returncode:sys.exit(result.returncode)
    after=hashes();save('inputs-after.json',after);assert before==after,'冻结输入期间变化'
    catalog=json.loads((ROOT/'config/development/pet-runtime-candidates.json').read_text(encoding='utf-8'))
    fixture=json.loads((ROOT/'config/development/first-loop.json').read_text(encoding='utf-8'))
    assert catalog['runtime_enabled'] is False
    assert catalog['definitions']==fixture['pet_runtime']['definitions']
    assert catalog['budgets']==fixture['pet_runtime']['budgets']
    assert fixture['_source_evidence']['pet_runtime_inputs']['compiler_sha256']==sha(ROOT/'tools/resources/prepare_pet_runtime.py')
    artifacts={name:sha(ROOT/name) for name in ['config/development/pet-runtime-candidates.json','config/development/first-loop.json',
        'client/assets/resources/development/gameplay.json']}
    summary={'commands':results,'inputs_stable':True,'input_sha256':after,'artifact_sha256':artifacts,
             'candidate_and_runtime_same_definitions_and_budgets':True,'candidate_runtime_enabled':False,
             'fixture_tracks_compiler_and_both_books':True,'preview_current_untouched':True}
    save('summary.json',summary)
    print(json.dumps({'passed':True,'inputs_stable':True,'artifact_sha256':artifacts},ensure_ascii=False))

if __name__=='__main__':main()
