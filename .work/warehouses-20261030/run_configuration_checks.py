"""运行实际配置生成和检查，逐项保留命令原输出及终态。"""
from datetime import datetime, timezone
from pathlib import Path
import hashlib
import json
import re
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
OUT = WORK / 'configuration-run-01'
DEV = [sys.executable, str(ROOT / 'tools/dev')]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def inputs():
    names = set((ROOT / 'tools/config').rglob('*.py'))
    names.update(ROOT / 'tools/resources' / name for name in ['prepare_first_loop.py', 'prepare_pet_runtime.py', 'prepare_summon_candidates.py'])
    names.update(ROOT / name for name in ['config/schema/catalog.json', 'outputs/development-prep-20261026/玩法配置.xlsx',
                 'outputs/development-prep-20261026/架构配置.xlsx', 'shared/proto/protocol_manifest.json'])
    return {path.relative_to(ROOT).as_posix(): sha(path) for path in sorted(names)}


def write(report):
    (OUT / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def main():
    OUT.mkdir(exist_ok=False)
    report = {'started_at_utc': datetime.now(timezone.utc).isoformat(), 'passed': False,
              'scope': '实际唯一表配置导出/投影与Python配置全套；不替代Go、GUI、Creator、业务或容量验收',
              'input_source_sha256': inputs(), 'commands': []}
    write(report)
    commands = [
        ('export', ['python3', 'tools/config/export_configs.py', '--version', 'dev-warehouses-v0.23']),
        ('sync-bindings', ['python3', 'tools/config/sync_bindings.py']),
        ('fixture', ['python3', 'tools/resources/prepare_first_loop.py']),
        ('pet-candidates', ['python3', 'tools/resources/prepare_pet_runtime.py']),
        ('summon-candidates', ['python3', 'tools/resources/prepare_summon_candidates.py']),
        ('export-check', ['python3', 'tools/config/export_configs.py', '--check']),
        ('fixture-check', ['python3', 'tools/resources/prepare_first_loop.py', '--check']),
        ('pet-check', ['python3', 'tools/resources/prepare_pet_runtime.py', '--check']),
        ('summon-check', ['python3', 'tools/resources/prepare_summon_candidates.py', '--check']),
        ('bindings-check', ['node', 'tools/config/verify_bindings.mjs']),
        ('python-full', ['python3', '-m', 'unittest', 'discover', '-s', 'tools/config', '-p', 'test_*.py'])]
    for name, arguments in commands:
        started = time.monotonic()
        result = subprocess.run(DEV + arguments, cwd=ROOT, capture_output=True)
        stdout, stderr = OUT / (name + '.stdout.txt'), OUT / (name + '.stderr.txt')
        stdout.write_bytes(result.stdout)
        stderr.write_bytes(result.stderr)
        row = {'name': name, 'command': DEV + arguments, 'exit_code': result.returncode,
               'seconds': time.monotonic() - started,
               'stdout': stdout.relative_to(ROOT).as_posix(), 'stdout_sha256': sha(stdout),
               'stderr': stderr.relative_to(ROOT).as_posix(), 'stderr_sha256': sha(stderr)}
        report['commands'].append(row)
        write(report)
        if result.returncode:
            print(json.dumps({'passed': False, 'failed_command': name, 'exit_code': result.returncode}, ensure_ascii=False))
            return result.returncode
    report['input_source_sha256_after'] = inputs()
    assert report['input_source_sha256'] == report['input_source_sha256_after'], '配置检查期间输入变化'
    match = re.search(rb'Ran (\d+) tests?', (OUT / 'python-full.stderr.txt').read_bytes())
    assert match
    report['python_tests'] = int(match[1])
    report['passed'] = True
    report['finished_at_utc'] = datetime.now(timezone.utc).isoformat()
    report['generated_sha256'] = {name: sha(ROOT / name) for name in ['config/development/first-loop.json',
        'client/assets/resources/development/gameplay.json', 'config/development/pet-runtime-candidates.json',
        'config/development/summon-candidates.json', 'config/releases/preview_current.json', 'config/reports/配置校验报告.json']}
    write(report)
    print(json.dumps({'passed': True, 'python_tests': report['python_tests'],
                      'commands': len(report['commands']), 'source_inputs': len(report['input_source_sha256'])}, ensure_ascii=False))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
