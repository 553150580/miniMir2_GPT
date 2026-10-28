"""跨平台工程任务；与 Makefile 保持同名入口，逐项失败即停止并保存真实输出。"""
import argparse
import datetime
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
DEV = [sys.executable, str(ROOT / 'tools/dev')]
TASKS = {
    'client-scene-test': [
        ['node', 'tools/client/compile_scene_tests.mjs'],
        ['node', '--test', 'client/tests/scene/*.test.cjs']],
    'client-test': [
        ['node', 'client/tests/ui/compile_models.mjs'],
        ['node', '--test', 'client/tests/ui/*.test.cjs', 'client/tests/app/*.test.cjs']],
    'python-test': [['python3', '-m', 'unittest', 'discover', '-s', 'tools/config', '-p', 'test_*.py']],
    'go-test': [['go', 'test', './...']],
    'protocol-check': [['node', 'tools/protocol/verify_client_pb.cjs'], ['node', 'tools/config/verify_bindings.mjs']],
    'client-build': [['python3', 'tools/client/build.py']],
    'client-typecheck': [['node', 'tools/client/typecheck.mjs']],
    'client-browser-test': [['node', 'tools/client/verify_windows_browser.cjs']],
    'environment': [['python3', 'tools/environment/check.py', '--strict']],
    'protocol': [['python3', 'tools/config/generate_protocol.py'], ['python3', 'tools/protocol/generate_go_pb.py'], ['node', 'tools/protocol/generate_client_pb.cjs']],
    'references': [['python3', 'tools/reference/verify_copies.py'], ['python3', 'tools/reference/verify_pet_sources.py']],
    'server-integration-test': [['python3', 'tools/server/test.py', '--mongo', '--race']],
    'dev-start': [['python3', 'tools/environment/mongodb_dev.py', 'start'], ['python3', 'tools/server/dev.py', 'start'], ['python3', 'tools/client/dev.py', 'start']],
    'dev-stop': [['python3', 'tools/client/dev.py', 'stop'], ['python3', 'tools/server/dev.py', 'stop']],
    'dev-status': [['python3', 'tools/server/dev.py', 'status'], ['python3', 'tools/client/dev.py', 'status']],
    'check': [['python3', 'tools/reference/inventory.py', '--check'], ['python3', 'tools/reference/prepare_local.py', '--check'], ['python3', 'tools/reference/verify_pet_sources.py'], ['python3', 'tools/config/export_configs.py', '--check'], ['python3', 'tools/config/generate_protocol.py', '--check']],
    'preview': [['python3', 'tools/config/export_configs.py'], ['python3', 'tools/config/sync_bindings.py']],
    'release-check': [['python3', 'tools/config/export_configs.py', '--check', '--mode', 'release']],
    'development-resources-check': [['python3', 'tools/resources/build_development_assets.py', '--check'], ['python3', 'tools/resources/prepare_first_loop.py', '--check']],
}
GROUPS = {'test': ['client-scene-test', 'client-test', 'python-test', 'go-test', 'protocol-check']}
if os.name == 'nt':
    # 原始来源树仍保留历史清单；迁移现场核验独立副本，不伪造原机目录或绕过摘要。
    TASKS['check'] = [['python3', 'tools/reference/verify_copies.py']] + TASKS['check'][2:]
    TASKS['development-resources-check'][0] = ['python3', 'tools/resources/verify_windows_assets.py']
    TASKS['host-test'] = [['python3', '-m', 'unittest', 'discover', '-s', 'tools/environment', '-p', 'test_*.py']]
    GROUPS['test'].insert(0, 'host-test')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('task', choices=sorted(set(TASKS) | set(GROUPS)))
    parser.add_argument('--report-dir', default='.work/tasks')
    args = parser.parse_args()
    directory = (ROOT / args.report_dir).resolve()
    if not directory.is_relative_to(ROOT):
        parser.error('报告必须保存在项目目录内')
    directory.mkdir(parents=True, exist_ok=True)
    for task in GROUPS.get(args.task, [args.task]):
        for index, command in enumerate(TASKS[task]):
            started = datetime.datetime.now(datetime.timezone.utc).isoformat()
            environment = os.environ.copy()
            if task == 'go-test':
                environment['MINIMIR_MONGO_E2E'] = '0'
            result = subprocess.run(DEV + command, cwd=ROOT, env=environment, capture_output=True)
            log = directory / f'{task}-{index}.txt'
            log.write_bytes(result.stdout + result.stderr)
            report = dict(task=task, command=command, exit_code=result.returncode,
                          started_at=started, finished_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                          output=log.relative_to(ROOT).as_posix(), scope='仅所列命令，不替代业务、真机或容量验收')
            (directory / f'{task}-{index}.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
            print(json.dumps(report, ensure_ascii=False), flush=True)
            if result.returncode:
                print((result.stdout + result.stderr).decode('utf-8', errors='replace')[-6000:])
                return result.returncode
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
