"""只运行devconfig的.work overlay反例，不修改Go源、不连接DB。"""
from pathlib import Path
import datetime
import hashlib
import json
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[3]
BASE=Path(__file__).resolve().parent
probe=BASE/'legacy_name_overlay_test.go'
virtual=ROOT/'server/internal/devconfig/summon_peer_overlay_test.go'
overlay=BASE/'legacy-overlay.json'
overlay.write_text(json.dumps({'Replace':{str(virtual):str(probe)}},indent=2)+'\n',encoding='utf-8',newline='\n')
paths=[ROOT/name for name in ['server/internal/devconfig/summon.go','server/internal/devconfig/summon_candidates.go','server/internal/devconfig/pet_runtime.go','config/development/first-loop.json','config/development/pet-runtime-candidates.json']]
snapshot=lambda:{path.relative_to(ROOT).as_posix():hashlib.sha256(path.read_bytes()).hexdigest() for path in paths}
before=snapshot()
args=['go','test','-overlay',str(overlay),'./internal/devconfig','-run','^TestAuditLegacy','-count=1','-v']
result=subprocess.run(['powershell','-NoProfile','-ExecutionPolicy','Bypass','-File',str(ROOT/'tools/dev.ps1'),*args],cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
output=result.stdout.decode('utf-8',errors='replace')
(BASE/'legacy-probe.txt').write_text(output,encoding='utf-8',newline='\n')
after=snapshot()
record={'command':'.\\tools\\dev.ps1 '+' '.join(args),'exit_code':result.returncode,'mongo_invoked':False,
    'source_before':before,'source_after':after,'source_stable':before==after,'overlay_only':True,'recorded_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat()}
(BASE/'legacy-probe.json').write_text(json.dumps(record,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
print(json.dumps({'exit_code':result.returncode,'source_stable':before==after,'mongo_invoked':False}))
print(output)
sys.exit(result.returncode)
