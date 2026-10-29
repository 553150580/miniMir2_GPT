"""验收后只通过项目受控命令重启本任务后端；不操作其它端口或数据库。"""
from pathlib import Path
import datetime, hashlib, json, subprocess, sys, time

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
DEV = [sys.executable, str(ROOT / 'tools/dev')]
go = json.loads((WORK / 'go/full-2/report.json').read_text(encoding='utf-8'))
build = json.loads((WORK / 'creator/full-1/report.json').read_text(encoding='utf-8'))
assert go['passed'] and go['exit_code'] == 0 and not go['failed_tests'] and not go['skipped_tests']
assert build['success'] and build['exit_code'] == 36 and build['scene_prefab_layout_preserved'] and build['source_stable_during_build']
for name, value in {**go['source_sha256'], **build['asset_hashes'], **build['script_hashes']}.items():
    assert hashlib.sha256((ROOT / name).read_bytes()).hexdigest() == value, '已验输入变化：' + name
out = WORK / 'runtime-control'
out.mkdir(exist_ok=False)
report = {'started_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'commands': [],
          'scope': '仅重启本任务已登记后端；沿受控PID/身份核验，不操作数据库或其它服务', 'other_ports_touched': []}
for name in ['stop', 'start']:
    command = DEV + ['python3', 'tools/server/dev.py', name]
    begin = time.monotonic()
    with (out / (name + '.stdout.txt')).open('wb') as stdout, (out / (name + '.stderr.txt')).open('wb') as stderr:
        result = subprocess.run(command, cwd=ROOT, stdout=stdout, stderr=stderr)
    row = {'name': name, 'command': command, 'exit_code': result.returncode, 'seconds': round(time.monotonic() - begin, 3)}
    for stream in ['stdout', 'stderr']:
        path = out / (name + '.' + stream + '.txt')
        row[stream] = path.relative_to(ROOT).as_posix()
        row[stream + '_sha256'] = hashlib.sha256(path.read_bytes()).hexdigest()
    report['commands'].append(row)
    report['passed'] = all(c['exit_code'] == 0 for c in report['commands']) and len(report['commands']) == 2
    report['finished_at_utc'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
    (out / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps(row, ensure_ascii=False), flush=True)
    if result.returncode:
        raise SystemExit(result.returncode)
print(json.dumps({'controlled_restart_passed': report['passed']}, ensure_ascii=False))
