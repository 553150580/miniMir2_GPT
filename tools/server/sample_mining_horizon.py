#!/usr/bin/env python3
"""使用工作簿投影、独立副本集和真实游戏连接实测同墙钟的矿战与纯战斗。"""

import argparse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from hashlib import sha256
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
from uuid import uuid4

import run_integration_batches as matrix
import mining_horizon_report as ledger


STUDIES = matrix.ROOT / '.work/mining-horizon'
DEV = [sys.executable, str(matrix.ROOT / 'tools/dev')]
CASE = 'TestMiningHorizonMongoWS'
TAGS = 'm4_economy_sampling,m1_natural_upgrade'


def freeze_binary(directory, source, compiler):
    """复用同源码的已验证程序，独立留存本次执行的原件。"""
    slot = STUDIES / 'build'
    slot.mkdir(exist_ok=True)
    binary = slot / 'mining-horizon.test.exe'
    note = slot / 'manifest.json'
    build_key = sha256(json.dumps([TAGS, compiler, source], sort_keys=True).encode('utf-8')).hexdigest()
    prior = json.loads(note.read_bytes()) if note.exists() else {}
    if prior.get('build_key') != build_key or not binary.exists() or ledger.digest(binary) != prior.get('sha256'):
        candidate = slot / 'mining-horizon.next.exe'
        with (slot / 'build.log').open('wb') as log:
            subprocess.run([*DEV, 'go', 'test', '-c', '-race', f'-tags={TAGS}', '-o', str(candidate), './internal/integration'],
                           cwd=matrix.ROOT, stdout=log, stderr=log, check=True, timeout=180)
        if matrix.source_snapshot() != source:
            raise ValueError('构建时源码变化，无法冻结本次实际程序')
        candidate.replace(binary)
        matrix.save(note, {'build_key': build_key, 'sha256': ledger.digest(binary)})
    frozen = directory / binary.name
    shutil.copy2(binary, frozen)
    return frozen


def run(args):
    """按本金独立运行同一场景，两批都使用原秒数和实际服务端。"""
    directory = STUDIES / uuid4().hex
    directory.mkdir(parents=True)
    print(f'矿战对照证据目录 {directory}', flush=True)
    controls = args.capital or ['one_tool', 'full']
    source = matrix.source_snapshot()
    compiler = subprocess.check_output([*DEV, 'go', 'version'], cwd=matrix.ROOT, text=True).strip()
    binary = freeze_binary(directory, source, compiler)
    commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=matrix.ROOT, text=True).strip()
    manifest = {'seconds': args.seconds, 'capital': controls, 'base_commit': commit, 'source_sha256': source,
                'compiler': compiler, 'binary_sha256': ledger.digest(binary), 'started_at': datetime.now(timezone.utc).isoformat()}
    matrix.save(directory / 'manifest.json', manifest)

    def test_command(expression, limit):
        """读取已冻结测试程序的结构化结果，支持配对双方实际同时运行。"""
        return [*DEV, '--cwd', 'server/internal/integration', 'go', 'tool', 'test2json', '-t', '-p',
                'minimir/internal/integration', str(binary), '-test.v=test2json', '-test.count=1', f'-test.run={expression}',
                '-test.parallel=24', f'-test.timeout={limit}s']

    def run_capital(capital):
        """每个本金批次拥有独立端点和相同工作簿配置。"""
        output = directory / capital
        output.mkdir()
        report = matrix.run_batch(output, 'mining', [CASE], args.seconds + 260, test_command=test_command,
                                  sample_env={'MINIMIR_MINING_HORIZON_SECONDS': str(args.seconds), 'MINIMIR_MINING_HORIZON_CAPITAL': capital})
        print(f'{capital} 验收通过={report["passed"]}', flush=True)
        return report

    with ThreadPoolExecutor(max_workers=len(controls)) as pool:
        reports = list(pool.map(run_capital, controls))
    passed = (all(row['passed'] and row['mongo_cleaned'] for row in reports) and ledger.digest(binary) == manifest['binary_sha256']
              and len({row.get('mining_fixture_sha256') for row in reports}) == 1)
    matrix.save(directory / 'result.json', {'passed': passed, 'seconds': args.seconds, 'capital': dict(zip(controls, reports))})
    print(f'矿战长窗结果 {directory / "result.json"}', flush=True)
    if passed:
        replay(directory)
    return 0 if passed else 1


def replay(path):
    """按原样本从工作目录复算，不启动任何服务。"""
    directory = path.resolve(strict=True)
    if directory.parent != STUDIES.resolve(strict=True) or directory.name == 'build':
        raise ValueError('只允许复核项目 .work/mining-horizon 中的本次样本目录')
    output = ledger.summarize(directory)
    print(f'{output["pairs"]} 对实际矿战和普通战斗原账本已复算 {directory / "summary.json"}', flush=True)


def main():
    """校验当前工具和采样输入，失败时保留原进程结果及证据路径。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--seconds', type=int, default=900)
    parser.add_argument('--capital', action='append', choices=('one_tool', 'full'))
    parser.add_argument('--replay', type=Path, help='从已完成的独立运行目录复核')
    args = parser.parse_args()
    if os.name != 'nt' or os.environ.get('MINIMIR_TOOLCHAIN_ACTIVE') != '1' or not 60 <= args.seconds <= 900:
        parser.error('请使用项目工具链 tools/dev.ps1，选取 60 至 900 秒墙钟')
    if args.capital and len(args.capital) != len(set(args.capital)):
        parser.error('相同的本金批次只能选择一次')
    try:
        if args.replay:
            replay(args.replay)
            return 0
        return run(args)
    except (OSError, ValueError, KeyError, subprocess.SubprocessError) as error:
        print(error, file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
