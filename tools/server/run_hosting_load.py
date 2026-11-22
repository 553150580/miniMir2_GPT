"""在隔离 MongoDB 和回环端口运行个人图挂机容量测试。"""

import argparse
import datetime as dt
import json
import os
from pathlib import Path
import re
import socket
import subprocess
import sys
import time
import uuid

from verify_startup_recovery import ROOT, fixture_identity, http_json, load, main_identity, save, sha, stop_owned

sys.path.insert(0, str(ROOT / 'tools' / 'environment'))
import mongodb_hosting_load as mongo
from process_control import background_options, windows_identity
from windows_process_samples import ProcessSamples


def progress(message):
    print(message, flush=True)


def ports(count, excluded):
    listeners = []
    try:
        selected = set(excluded)
        result = []
        while len(result) < count:
            listener = socket.socket()
            listeners.append(listener)
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_EXCLUSIVEADDRUSE, 1)
            listener.bind(('127.0.0.1', 0))
            port = listener.getsockname()[1]
            if port not in selected and not 18080 <= port <= 18102:
                selected.add(port)
                result.append(port)
        return result
    finally:
        for listener in listeners:
            listener.close()


def launch(argv, stdout, handles, interactive=False):
    with stdout.open('xb') as output, stdout.with_suffix('.stderr.log').open('xb') as errors:
        child = subprocess.Popen(
            argv, cwd=ROOT, stdin=subprocess.PIPE if interactive else subprocess.DEVNULL,
            stdout=output, stderr=errors, **background_options(),
        )
    handle = {'binary': argv[0], 'argv': argv, 'child': child, 'meta': windows_identity(child.pid)}
    handles.append(handle)
    if not handle['meta']:
        raise RuntimeError(f'压测子进程身份未登记，日志位于 {stdout}')
    return handle


def stop(handle):
    child = handle['child']
    if child.poll() is None and child.stdin is not None:
        child.stdin.close()
        try:
            child.wait(timeout=25)
        except subprocess.TimeoutExpired:
            stop_owned(handle)
    elif child.poll() is None:
        stop_owned(handle)


def wait_healthy(handle, port, mode, fixture):
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline and handle['child'].poll() is None:
        try:
            result = http_json(f'127.0.0.1:{port}')
        except (OSError, ValueError):
            time.sleep(0.15)
            continue
        if result.get('status') == 'ok':
            if any(result.get(name) != value for name, value in {
                'mode': mode, 'rule_version': fixture['rule_version'], 'fixture_hash': fixture['fixture_hash'],
            }.items()):
                raise RuntimeError(f'压测节点 {mode} 身份或规则校验失败')
            return result
    raise RuntimeError(f'压测节点 {mode} 未就绪，请查看独立日志')


def show_event(row):
    event = row.get('event')
    if event == 'seed':
        progress(f"已预置 {row.get('roles')} 个独立角色，耗时 {row.get('seconds', 0):.1f} 秒")
    elif event == 'default_admission':
        progress(f"默认接入 {row.get('first_128', {}).get('online')} 个，"
                 f"第 129 个结果 {row.get('connection_129')}")
    elif event == 'soak_sample':
        server = row.get('server', {})
        elapsed = row.get('elapsed_seconds', 0)
        census = row.get('census', {})
        rooms = server.get('rooms', {})
        progress(f"混合负载 {elapsed:.0f} 秒，在线 {rooms.get('online', census.get('online'))}，"
                 f"托管 {rooms.get('managed')}，总房间 {rooms.get('total')}，"
                 f"累计回收 {server.get('room_evictions')}")
    elif event in {'admission', 'hold_start', 'hold_sample', 'hold_end'}:
        census = row.get('census', row.get('current', {}))
        stage_name = {'admission': '入场', 'hold_start': '开始观察', 'hold_sample': '观察中', 'hold_end': '观察结束'}[event]
        progress(f"{row.get('target')} 人阶段{stage_name}，"
                 f"在线 {census.get('online')}，存活自动战斗 {census.get('auto_alive')}，"
                 f"累计掉线 {census.get('disconnects')}，累计收益结算 {census.get('settlements')}")


