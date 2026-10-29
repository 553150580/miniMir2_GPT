"""重任务串行执行，任何终态都尝试按所有权恢复主预览。"""
from pathlib import Path
import datetime,hashlib,json,subprocess,sys,time
ROOT=Path(__file__).resolve().parents[2];WORK=Path(__file__).parent
sys.path.insert(0,str(ROOT/'tools/client'))
import prepare_pet_runtime_qa as common

def save(path,value):common.save(path,value)
def current_go():return {p.relative_to(ROOT).as_posix():hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted((ROOT/'server').rglob('*.go'))}
report={'passed':False,'started_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'steps':[],'main_restored':False}
tasks=sys.argv[1:] or ['build','go']
if any(task not in ['build','go'] for task in tasks):raise SystemExit('仅接受build/go检查')
stopped=False
try:
 common.assert_main_identity()
 report['main_before']=common.dev(['python3','tools/server/dev.py','status'])
 report['main_stop']=common.dev(['python3','tools/server/dev.py','stop'])
 if report['main_stop']['running']:raise RuntimeError('main未完成受控停止')
 stopped=True
 common.ports_free()
 for step in tasks:
  if step=='go':
   frozen=WORK/'server/go-frozen.json'
   deadline=time.monotonic()+60
   while not frozen.exists() and time.monotonic()<deadline:time.sleep(.2)
   if not frozen.exists():raise RuntimeError('未收到真实Go冻结证据，未启动全包')
   marker=json.loads(frozen.read_text(encoding='utf-8'))
   if not marker.get('frozen') or marker['go_sha256']!=current_go():raise RuntimeError('Go冻结证据与当前源码不符，未启动全包')
   report['go_freeze_verified']=True
  result=subprocess.run([sys.executable,str(ROOT/'tools/dev'),'python3',str(WORK/'run_checks.py'),step],cwd=ROOT,capture_output=True)
  (WORK/'checks'/('serial-'+step+'.txt')).write_bytes(result.stdout+result.stderr)
  report['steps'].append({'task':step,'exit_code':result.returncode})
  print(json.dumps(report['steps'][-1]),flush=True)
  if result.returncode:raise RuntimeError(step+'失败，原始日志已保存')
 report['passed']=True
except Exception as error:report['error']=str(error)
finally:
 if stopped:
  try:
   common.ports_free()
   report['main_after']=common.dev(['python3','tools/server/dev.py','start'],WORK/'checks/main-restored.txt',timeout=120)
   report['main_restored']=report['main_after']['running'] and len(report['main_after']['health'])==3 and all(s['status']=='ok' for s in report['main_after']['health'].values())
  except Exception as error:report['restore_error']=str(error)
 report['passed']=report['passed'] and report['main_restored']
 report['finished_at_utc']=datetime.datetime.now(datetime.timezone.utc).isoformat()
 save(WORK/'checks'/('serial-report-'+'-'.join(tasks)+'.json'),report)
 print(json.dumps({k:report.get(k) for k in ['passed','steps','main_restored','error','restore_error']},ensure_ascii=False),flush=True)
raise SystemExit(0 if report['passed'] else 1)
