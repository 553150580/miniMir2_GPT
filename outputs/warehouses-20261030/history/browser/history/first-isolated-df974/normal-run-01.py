"""正常主23个人仓触屏包装器；只有--run启动Chrome，不管理服务或数据库。"""
from __future__ import annotations
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[5]
OUT = Path(__file__).resolve().parent
TOOL = ROOT / 'tools/client/verify_warehouses_browser.cjs'
CREATOR = ROOT / '.work/warehouses-20261030/creator/full-1/report.json'
INPUTS = [TOOL, CREATOR, ROOT / 'config/development/first-loop.json', ROOT / 'client/assets/resources/development/gameplay.json']

def sha(path): return hashlib.sha256(path.read_bytes()).hexdigest()
def now(): return datetime.now(timezone.utc).isoformat()
def save(path, data): path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run', action='store_true')
    args = parser.parse_args()
    if ROOT.name != 'miniMir2_GPT' or not (ROOT / 'tools/dev.ps1').is_file():
        raise RuntimeError('非预期项目目录')
    command = ['powershell.exe', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', str(ROOT / 'tools/dev.ps1'),
               'node', str(TOOL), '--url', 'http://127.0.0.1:18080/', '--creator-report', str(CREATOR), '--report-dir', str(OUT)]
    before = {p.relative_to(ROOT).as_posix(): sha(p) for p in INPUTS}
    plan = {'report_directory': OUT.relative_to(ROOT).as_posix(), 'command': command, 'inputs_before': before,
            'run_requested': args.run, 'service_control': False, 'database_seeding': False, 'product_source_modified': False}
    if not args.run:
        print(json.dumps(plan, ensure_ascii=False, indent=2))
        return 0
    if any((OUT / name).exists() for name in ['report.json', 'stdout.log', 'stderr.log', 'wrapper-start.json', 'wrapper-result.json', 'tool-at-start.cjs']):
        raise RuntimeError('本轮已有原始证据，拒绝覆盖')
    (OUT / 'tool-at-start.cjs').write_bytes(TOOL.read_bytes())
    started = now()
    save(OUT / 'wrapper-start.json', {**plan, 'started_at': started, 'wrapper_sha256': sha(Path(__file__))})
    with (OUT / 'stdout.log').open('xb') as stdout, (OUT / 'stderr.log').open('xb') as stderr:
        task = subprocess.run(command, cwd=ROOT, stdout=stdout, stderr=stderr, check=False,
                              creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    after = {p.relative_to(ROOT).as_posix(): sha(p) for p in INPUTS}
    result = {**plan, 'started_at': started, 'finished_at': now(), 'exit_code': task.returncode, 'inputs_after': after,
              'inputs_stable': before == after, 'stdout_sha256': sha(OUT / 'stdout.log'), 'stderr_sha256': sha(OUT / 'stderr.log'),
              'browser_report_sha256': sha(OUT / 'report.json') if (OUT / 'report.json').is_file() else None}
    save(OUT / 'wrapper-result.json', result)
    print(json.dumps({'exit_code': task.returncode, 'report_directory': plan['report_directory'], 'inputs_stable': before == after}))
    return task.returncode if before == after else 1

if __name__ == '__main__':
    raise SystemExit(main())