def wait_logged(handle, path, timeout, stage, sample=None):
    rows = []
    last_progress = None
    child = handle['child']
    deadline = time.monotonic() + timeout
    sample_due = time.monotonic()
    with path.open('rb') as source:
        while True:
            done = child.poll() is not None
            while True:
                offset = source.tell()
                line = source.readline()
                if not line:
                    break
                if not line.endswith(b'\n') and not done:
                    source.seek(offset)
                    break
                row = json.loads(line)
                rows.append(row)
                show_event(row)
                last_progress = time.monotonic()
            now = time.monotonic()
            if sample and now >= sample_due:
                sample()
                sample_due = now + 5
            if done:
                if child.returncode:
                    raise RuntimeError(f'{stage}失败（退出码 {child.returncode}），请查看 {path.with_suffix(".stderr.log")}')
                return rows
            if now >= deadline:
                raise RuntimeError(f'{stage}超过 {timeout} 秒，保留日志')
            if last_progress is None or now - last_progress >= 10:
                progress(f'{stage}进行中')
                last_progress = now
            time.sleep(min(0.4, deadline - now))


def run(args, hold_seconds):
    if os.name != 'nt' or os.environ.get('MINIMIR_TOOLCHAIN_ACTIVE') != '1':
        raise RuntimeError('请通过 ./tools/dev.ps1 在 Windows 锁定工具环境下运行')
    binary = args.binary.resolve(strict=True)
    if not binary.is_file() or binary.suffix.lower() != '.exe' or not binary.is_relative_to(ROOT):
        raise RuntimeError('请提供位于本项目内的压测专用 Windows 可执行文件')
    startup_config = ROOT / 'conf/dev/server.json'
    fixture = fixture_identity(load(startup_config), startup_config)
    run_id = uuid.uuid4().hex
    mongo_dir = mongo.run_directory(run_id)
    work = mongo_dir.parent
    work.mkdir(parents=True)
    db_record, _ = mongo.installation()
    report = {
        'run_id': run_id, 'directory': str(work), 'binary_sha256': sha(binary), 'fixture': fixture,
        'roles': args.seed, 'parallel': args.parallel, 'hold_seconds': hold_seconds,
        'logical_processors': os.cpu_count(), 'started_at': dt.datetime.now(dt.timezone.utc).isoformat(),
        'profiles': {}, 'cleanup_errors': [],
    }
    handles = []
    used_ports = set()
    progress(f'本轮独立压测证据位于 {work}')
    try:
        if args.profile == 'soak':
            report['main_before'] = main_identity()
        endpoint = mongo.start(db_record, mongo_dir, run_id)
        report['mongo'] = endpoint
        if not endpoint['endpoint_verified']:
            raise RuntimeError('本轮 MongoDB 端点未核验')
        used_ports.add(endpoint['port'])
        progress(f"独立 MongoDB {endpoint['version']} 已就绪，进程 {endpoint['pid']}")

        def command(mode, profile, addresses):
            account, game, diagnostic = addresses
            return [str(binary), '-mode', mode, '-mongo', endpoint['uri'], '-run', run_id,
                    '-account', str(account), '-game', str(game), '-diagnostic', str(diagnostic),
                    '-seed', str(args.seed), '-profile', profile, '-hold', args.hold,
                    '-parallel', str(args.parallel)]

        selected = ('default', 'raised') if args.profile == 'both' else (args.profile,)
        addresses = ports(3, used_ports)
        path = work / 'seed.jsonl'
        seed = launch(command('seed', selected[0], addresses), path, handles)
        seed_rows = wait_logged(seed, path, 180, '预置角色')
        seeded = seed_rows[-1] if seed_rows else {}
        expected = seeded.get('target_active', seeded.get('roles'))
        if expected != args.seed or seeded.get('roles', 0) < args.seed:
            raise RuntimeError('预置角色结果与目标不一致')
        report['seed'] = seed_rows[-1]

        with (work / 'process-samples.jsonl').open('x', encoding='utf-8', newline='\n', buffering=1) as samples:
            for index, profile in enumerate(selected):
                if index:
                    addresses = ports(3, used_ports)
                used_ports.update(addresses)
                progress(f'启动 {profile} 配额的独立服务')
                account = launch(command('account', profile, addresses), work / f'account-{profile}.log',
                                 handles, interactive=True)
                account_ready = wait_healthy(account, addresses[0], 'account', fixture)
                game = launch(command('game', profile, addresses), work / f'game-{profile}.log',
                              handles, interactive=True)
                game_ready = wait_healthy(game, addresses[1], 'game', fixture)
                clients_path = work / f'clients-{profile}.jsonl'
                clients = launch(command('clients', profile, addresses), clients_path, handles)
                processes = {name: handle['child'].pid for name, handle in
                             [('account', account), ('game', game), ('clients', clients)]}
                processes['mongo'] = endpoint['pid']
                counters = ProcessSamples(processes)

                def sample():
                    row = {'profile': profile, 'sampled_at': dt.datetime.now(dt.timezone.utc).isoformat(),
                           'processes': counters.read()}
                    samples.write(json.dumps(row) + '\n')

                try:
                    rows = wait_logged(clients, clients_path, hold_seconds + 600, f'{profile} 配额压测', sample)
                finally:
                    counters.close()
                if game['child'].poll() is not None or account['child'].poll() is not None:
                    raise RuntimeError('本轮服务在压测结束前退出')
                report['profiles'][profile] = {
                    'ports': dict(zip(('account', 'game', 'diagnostic'), addresses)),
                    'pids': processes, 'health': [account_ready, game_ready],
                    'events': str(clients_path), 'last_event': rows[-1] if rows else None,
                }
                stop(game)
                stop(account)
                if game['child'].returncode != 0 or account['child'].returncode != 0:
                    raise RuntimeError('压测服务正常退出时返回错误，请查看该阶段独立日志')
    except BaseException as error:
        report['error'] = str(error)
        raise
    finally:
        for handle in reversed(handles):
            try:
                stop(handle)
            except (OSError, RuntimeError, subprocess.SubprocessError) as error:
                report['cleanup_errors'].append(str(error))
        try:
            report['mongo_cleanup'] = mongo.stop(db_record, mongo_dir, run_id)
        except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as error:
            report['cleanup_errors'].append(str(error))
        if 'main_before' in report:
            try:
                report['main_after'] = main_identity()
                report['main_preserved'] = report['main_after'] == report['main_before']
            except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as error:
                report['main_check_error'] = str(error)
        report['ended_at'] = dt.datetime.now(dt.timezone.utc).isoformat()
        save(work / 'runner.json', report)
        progress(f'证据已保存至 {work / "runner.json"}')
    if report['cleanup_errors']:
        raise RuntimeError('隔离负载资源未全部清理，请核对报告')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--hold', default='180s')
    parser.add_argument('--seed', type=int, default=5000)
    parser.add_argument('--parallel', type=int, default=16)
    parser.add_argument('--profile', choices=['both', 'default', 'raised', 'prejoin', 'soak'], default='both')
    args = parser.parse_args()
    duration = re.fullmatch(r'([1-9]\d*)([sm])', args.hold)
    hold_seconds = int(duration[1]) * (60 if duration[2] == 'm' else 1) if duration else 0
    if not 10 <= hold_seconds <= 3600 or not 129 <= args.seed <= 5000 or not 1 <= args.parallel <= 48:
        parser.error('保压时长须为10秒至60分钟，角色数129至5000，并行数1至48')
    try:
        run(args, hold_seconds)
        return 0
    except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as error:
        print(str(error), file=sys.stderr, flush=True)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
