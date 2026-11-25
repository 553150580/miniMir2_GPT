"""用独立 MongoDB、副本集归属核验和实际 Creator 构建验收镜像竞技。"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import uuid

ROOT = Path(__file__).resolve().parents[2]
sys.path[:0] = [str(ROOT / 'tools'), str(ROOT / 'tools/environment')]
import mongodb_hosting_load as mongo


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--web-root', required=True, type=Path)
    parser.add_argument('--fixture', required=True, type=Path)
    parser.add_argument('--season', action='store_true', help='验收已结转赛季、奖励和隔离兑换')
    args = parser.parse_args()
    work = ROOT / '.work'
    build, fixture = args.web_root.resolve(), args.fixture.resolve()
    if os.name != 'nt' or os.environ.get('MINIMIR_TOOLCHAIN_ACTIVE') != '1':
        parser.error('请从 Windows 项目锁定工具链运行')
    if not all(source.is_relative_to(work) for source in (build, fixture)) or not (
            build.joinpath('index.html').is_file() and fixture.is_file()):
        parser.error('构建和规则输入须为 .work 下的独立候选')

    identity = uuid.uuid4().hex
    output = work / 'mirror-browser' / identity
    output.mkdir(parents=True)
    print(f'镜像竞技隔离验收目录 {output}', flush=True)
    directory = mongo.run_directory(identity)
    installation, _ = mongo.installation()
    scope = 'season' if args.season else 'battle'
    report = dict(passed=False, mongo_cleaned=False, scope=scope, build=str(build), fixture=str(fixture),
                  fixture_sha256=hashlib.sha256(fixture.read_bytes()).hexdigest(),
                  index_sha256=hashlib.sha256(build.joinpath('index.html').read_bytes()).hexdigest())
    try:
        endpoint = mongo.start(installation, directory, identity)
        if not endpoint['endpoint_verified'] or not endpoint['ownership_verified'] or endpoint['port'] in (27118, 27119):
            raise RuntimeError('镜像触屏专项副本集端点或所有权核验失败')
        report['mongo'] = {key: endpoint[key] for key in ('version', 'port', 'replica_set')}
        env = dict(os.environ, MINIMIR_MONGO_E2E='1', MINIMIR_INTEGRATION_TEST_MONGO_URI=endpoint['uri'],
                   MINIMIR_MIRROR_BROWSER_WEB_ROOT=str(build), MINIMIR_MIRROR_BROWSER_FIXTURE=str(fixture))
        case = 'TestMirrorSeasonCreatorBrowserMongoWS' if args.season else 'TestMirrorArenaCreatorBrowserMongoWS'
        command = [sys.executable, str(ROOT / 'tools/dev'), 'go', 'test', '-tags=creator_mirror_arena', '-v',
                   './internal/integration', '-run', f'^{case}$', '-count=1', '-timeout=260s']
        log = output / 'go-test.log'
        with log.open('wb') as stream:
            done = subprocess.run(command, cwd=ROOT, env=env, stdout=stream, stderr=subprocess.STDOUT, timeout=280)
        report['exit_code'] = done.returncode
        report_prefix = '镜像赛季触屏证据目录' if args.season else '镜像触屏验收证据目录'
        found = re.search(report_prefix + r'\s+([^\r\n]+)', log.read_text(encoding='utf-8'))
        if found:
            name = 'mirror-season-browser-report.json' if args.season else 'mirror-arena-browser-report.json'
            browser = Path(found[1]) / name
            report['browser_report'] = str(browser)
            report['passed'] = done.returncode == 0 and browser.is_file() and json.loads(browser.read_text(encoding='utf-8'))['passed']
    except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as error:
        report['error'] = str(error)
    finally:
        if directory.exists():
            try:
                stopped = mongo.stop(installation, directory, identity)
                report['mongo_cleaned'] = not stopped['running'] and stopped['data_removed']
            except (OSError, RuntimeError) as error:
                report['cleanup_error'] = str(error)
        report['passed'] = report['passed'] and report['mongo_cleaned']
        output.joinpath('result.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(f'镜像竞技隔离验收通过={report["passed"]}', flush=True)
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
