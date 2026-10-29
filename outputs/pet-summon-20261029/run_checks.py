"""逐项保存统一入口的真实命令、退出码和原始输出，不从生成成功推断业务验收。"""
from pathlib import Path
import datetime,hashlib,json,subprocess,sys
ROOT=Path(__file__).resolve().parents[2];BASE=Path(__file__).parent/'checks';BASE.mkdir(exist_ok=True)
TASKS={
 'pet-sources':['python3','tools/reference/verify_pet_sources.py'],
 'references':['python3','tools/reference/verify_copies.py'],
 'config':['python3','tools/config/export_configs.py','--check'],
 'protocol':['python3','tools/config/generate_protocol.py','--check'],
 'protobuf':['node','tools/protocol/verify_client_pb.cjs'],
 'bindings':['node','tools/config/verify_bindings.mjs'],
 'python':['python3','-m','unittest','discover','-s','tools/config','-p','test_*.py'],
 'host':['python3','-m','unittest','discover','-s','tools/environment','-p','test_*.py'],
 'resources':['python3','tools/resources/verify_windows_assets.py'],
 'fixture':['python3','tools/resources/prepare_first_loop.py','--check'],
 'go':['python3','tools/server/test.py','--mongo','--race'],
 'typecheck':['node','tools/client/typecheck.mjs'],
 'models':['node','client/tests/ui/compile_models.mjs'],
 'client':['node','--test','client/tests/ui/*.test.cjs','client/tests/app/*.test.cjs'],
 'scene-compile':['node','tools/client/compile_scene_tests.mjs'],
 'scene':['node','--test','client/tests/scene/*.test.cjs'],
 'build':['python3','tools/client/build.py'],
 'main-status':['python3','tools/server/dev.py','status'],
 'mongodb-status':['python3','tools/environment/mongodb_dev.py','status'],
 'client-status':['python3','tools/client/dev.py','status'],
}
for name in sys.argv[1:]:
 args=TASKS[name];started=datetime.datetime.now(datetime.timezone.utc).isoformat()
 inputs=[]
 if name in ['models','client']:
  inputs=[p for directory in [ROOT/'client/assets/scripts',ROOT/'client/tests'] for p in directory.rglob('*') if p.is_file() and p.suffix in ['.ts','.js','.cjs','.mjs']]+[ROOT/'tools/client/verify_town_travel_browser.cjs']
 before={p.relative_to(ROOT).as_posix():hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(inputs)}
 result=subprocess.run([sys.executable,str(ROOT/'tools/dev'),*args],cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 output=BASE/(name+'.txt');output.write_bytes(result.stdout)
 record={'task':name,'command':args,'exit_code':result.returncode,'started_at':started,'finished_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'output':output.relative_to(ROOT).as_posix(),'output_sha256':hashlib.sha256(result.stdout).hexdigest(),'scope':'仅本项检查；生成与静态测试不等于实际业务、真机或容量验收'}
 if inputs:
  after={p.relative_to(ROOT).as_posix():hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(inputs)}
  record.update(input_sha256=before,input_sha256_after=after,inputs_stable=before==after)
  if before!=after:record['exit_code']=2
 (BASE/(name+'.json')).write_text(json.dumps(record,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
 print(json.dumps({'task':name,'exit_code':record['exit_code'],'output':record['output']},ensure_ascii=False),flush=True)
 if record['exit_code']:
  print(result.stdout.decode('utf-8',errors='replace')[-5000:],flush=True)
  raise SystemExit(record['exit_code'])
