#!/usr/bin/env python3
"""执行真实Go测试并保存结果，默认不连接Mongo；--mongo显式测试专属开发副本集。"""
import argparse,datetime,hashlib,json,os,subprocess,sys
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def main():
 p=argparse.ArgumentParser();p.add_argument('--mongo',action='store_true');p.add_argument('--race',action='store_true');p.add_argument('--integration-only',action='store_true');a=p.parse_args()
 cmd=[str(ROOT/'tools/dev'),'go','test','-json']
 if a.race:cmd+=['-race']
 cmd+=['./internal/integration' if a.integration_only else './...','-count=1','-timeout=180s']
 env=dict(os.environ);env['MINIMIR_MONGO_E2E']='1' if a.mongo else '0'
 files=list((ROOT/'server/internal').rglob('*.go'))+list((ROOT/'server/cmd/nbserver').rglob('*.go'))+[ROOT/'server/go.mod',ROOT/'server/go.sum',ROOT/'conf/dev/server.json',ROOT/'config/development/first-loop.json']
 before={str(x.relative_to(ROOT)):sha(x) for x in sorted(files)}
 started=datetime.datetime.now(datetime.timezone.utc).isoformat()
 result=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True,timeout=240)
 directory=ROOT/'server/test-results';directory.mkdir(exist_ok=True)
 (directory/'latest.events.jsonl').write_text(result.stdout,encoding='utf-8')
 events=[]
 for line in result.stdout.splitlines():
  try:events.append(json.loads(line))
  except json.JSONDecodeError:pass
 passed=[{'package':e.get('Package'),'test':e['Test']} for e in events if e.get('Action')=='pass' and e.get('Test')]
 skipped=[{'package':e.get('Package'),'test':e['Test']} for e in events if e.get('Action')=='skip' and e.get('Test')]
 after={str(x.relative_to(ROOT)):sha(x) for x in sorted(files)}
 changed=before!=after
 report={'schema_version':1,'started_at_utc':started,'finished_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'command':cmd,'mongo_e2e_explicit':a.mongo,'race_detector':a.race,'exit_code':result.returncode,'passed':result.returncode==0 and not changed,'source_changed_during_test':changed,'passed_tests':passed,'skipped_tests':skipped,'fixture':{'path':'config/development/first-loop.json','sha256':sha(ROOT/'config/development/first-loop.json'),'candidate_only':True,'runtime_ready':False},'source_sha256':before,'source_sha256_after':after,'events_sha256':sha(directory/'latest.events.jsonl'),'production_ready':False}
 (directory/'latest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
 print(json.dumps({'passed':report['passed'],'tests':passed,'skipped':skipped,'report':str(directory/'latest.json'),'fixture_sha256':report['fixture']['sha256']},ensure_ascii=False,indent=2))
 if result.returncode:sys.stderr.write(result.stdout+result.stderr)
 raise SystemExit(result.returncode or (2 if changed else 0))
if __name__=='__main__':main()
