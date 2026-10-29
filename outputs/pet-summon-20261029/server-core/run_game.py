import datetime,hashlib,json,os,pathlib,subprocess,sys,time
root=pathlib.Path(__file__).resolve().parents[3]
out=pathlib.Path(__file__).resolve().parent
name=sys.argv[1]
files=list((root/'server/internal/game').glob('*.go'))
files += [p for p in (root/'server/internal').rglob('*.go') if not p.name.endswith('_test.go')]
files += list((root/'server/api/generated').rglob('*.go'))
files += [p for p in (root/'server/internal/game/testdata').rglob('*') if p.is_file()]
files += [root/p for p in ['server/go.mod','server/go.sum','conf/dev/server.json','config/development/first-loop.json','config/development/pet-runtime-candidates.json','config/development/summon-candidates.json']]
def hashes():return {str(p.relative_to(root)).replace('\\','/'):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(set(files))}
before=hashes()
cmd=[sys.executable,str(root/'tools/dev'),'go','test','./internal/game','-race','-json','-count=1','-timeout=300s']
if len(sys.argv)>2:cmd+=['-run',sys.argv[2]]
start=time.monotonic();started=datetime.datetime.now(datetime.timezone.utc).isoformat()
env=dict(os.environ,MINIMIR_MONGO_E2E='0')
r=subprocess.run(cmd,cwd=root,env=env,capture_output=True,encoding='utf-8',errors='replace',timeout=420)
(out/(name+'.events.jsonl')).write_text(r.stdout,encoding='utf-8')
(out/(name+'.stderr.txt')).write_text(r.stderr,encoding='utf-8')
events=[]
for line in r.stdout.splitlines():
 try:events.append(json.loads(line))
 except ValueError:pass
after=hashes()
report=dict(command=cmd,started_at_utc=started,wall_seconds=round(time.monotonic()-start,3),exit_code=r.returncode,mongo_enabled=False,source_sha256=before,source_sha256_after=after,source_stable=before==after,
 passed=[e['Test'] for e in events if e.get('Action')=='pass' and e.get('Test')],failed=[e.get('Test',e.get('Package')) for e in events if e.get('Action')=='fail'],skipped=[e['Test'] for e in events if e.get('Action')=='skip' and e.get('Test')])
report['passed_run']=r.returncode==0 and before==after
(out/(name+'.json')).write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({k:report[k] for k in ['exit_code','wall_seconds','source_stable','passed_run','failed','skipped']},ensure_ascii=False))
print('passed_nodes='+str(len(report['passed'])))
raise SystemExit(r.returncode or (2 if before!=after else 0))
