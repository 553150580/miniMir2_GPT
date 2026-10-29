from __future__ import annotations

# 只包装正常触屏验收：不启停服务、不写角色、不修改产品或覆盖既有证据。
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
from datetime import datetime, timezone

ROOT = Path(__file__).resolve().parents[4]
OUT = Path(__file__).resolve().parent
TOOL = ROOT / 'tools/client/verify_level_growth_browser.cjs'
INPUTS = [TOOL, ROOT / 'docs/客户端构建检查.json', ROOT / 'config/development/first-loop.json', ROOT / 'client/assets/resources/development/gameplay.json']

def sha(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()

def now() -> str:
    return datetime.now(timezone.utc).isoformat()

def write(path: Path, value: dict) -> None:
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')

def main() -> int:
    parser = argparse.ArgumentParser(description='自然成长正常UI验收；只有--run才启动Chrome')
    parser.add_argument('--run', action='store_true')
    args = parser.parse_args()
    if ROOT.name != 'miniMir2_GPT' or not (ROOT / 'tools/dev.ps1').is_file():
        raise RuntimeError('包装器不在预期项目目录')
    relative = OUT.relative_to(ROOT).as_posix()
    command = ['powershell.exe', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', str(ROOT / 'tools/dev.ps1'), 'node', str(TOOL), '--report-dir', relative]
    inputs = {p.relative_to(ROOT).as_posix(): sha(p) for p in INPUTS}
    plan = {'report_directory': relative, 'command': command, 'inputs': inputs, 'run_requested': args.run, 'product_source_modified': False, 'service_control': False, 'database_seeding': False}
    if not args.run:
        print(json.dumps(plan, ensure_ascii=False, indent=2))
        return 0
    forbidden = [OUT / name for name in ('report.json', 'stdout.log', 'stderr.log', 'wrapper-start.json', 'wrapper-result.json')]
    if any(p.exists() for p in forbidden):
        raise RuntimeError('本轮已有运行证据，拒绝覆盖；应保留现场并使用新run目录')
    started = now()
    write(OUT / 'wrapper-start.json', {**plan, 'started_at': started, 'wrapper_sha256': sha(Path(__file__))})
    with (OUT / 'stdout.log').open('xb') as stdout, (OUT / 'stderr.log').open('xb') as stderr:
        completed = subprocess.run(command, cwd=ROOT, stdout=stdout, stderr=stderr, creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0, check=False)
    after = {p.relative_to(ROOT).as_posix(): sha(p) for p in INPUTS}
    result = {**plan, 'started_at': started, 'finished_at': now(), 'exit_code': completed.returncode, 'inputs_after': after, 'inputs_stable': inputs == after, 'stdout_sha256': sha(OUT / 'stdout.log'), 'stderr_sha256': sha(OUT / 'stderr.log'), 'browser_report_sha256': sha(OUT / 'report.json') if (OUT / 'report.json').exists() else None}
    write(OUT / 'wrapper-result.json', result)
    print(json.dumps({'exit_code': completed.returncode, 'report_directory': relative, 'inputs_stable': inputs == after}, ensure_ascii=False))
    return completed.returncode if inputs == after else 1

if __name__ == '__main__':
    sys.exit(main())
