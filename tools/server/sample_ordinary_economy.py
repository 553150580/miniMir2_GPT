#!/usr/bin/env python3
"""使用项目专属 MongoDB 和真实 WebSocket 采样普通挂机的可复算账户收益。"""

import argparse
from datetime import datetime, timezone
import json
import math
import os
from pathlib import Path
import subprocess
import sys
import uuid

import ordinary_economy_report as ledger
import run_integration_batches as matrix


ROOT = matrix.ROOT
WORK = ROOT / '.work'
STUDIES = WORK / 'economy-sampling'
DEFAULT = ROOT / 'config/development/first-loop.json'
CASE = 'TestOrdinaryEconomyMatrixMongoWS'
DEV = [sys.executable, str(ROOT / 'tools/dev')]


def read(path):
    """读取已保存的本次隔离运行文件。"""
    return json.loads(path.read_bytes())


def safe_directory(path):
    """只接受此采样工具的项目盘目录，避免称其他环境的报告为本轮证据。"""
    resolved = path.resolve(strict=True)
    if resolved.parent != STUDIES.resolve(strict=True) or not resolved.is_dir():
        raise ValueError('只允许复核项目 .work/economy-sampling 下的独立运行目录')
    return resolved


def replay(directory):
    """从该次 MongoDB 已清退的原证据重新产生汇总。"""
    directory = safe_directory(directory)
    manifest = read(directory / 'manifest.json')
    test = read(directory / 'ordinary-economy/result.json')
    if not test['passed'] or not test['mongo_cleaned']:
        raise ValueError('原隔离联机用例或副本集清退尚未通过')
    if 'test_binary_sha256' in manifest and (
            ledger.digest(directory / 'ordinary-economy.test.exe') != manifest['test_binary_sha256'] or
            ledger.digest(directory / 'fixture.json') != manifest['fixture_sha256']):
        raise ValueError('已冻结采样程序或配置摘要不匹配')
    summary = ledger.summarize(directory / 'samples', manifest)
    matrix.save(directory / 'summary.json', {'passed': True, **summary})
    print(f'{summary["verified_samples"]} 个样本已逐笔复算，结果 {directory / "summary.json"}', flush=True)


def compile_sampler(directory, manifest):
    """冻结实际运行的测试程序，长窗期间仍可继续独立开发工作区。"""
    binary = directory / 'ordinary-economy.test.exe'
    with (directory / 'compile.log').open('wb') as log:
        subprocess.run([*DEV, 'go', 'test', '-c', '-race', '-tags=m4_economy_sampling,m1_natural_upgrade',
                        '-o', str(binary), './internal/integration'], cwd=ROOT, stdout=log, stderr=log,
                       check=True, timeout=180)
    if matrix.source_snapshot() != manifest['source_sha256']:
        raise ValueError('采样程序构建期间源文件变化')
    manifest['test_binary_sha256'] = ledger.digest(binary)
    return binary


