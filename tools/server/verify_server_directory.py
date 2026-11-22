"""在独立副本集上验收账号区服目录、两个节点入场及路由隔离。"""

import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import uuid

ROOT = Path(__file__).resolve().parents[2]
sys.path[:0] = [str(ROOT / 'tools'), str(ROOT / 'tools/environment')]
import mongodb_hosting_load as mongo


def browser_build(path):
    """只接受工作目录中的独立构建，避免将实际试玩入口作为验收对象。"""
    if path is None:
        return None
    build = path.resolve(strict=True)
    if not build.is_relative_to(ROOT / '.work') or not (build / 'index.html').is_file():
        raise ValueError('浏览器验收须提供项目 .work 内的独立 Creator Web Mobile 构建')
    return build


def verify(web_root):
    run_id = uuid.uuid4().hex
    data_dir = mongo.run_directory(run_id)
    report_dir = ROOT / '.work/server-directory' / run_id
    report_dir.mkdir(parents=True)
    installation, _ = mongo.installation()
    report = {'run_id': run_id, 'browser': web_root is not None, 'passed': False}
    try:
        endpoint = mongo.start(installation, data_dir, run_id)
        report['mongo'] = {'version': endpoint['version'], 'pid': endpoint['pid'], 'port': endpoint['port']}
        if not endpoint['endpoint_verified'] or not endpoint['ownership_verified']:
            raise RuntimeError('本轮副本集端点和进程身份核验失败')
        print(f'独立 MongoDB PID={endpoint["pid"]}，验收编号={run_id}', flush=True)
        environment = os.environ.copy()
        environment['MINIMIR_MONGO_E2E'] = '1'
        environment['MINIMIR_SERVER_DIRECTORY_TEST_MONGO_URI'] = endpoint['uri']
        environment.pop('MINIMIR_SERVER_DIRECTORY_WEB_ROOT', None)
        if web_root:
            environment['MINIMIR_SERVER_DIRECTORY_WEB_ROOT'] = str(web_root)
        command = [sys.executable, str(ROOT / 'tools/dev'), 'go', 'test', '-race', '-p', '1',
                   './internal/integration', '-run', '^TestServerDirectoryTwoGameNodesMongoWS$',
                   '-count=1', '-timeout=300s', '-v']
        result = subprocess.run(command, cwd=ROOT, env=environment, capture_output=True, timeout=330)
        output = result.stdout + result.stderr
        (report_dir / 'test.log').write_bytes(output)
        print(output.decode('utf-8', errors='replace'), end='', flush=True)
        report['test_exit_code'] = result.returncode
    finally:
        try:
            state = mongo.stop(installation, data_dir, run_id)
            report['mongo_cleaned'] = not state['running'] and state['data_removed']
            report['passed'] = report.get('test_exit_code') == 0 and report['mongo_cleaned']
        finally:
            result_path = report_dir / 'result.json'
            result_path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n',
                                   encoding='utf-8')
            print(f'验收记录 {result_path}', flush=True)
    return 0 if report['passed'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--web-root', type=Path, help='可选，加入两个区服的真实 Chromium 触屏验收')
    args = parser.parse_args()
    if os.name != 'nt' or os.environ.get('MINIMIR_TOOLCHAIN_ACTIVE') != '1':
        parser.error('请通过 ./tools/dev.ps1 在项目锁定的 Windows 工具链中运行')
    return verify(browser_build(args.web_root))


if __name__ == '__main__':
    raise SystemExit(main())
