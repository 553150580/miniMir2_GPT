"""新召唤费用合同的只读Go overlay反例，不连接DB。"""
from pathlib import Path
import hashlib,json,subprocess,sys
ROOT=Path(__file__).resolve().parents[3]
BASE=Path(__file__).resolve().parent
overlay=BASE/'cost-overlay.json'
overlay.write_text(json.dumps({'Replace':{str(ROOT/'server/internal/devconfig/summon_cost_peer_overlay_test.go'):str(BASE/'cost_overlay_test.go')}},indent=2)+'\n',encoding='utf-8',newline='\n')
paths=[ROOT/name for name in ['server/internal/devconfig/summon.go','server/internal/devconfig/summon_candidates.go','server/internal/devconfig/skill_economy.go','config/development/first-loop.json','config/development/summon-candidates.json']]
snapshot=lambda:{path.relative_to(ROOT).as_posix():hashlib.sha256(path.read_bytes()).hexdigest() for path in paths}
before=snapshot()
args=['go','test','-overlay',str(overlay),'./internal/devconfig','-run','^TestAuditSummonCatalog','-count=1','-v']
result=subprocess.run(['powershell','-NoProfile','-ExecutionPolicy','Bypass','-File',str(ROOT/'tools/dev.ps1'),*args],cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
output=result.stdout.decode('utf-8',errors='replace')
(BASE/'cost-probe.txt').write_text(output,encoding='utf-8',newline='\n')
after=snapshot()
record={'command':'.\\tools\\dev.ps1 '+' '.join(args),'exit_code':result.returncode,'mongo_invoked':False,'overlay_only':True,'source_before':before,'source_after':after,'source_stable':before==after}
(BASE/'cost-probe.json').write_text(json.dumps(record,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
print(json.dumps({'exit_code':result.returncode,'source_stable':before==after,'mongo_invoked':False}))
print(output)
sys.exit(result.returncode)