def run(args):
    """启动一次完整测试，按本次试验时长组织并发角色并保存临时副本集报告。"""
    if not 30 <= args.seconds <= 86400 or not 1 <= args.parallel <= 108:
        raise ValueError('采样时长须为 30 至 86400 秒，并发须为 1 至 108')
    fixture = args.fixture.resolve(strict=True)
    if fixture != DEFAULT and not fixture.is_relative_to(WORK.resolve()):
        raise ValueError('候选夹具必须来自项目隔离工作目录')
    selection = {'seconds': args.seconds, 'map_ids': args.map_ids or [args.map_id], 'mode': args.mode, 'levels': args.levels or [args.level],
                 'jobs': args.jobs, 'monsters': args.monsters, 'kits': args.kits,
                 'cards': args.cards, 'repeat': args.repeat, 'maintenance': args.maintenance}
    axes = ('levels', 'map_ids', 'jobs', 'monsters', 'kits', 'cards')
    if any(len(selection[key]) != len(set(selection[key])) for key in axes):
        raise ValueError('采样等级、地图或角色其他条件出现重复')
    size = math.prod(len(selection[key]) for key in axes) * args.repeat
    directory = STUDIES / uuid.uuid4().hex
    samples = directory / 'samples'
    samples.mkdir(parents=True)
    code = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    before = matrix.source_snapshot()
    isolated_fixture = directory / 'fixture.json'
    isolated_fixture.write_bytes(fixture.read_bytes())
    manifest = {'selection': selection, 'roles': size, 'parallel': args.parallel, 'base_commit': code,
                'fixture_sha256': ledger.digest(isolated_fixture), 'fixture_file': str(fixture), 'source_sha256': before,
                'launcher_sha256': ledger.digest(Path(__file__)), 'reporter_sha256': ledger.digest(Path(ledger.__file__)),
                'runner_pid': os.getpid(), 'started_at': datetime.now(timezone.utc).isoformat()}
    matrix.save(directory / 'manifest.json', manifest)
    print(f'准备实际经济采样 {directory}，计划 {size} 个角色，每个 {args.seconds} 秒', flush=True)
    binary = compile_sampler(directory, manifest)
    matrix.save(directory / 'manifest.json', manifest)
    os.environ.update(MINIMIR_ECONOMY_FIXTURE=str(isolated_fixture), MINIMIR_ECONOMY_SELECTION=json.dumps(selection),
                      MINIMIR_ECONOMY_SAMPLES=str(samples))
    print(f'开始真实经济采样 {directory}，计划 {size} 个角色，每个 {args.seconds} 秒', flush=True)
    # 波次按实际墙钟长度调配。组队停战时间属于采样，额外时间留给正常商店动作与逐笔复算。
    timeout = math.ceil(size / args.parallel) * (args.seconds + 60) + (
        size * 15 if args.maintenance else 60) + 180

    def test_command(expression, limit):
        """用 Go 的结构化测试事件记录已冻结程序的实际结果。"""
        return [*DEV, '--cwd', 'server/internal/integration', 'go', 'tool', 'test2json', '-t', '-p', 'minimir/internal/integration',
                str(binary), '-test.v=test2json', '-test.count=1', f'-test.run={expression}',
                f'-test.parallel={args.parallel}', f'-test.timeout={limit}s']

    result = matrix.run_batch(directory, 'ordinary-economy', [CASE], timeout, test_command=test_command)
    if not result['passed']:
        raise ValueError(f'联机采样失败，保留报告 {directory / "ordinary-economy" / "result.json"}')
    if ledger.digest(binary) != manifest['test_binary_sha256'] or ledger.digest(isolated_fixture) != manifest['fixture_sha256']:
        raise ValueError('采样期间已冻结的程序或夹具发生变化，保留细账供定位')
    replay(directory)


def main():
    """选择实验维度或复算一轮既有证据。"""
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='mode_command', required=True)
    trial = sub.add_parser('run', help='启动可追踪的真实联机采样')
    trial.add_argument('--seconds', type=int, required=True)
    trial.add_argument('--mode', choices=('online', 'hosting'), default='online')
    maps = trial.add_mutually_exclusive_group()
    maps.add_argument('--map-id', type=int, default=1)
    maps.add_argument('--map-ids', nargs='+', type=int)
    levels = trial.add_mutually_exclusive_group()
    levels.add_argument('--level', type=int, default=15)
    levels.add_argument('--levels', nargs='+', type=int)
    trial.add_argument('--fixture', type=Path, default=DEFAULT)
    trial.add_argument('--jobs', nargs='+', type=int, choices=(0, 1, 2), default=[0, 1, 2])
    trial.add_argument('--monsters', nargs='+', type=int, choices=(1, 4, 8), default=[1, 4, 8])
    trial.add_argument('--kits', nargs='+', choices=('low', 'mid', 'high'), default=['low', 'mid', 'high'])
    trial.add_argument('--cards', nargs='+', choices=('none', 'privilege', 'hosting', 'both'),
                       default=['none', 'privilege', 'hosting', 'both'])
    trial.add_argument('--repeat', type=int, choices=(1, 2, 3), default=1)
    trial.add_argument('--parallel', type=int, default=12)
    trial.add_argument('--maintenance', action='store_true', help='收窗后走真实商人，补库存和修装备并对账')
    sub.add_parser('replay', help='在数据库清退后从同份证据复算').add_argument('directory', type=Path)
    options = parser.parse_args()
    if os.name != 'nt' or os.environ.get('MINIMIR_TOOLCHAIN_ACTIVE') != '1':
        parser.error('请使用项目已锁定的 Windows 工具链 ./tools/dev.ps1')
    try:
        if options.mode_command == 'run':
            run(options)
        else:
            replay(options.directory)
        return 0
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError) as error:
        print(error, file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
