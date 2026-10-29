"""显式所有权探测后运行单个真实召唤WS链路，原始退出与全部输入摘要一并保存。"""
from pathlib import Path
import datetime,hashlib,importlib.util,json,os,subprocess,sys,time
ROOT=Path(__file__).resolve().parents[2];BASE=Path(__file__).parent/'ws-runtime';BASE.mkdir(exist_ok=True)
dev=[sys.executable,str(ROOT/'tools/dev')]
probe=subprocess.run([*dev,'python3','tools/environment/mongodb_dev.py','status'],cwd=ROOT,capture_output=True,check=True)
owned=json.loads(probe.stdout)
conf=json.loads((ROOT/'conf/dev/server.json').read_text(encoding='utf-8'))
assert owned['running'] and owned['endpoint_verified'] and owned['uri']==conf['mongo_uri_by_platform']['windows']
spec=importlib.util.spec_from_file_location('go_runner',ROOT/'tools/server/test.py');runner=importlib.util.module_from_spec(spec);spec.loader.exec_module(runner)
def snapshot():return {p.relative_to(ROOT).as_posix():hashlib.sha256(p.read_bytes()).hexdigest() for p in runner.input_files()}
before=snapshot();started=datetime.datetime.now(datetime.timezone.utc).isoformat();clock=time.monotonic()
args=['go','test','-race','./internal/integration','-run','^TestSummonPetMaterialRecallTrainingNoPetsAndColdMongoWS$','-count=1','-timeout=120s','-v']
env=dict(os.environ);env['MINIMIR_MONGO_E2E']='1'
r=subprocess.run([*dev,*args],cwd=ROOT,env=env,capture_output=True,timeout=150)
after=snapshot()
run=BASE/('run-'+datetime.datetime.now(datetime.timezone.utc).strftime('%H%M%S'));run.mkdir(exist_ok=False)
(run/'output.txt').write_bytes(r.stdout+r.stderr)
report={'command':args,'started_at_utc':started,'seconds':round(time.monotonic()-clock,3),'exit_code':r.returncode,'passed':r.returncode==0 and before==after,'real_mongo_ws':True,'mongo_endpoint_verified':True,'input_sha256':before,'input_sha256_after':after,'inputs_stable':before==after,'changed_inputs':[p for p in sorted(set(before)|set(after)) if before.get(p)!=after.get(p)],'output_sha256':hashlib.sha256((run/'output.txt').read_bytes()).hexdigest()}
(run/'report.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
print(json.dumps({'passed':report['passed'],'exit_code':r.returncode,'seconds':report['seconds'],'report':str(run/'report.json'),'inputs':len(before),'changed_inputs':report['changed_inputs']},ensure_ascii=False),flush=True)
if not report['passed']:print((r.stdout+r.stderr).decode('utf-8',errors='replace')[-6000:],flush=True)
raise SystemExit(r.returncode or (2 if before!=after else 0))
