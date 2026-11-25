"""分批跑完整 Go 联机竞态回归，每批使用独立且可清理的 MongoDB 副本集。"""

import argparse
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
sys.path[:0] = [str(ROOT / 'tools'), str(ROOT / 'tools/environment')]
import mongodb_hosting_load as mongo


GROUP_PREFIXES = {
    'identity': ('Authentication', 'RoleBinding', 'Rule24', 'MongoOwnership', 'DormantRoom',
                 'RetiredRoom', 'SafeZone', 'ServerDirectory'),
    'inventory': ('AutoRecycle', 'Equipment', 'FullBagContinuous', 'LevelGrowthMigration',
                  'MongoPermanent', 'MongoReceipt', 'PotionMongo'),
    'warehouses': ('Warehouse', 'SkillAsset', 'SkillPageMerge'),
    'hosting': ('ManaPotion', 'AutoTarget', 'CompactTown', 'FirstLoop', 'FullBagOffline',
                'LevelGrowthNatural', 'MapTravel'),
    'scene-lifecycle': ('MonsterRespawn', 'SharedTown'),
    'skills': ('Combat', 'Displacement', 'PetRuntime', 'SkillLearning', 'SummonPet', 'Taoist', 'Teleport'),
    'bosses': ('PersonalBoss', 'SharedWorldBoss'),
}
# 子进程入口由父用例启动；浏览器专项另传真实 Creator 构建并独立生成报告。
EXTERNAL_CASES = {
    'TestServerDirectoryNodeProcess': '由四服联机父用例启动',
    'TestWorldBossTwoClientBrowserMongoWS': '由双客户端真实构建专项执行',
}
GO = [sys.executable, str(ROOT / 'tools/dev'), 'go', 'test']


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def suites():
    listing = subprocess.run([*GO, '-list', '^Test', './internal/integration'], cwd=ROOT, check=True,
                             capture_output=True, encoding='utf-8').stdout
    discovered = [name for name in listing.splitlines() if re.fullmatch(r'Test\w+', name)]
    if not discovered:
        raise RuntimeError('没有发现联机用例')
    groups = {group: [] for group in GROUP_PREFIXES}
    external = {}
    for name in discovered:
        if name in EXTERNAL_CASES:
            external[name] = EXTERNAL_CASES[name]
            continue
        match = [group for group, prefixes in GROUP_PREFIXES.items() if name[4:].startswith(prefixes)]
        if len(match) != 1:
            raise ValueError(f'联机用例 {name} 需要明确的单个验收分组')
        groups[match[0]].append(name)
    return groups, external


def source_snapshot():
    paths = [ROOT / 'server/go.mod', ROOT / 'server/go.sum', ROOT / 'conf/dev/server.json']
    paths.extend((ROOT / 'config/development').glob('*.json'))
    paths.extend((ROOT / 'server/internal').rglob('*.go'))
    paths.extend((ROOT / 'server/api/generated').rglob('*.go'))
    return {path.relative_to(ROOT).as_posix(): hashlib.sha256(path.read_bytes()).hexdigest() for path in sorted(paths)}


