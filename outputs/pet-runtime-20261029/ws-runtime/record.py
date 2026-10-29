"""受控单项编译/真实MongoWS记录；--real仅在root通知后使用。"""
from pathlib import Path
import datetime
import hashlib
import json
import os
import subprocess
import sys
import time

ROOT=Path(__file__).resolve().parents[3]
BASE=Path(__file__).resolve().parent
real='--real' in sys.argv
name='real-mongo-ws' if real else 'compile'
args=['go','test','./internal/integration','-run','^TestPetRuntimeCorpseHealthyExpiryAndColdEntryMongoWS$' if real else '^$','-count=1','-timeout','90s','-v']
if real:args.insert(2,'-race')
command=['powershell','-NoProfile','-ExecutionPolicy','Bypass','-File',str(ROOT/'tools/dev.ps1'),*args]
env=os.environ.copy()
if real:env['MINIMIR_MONGO_E2E']='1'
else:env.pop('MINIMIR_MONGO_E2E',None)
files=list((ROOT/'server').rglob('*.go'))+[ROOT/'go.work',ROOT/'config/development/first-loop.json',ROOT/'config/development/pet-runtime-candidates.json']
files=[path for path in files if path.is_file()]
hashes=lambda:{path.relative_to(ROOT).as_posix():hashlib.sha256(path.read_bytes()).hexdigest() for path in files}
before=hashes()
(BASE/(name+'-before.json')).write_text(json.dumps(before,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
started=datetime.datetime.now(datetime.timezone.utc).isoformat();tick=time.monotonic()
result=subprocess.run(command,cwd=ROOT,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
output=result.stdout.decode('utf-8',errors='replace')
(BASE/(name+'.txt')).write_text(output,encoding='utf-8',newline='\n')
after=hashes()
(BASE/(name+'-after.json')).write_text(json.dumps(after,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
record={'command':'.\\tools\\dev.ps1 '+' '.join(args),'started_at_utc':started,'elapsed_seconds':round(time.monotonic()-tick,3),
    'exit_code':result.returncode,'real_mongo_ws':real,'inputs_stable':before==after,
    'changed_inputs':[name for name in before if before[name]!=after.get(name)],'output':name+'.txt'}
(BASE/(name+'-result.json')).write_text(json.dumps(record,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
print(json.dumps(record,ensure_ascii=False))
print(output)
sys.exit(result.returncode)
