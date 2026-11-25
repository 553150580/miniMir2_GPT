"""在独立副本集上验收区服目录、双节点四服入场及路由隔离。"""

import argparse
import hashlib
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


def verify(web_root, world_boss_browser=False, world_boss_cold_entry_only=False, world_boss_rewards_browser=False):
    run_id = uuid.uuid4().hex
    data_dir = mongo.run_directory(run_id)
    report_dir = ROOT / '.work/server-directory' / run_id
    report_dir.mkdir(parents=True)
    installation, _ = mongo.installation()
    report = {'run_id': run_id, 'browser': web_root is not None, 'world_boss_browser': world_boss_browser,
              'world_boss_cold_entry_only': world_boss_cold_entry_only, 'world_boss_rewards_browser': world_boss_rewards_browser, 'passed': False}
    if web_root:
        report['web_root'] = str(web_root)
        report['web_index_sha256'] = hashlib.sha256((web_root / 'index.html').read_bytes()).hexdigest()
        report['web_main_bundle_sha256'] = hashlib.sha256((web_root / 'assets/main/index.js').read_bytes()).hexdigest()
    try:
        endpoint = mongo.start(installation, data_dir, run_id)
        report['mongo'] = {'version': endpoint['version'], 'pid': endpoint['pid'], 'port': endpoint['port']}
        if world_boss_rewards_browser:
            report['mongo']['replica_set'] = endpoint['replica_set']
        if not endpoint['endpoint_verified'] or not endpoint['ownership_verified']:
            raise RuntimeError('本轮副本集端点和进程身份核验失败')
        print(f'独立 MongoDB PID={endpoint["pid"]}，验收编号={run_id}', flush=True)
        environment = os.environ.copy()
        environment['MINIMIR_MONGO_E2E'] = '1'
        environment['MINIMIR_SERVER_DIRECTORY_TEST_MONGO_URI'] = endpoint['uri']
        environment.pop('MINIMIR_SERVER_DIRECTORY_WEB_ROOT', None)
        environment.pop('MINIMIR_WORLD_BOSS_BROWSER_WEB_ROOT', None)
        environment.pop('MINIMIR_WORLD_BOSS_COLD_ENTRY_ONLY', None)
        environment.pop('MINIMIR_WORLD_BOSS_REWARDS_BROWSER_WEB_ROOT', None)
        test = 'TestServerDirectoryTwoGameNodesMongoWS'
        if world_boss_rewards_browser:
            test = 'TestWorldBossRewardsBrowserMongoWS'
            environment['MINIMIR_WORLD_BOSS_REWARDS_BROWSER_WEB_ROOT'] = str(web_root)
            environment['MINIMIR_INTEGRATION_TEST_MONGO_URI'] = endpoint['uri']
        elif web_root:
            name = 'MINIMIR_WORLD_BOSS_BROWSER_WEB_ROOT' if world_boss_browser else 'MINIMIR_SERVER_DIRECTORY_WEB_ROOT'
            environment[name] = str(web_root)
        if world_boss_browser:
            test = 'TestWorldBossTwoClientBrowserMongoWS'
        if world_boss_cold_entry_only:
            environment['MINIMIR_WORLD_BOSS_COLD_ENTRY_ONLY'] = '1'
        command = [sys.executable, str(ROOT / 'tools/dev'), 'go', 'test', '-race', '-p', '1',
                   './internal/integration', '-run', f'^{test}$',
                   '-count=1', '-timeout=345s' if world_boss_browser else '-timeout=300s', '-v']
        result = subprocess.run(command, cwd=ROOT, env=environment, capture_output=True,
                                timeout=375 if world_boss_browser else 330)
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
    parser.add_argument('--web-root', type=Path, help='可选，加入四个区服的真实 Chromium 触屏验收')
    modes = parser.add_mutually_exclusive_group()
    modes.add_argument('--world-boss-browser', action='store_true', help='使用独立副本集验证战士和道士的世界首领触屏流程')
    modes.add_argument('--world-boss-rewards-browser', action='store_true', help='从真实世界首领击杀开始，验收触屏领取原实例')
    parser.add_argument('--world-boss-cold-entry-only', action='store_true', help='只测双端首次入场和跨职业真实施法')
    args = parser.parse_args()
    if os.name != 'nt' or os.environ.get('MINIMIR_TOOLCHAIN_ACTIVE') != '1':
        parser.error('请通过 ./tools/dev.ps1 在项目锁定的 Windows 工具链中运行')
    if (args.world_boss_browser or args.world_boss_rewards_browser) and args.web_root is None:
        parser.error('世界首领浏览器验收需要 --web-root 指定实际构建')
    if args.world_boss_cold_entry_only and not args.world_boss_browser:
        parser.error('首次入场专项需要 --world-boss-browser')
    return verify(browser_build(args.web_root), args.world_boss_browser, args.world_boss_cold_entry_only, args.world_boss_rewards_browser)


if __name__ == '__main__':
    raise SystemExit(main())