def run_batch(output, group, cases, timeout):
    identity = uuid.uuid4().hex
    mongo_directory = mongo.run_directory(identity)
    installation, _ = mongo.installation()
    report = {'batch': group, 'cases': cases, 'selected': len(cases), 'passed': False, 'mongo_cleaned': False}
    destination = output / group
    destination.mkdir()
    print(f'{group} 开始，{len(cases)} 个联机用例', flush=True)
    began = time.monotonic()
    try:
        endpoint = mongo.start(installation, mongo_directory, identity)
        if not endpoint['endpoint_verified'] or not endpoint['ownership_verified']:
            raise RuntimeError('独立 MongoDB 端点或所有权核验失败')
        report['mongo'] = {key: endpoint[key] for key in ('version', 'port', 'pid', 'replica_set')}
        environment = os.environ.copy()
        environment.update(
            MINIMIR_MONGO_E2E='1', MINIMIR_INTEGRATION_TEST_MONGO_URI=endpoint['uri'],
            MINIMIR_SERVER_DIRECTORY_TEST_MONGO_URI=endpoint['uri'],
        )
        environment.pop('MINIMIR_SERVER_DIRECTORY_WEB_ROOT', None)
        environment.pop('MINIMIR_WORLD_BOSS_BROWSER_WEB_ROOT', None)
        expression = '^(' + '|'.join(cases) + ')$'
        command = [*GO, '-json', '-race', '-p', '1', './internal/integration', '-run', expression,
                   '-count=1', f'-timeout={timeout}s']
        report['command'] = command
        log = destination / 'events.jsonl'
        with log.open('wb') as results, (destination / 'stderr.log').open('wb') as errors:
            result = subprocess.run(command, cwd=ROOT, env=environment, stdout=results, stderr=errors,
                                    timeout=timeout + 90)
        report['exit_code'] = result.returncode
        events = [json.loads(line) for line in log.read_text(encoding='utf-8').splitlines() if line.startswith('{')]
        report['passing'] = [e['Test'] for e in events if e.get('Test') and e['Action'] == 'pass']
        report['skipped'] = [e['Test'] for e in events if e.get('Test') and e['Action'] == 'skip']
        report['failing'] = [e.get('Test') or e.get('Package') for e in events if e['Action'] in ('fail', 'build-fail')]
        report['unverified'] = sorted(set(cases) - set(report['passing']))
        report['passed'] = result.returncode == 0 and not report['skipped'] and not report['unverified']
    except (OSError, RuntimeError, subprocess.SubprocessError, ValueError) as error:
        report['error'] = str(error)
    finally:
        if mongo_directory.exists():
            try:
                stopped = mongo.stop(installation, mongo_directory, identity)
                report['mongo_cleaned'] = not stopped['running'] and stopped['data_removed']
            except (OSError, RuntimeError) as error:
                report['cleanup_error'] = str(error)
        report['passed'] = report['passed'] and report['mongo_cleaned']
        report['seconds'] = round(time.monotonic() - began, 1)
        save(destination / 'result.json', report)
        print(f'{group} 完成，通过 {len(report.get("passing", []))} 项，耗时 {report["seconds"]} 秒，'
              f'验收通过={report["passed"]}', flush=True)
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--parallel', type=int, choices=(1, 2), default=2)
    parser.add_argument('--timeout', type=int, default=300, help='每个 Go 测试分组的总时限，单位秒')
    parser.add_argument('--group', action='append', choices=GROUP_PREFIXES, help='只跑指定分组，可重复指定')
    args = parser.parse_args()
    if os.name != 'nt' or os.environ.get('MINIMIR_TOOLCHAIN_ACTIVE') != '1' or not 30 <= args.timeout <= 900:
        parser.error('请通过 tools/dev.ps1 运行，单批总时限须为 30 至 900 秒')
    groups, external = suites()
    selected = [group for group in groups if not args.group or group in args.group]
    output = ROOT / '.work/integration-batches' / uuid.uuid4().hex
    output.mkdir(parents=True)
    print(f'分批联机验收目录 {output}', flush=True)
    before = source_snapshot()
    with ThreadPoolExecutor(max_workers=args.parallel) as workers:
        futures = [workers.submit(run_batch, output, group, groups[group], args.timeout) for group in selected]
        reports = [future.result() for future in futures]
    stable = source_snapshot() == before
    passed = stable and all(report['passed'] for report in reports)
    save(output / 'result.json', {
        'passed': passed, 'all_groups': selected == list(groups), 'source_stable': stable,
        'fixture_sha256': before['config/development/first-loop.json'],
        'source_sha256': before, 'selected': sum(report['selected'] for report in reports),
        'top_level_passes': sum(len(set(report.get('passing', [])) & set(report['cases'])) for report in reports),
        'external_cases': external, 'groups': reports,
    })
    print(f'分批联机验收报告 {output / "result.json"}', flush=True)
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
