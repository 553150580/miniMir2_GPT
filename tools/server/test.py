#!/usr/bin/env python3
"""执行真实Go测试并保存结果，默认不连接Mongo；--mongo显式测试专属开发副本集。"""
import argparse,datetime,hashlib,json,os,subprocess,sys
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()

def input_files():
 files=list((ROOT/'server/internal').rglob('*.go'))+list((ROOT/'server/cmd').rglob('*.go'))+list((ROOT/'server/api/generated').rglob('*.go'))
 files += [ROOT/'server/go.mod',ROOT/'server/go.sum',ROOT/'conf/dev/server.json',ROOT/'config/development/first-loop.json',ROOT/'config/development/pet-runtime-candidates.json',ROOT/'资料参考/myf/Mir200/Map/0.map',Path(__file__).resolve(),ROOT/'tools/dev',ROOT/'tools/toolchain.py']
 files += list((ROOT/'conf/toolchain').glob('*.lock.json'))
 files += [p for p in (ROOT/'server/internal').rglob('*') if p.is_file() and 'testdata' in p.parts]
 return sorted(set(files))
def main():
 p=argparse.ArgumentParser();p.add_argument('--mongo',action='store_true');p.add_argument('--race',action='store_true');p.add_argument('--integration-only',action='store_true');p.add_argument('--timeout-seconds',type=int,default=600 if os.name=='nt' else 180);p.add_argument('--package-parallelism',type=int,default=1 if os.name=='nt' else None);a=p.parse_args()
 if not 1<=a.timeout_seconds<=3600:p.error('测试包总时限必须在1–3600秒；单项RPC和业务时限不变')
 if a.mongo:
  probe=subprocess.run([sys.executable,str(ROOT/'tools/environment/mongodb_dev.py'),'status'],cwd=ROOT,capture_output=True,text=True,timeout=180)
  if probe.returncode:raise SystemExit('专属开发数据库所有权核验失败，未执行数据库测试：'+probe.stderr[-2000:])
  owned=json.loads(probe.stdout)
  config=json.loads((ROOT/'conf/dev/server.json').read_text(encoding='utf-8'))
  expected=config.get('mongo_uri_by_platform',{}).get('windows',config['mongo_uri']) if os.name=='nt' else config['mongo_uri']
  if not owned.get('running') or not owned.get('endpoint_verified',owned.get('forwarding_verified')) or owned.get('uri')!=expected:raise SystemExit('测试URI未通过专属数据库及回环监听核验，不连接')
 cmd=[sys.executable,str(ROOT/'tools/dev'),'go','test','-json']
 if a.race:cmd+=['-race']
 if a.package_parallelism is not None:
  if a.package_parallelism<1:p.error('测试包并行数必须为正数')
  cmd+=['-p',str(a.package_parallelism)]
 # Windows QEMU TCG 的累计执行较慢；只扩展测试包总预算，不改变单项RPC/业务deadline。
 cmd+=['./internal/integration' if a.integration_only else './...','-count=1',f'-timeout={a.timeout_seconds}s']
 env=dict(os.environ);env['MINIMIR_MONGO_E2E']='1' if a.mongo else '0'
 # 冻结业务输入和受控工具选择链；结束时重扫，以识别期间新增/删除的源码或testdata。
 before={x.relative_to(ROOT).as_posix():sha(x) for x in input_files()}
 started=datetime.datetime.now(datetime.timezone.utc).isoformat()
 result=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True,timeout=a.timeout_seconds+120)
 directory=ROOT/'server/test-results';directory.mkdir(exist_ok=True)
 (directory/'latest.events.jsonl').write_text(result.stdout,encoding='utf-8',newline='\n')
 events=[]
 for line in result.stdout.splitlines():
  try:events.append(json.loads(line))
  except json.JSONDecodeError:pass
 passed=[{'package':e.get('Package'),'test':e['Test']} for e in events if e.get('Action')=='pass' and e.get('Test')]
 skipped=[{'package':e.get('Package'),'test':e['Test']} for e in events if e.get('Action')=='skip' and e.get('Test')]
 failed=[{'package':e.get('Package'),'test':e.get('Test')} for e in events if e.get('Action') in ('fail','build-fail')]
 after={x.relative_to(ROOT).as_posix():sha(x) for x in input_files()}
 changed=before!=after
 report={'schema_version':1,'started_at_utc':started,'finished_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'command':cmd,'mongo_e2e_explicit':a.mongo,'race_detector':a.race,'exit_code':result.returncode,'passed':result.returncode==0 and not changed,'source_changed_during_test':changed,'passed_tests':passed,'skipped_tests':skipped,'fixture':{'path':'config/development/first-loop.json','sha256':sha(ROOT/'config/development/first-loop.json'),'candidate_only':True,'runtime_ready':False},'source_sha256':before,'source_sha256_after':after,'events_sha256':sha(directory/'latest.events.jsonl'),'production_ready':False}
 report.update(failed_tests=failed,package_timeout_seconds=a.timeout_seconds,package_parallelism=a.package_parallelism,single_rpc_and_business_deadlines_changed=False,capacity_verified=False)
 (directory/'latest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
 print(json.dumps({'passed':report['passed'],'tests':passed,'skipped':skipped,'report':str(directory/'latest.json'),'fixture_sha256':report['fixture']['sha256']},ensure_ascii=False,indent=2))
 if result.returncode:sys.stderr.write(result.stdout+result.stderr)
 raise SystemExit(result.returncode or (2 if changed else 0))
if __name__=='__main__':main()
