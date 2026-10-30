"""仓储GUI观察器静态收束，不启动Chrome、Creator、服务或数据库。"""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent / 'static-1'
FREEZE = ROOT / '.work/warehouses-20261030/client/production-freeze.json'
FILES = ['tools/client/verify_warehouses_browser.cjs', 'tools/client/test_warehouses_browser.cjs',
         'client/assets/scripts/generated/protobuf/client_pb.js', 'client/assets/scripts/app/GameConnection.ts',
         'client/assets/resources/development/gameplay.json', 'config/development/first-loop.json',
         'client/assets/scenes/Game.scene', '.work/warehouses-20261030/client/production-freeze.json',
         '.work/warehouses-20261030/creator/full-1/report.json',
         '.work/warehouses-20261030/browser/normal/run-01/run.py', '.work/warehouses-20261030/browser/run_static.py']

def sha(p): return hashlib.sha256(p.read_bytes()).hexdigest()
def snapshot(files): return {name: sha(ROOT / name) for name in files}
def save(name, data): (OUT / name).write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')

def main():
    OUT.mkdir(exist_ok=False)
    before = snapshot(FILES)
    expected = json.loads(FREEZE.read_text(encoding='utf-8'))['owned_files']
    owned_before = snapshot(expected)
    if owned_before != expected:
        raise RuntimeError('19份客户端冻结输入已变，拒绝静态收束')
    commands = [
        ['node', '--check', 'tools/client/verify_warehouses_browser.cjs'],
        ['node', '--check', 'tools/client/test_warehouses_browser.cjs'],
        ['node', 'tools/client/verify_warehouses_browser.cjs', '--preflight'],
        ['node', '--test', 'tools/client/test_warehouses_browser.cjs'],
        ['python3', '.work/warehouses-20261030/browser/normal/run-01/run.py'],
    ]
    records = []
    for index, args in enumerate(commands, 1):
        command = ['powershell.exe', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', str(ROOT / 'tools/dev.ps1'), *args]
        stdout, stderr = OUT / f'{index}.stdout.log', OUT / f'{index}.stderr.log'
        start = datetime.now(timezone.utc).isoformat()
        with stdout.open('xb') as out, stderr.open('xb') as err:
            task = subprocess.run(command, cwd=ROOT, stdout=out, stderr=err, check=False)
        records.append({'command': command, 'started_at': start, 'finished_at': datetime.now(timezone.utc).isoformat(),
                        'exit_code': task.returncode, 'stdout': stdout.name, 'stdout_sha256': sha(stdout), 'stderr': stderr.name, 'stderr_sha256': sha(stderr)})
    after, owned_after = snapshot(FILES), snapshot(expected)
    current, owned_current = snapshot(FILES), snapshot(expected)
    text = (OUT / '4.stdout.log').read_text(encoding='utf-8')
    count = re.search(r'(?:# |ℹ )pass (\d+)', text)
    report = {'passed': all(x['exit_code'] == 0 for x in records) and before == after == current and owned_before == owned_after == owned_current == expected,
              'scope': '仅静态/真实协议与只读观察器纯工具回归；不是Chrome/GPU/双仓业务验收',
              'commands': records, 'before': before, 'after': after, 'current_frozen': current, 'input_count': len(before),
              'source_stable': before == after == current, 'owned_production_before': owned_before, 'owned_production_after': owned_after,
              'owned_production_current': owned_current, 'owned_production_stable': owned_before == owned_after == owned_current == expected,
              'tool_tests_passed': int(count.group(1)) if count else None, 'browser_started': False, 'database_connected': False,
              'service_control': False, 'creator_started': False}
    save('report.json', report)
    print(json.dumps({'passed': report['passed'], 'report': str(OUT / 'report.json'), 'tool_tests_passed': report['tool_tests_passed'], 'input_count': len(before)}))
    return 0 if report['passed'] else 1

if __name__ == '__main__':
    raise SystemExit(main())
