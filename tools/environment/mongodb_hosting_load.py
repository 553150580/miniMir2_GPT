"""启动并清理个人图挂机压测专属的 Windows 单成员 MongoDB 副本集。"""

import argparse
import json
import os
from pathlib import Path
import re
import shutil
import socket
import stat
import subprocess
import sys
import time

from mongodb_smoke import DRIVER_PROGRAM
from mongodb_windows import KEY, ROOT, command_line, installation, listener, read, save, settings
from process_control import background_options, windows_identity, windows_owned, windows_stop


WORK = ROOT / '.work' / 'hosting-load'
DEFAULT_STORAGE = 'workspace'
LOCAL_STORAGE = 'local-app-data'
OPLOG_MIB = 128
WIREDTIGER_CACHE_GIB = 0.25


def add_storage_argument(parser):
    parser.add_argument('--mongo-storage', choices=(DEFAULT_STORAGE, LOCAL_STORAGE), default=DEFAULT_STORAGE,
                        help='隔离库使用项目工作目录或项目专属本机应用数据目录')


def storage_root(storage):
    if storage == DEFAULT_STORAGE:
        return WORK
    if storage != LOCAL_STORAGE:
        raise ValueError('未知的压测Mongo存储方式')
    app_data = Path(os.environ.get('LOCALAPPDATA', ''))
    if not app_data.is_absolute() or not re.fullmatch(r'[A-Za-z]:', app_data.drive):
        raise RuntimeError('隔离库的本机应用数据目录须在本机绝对路径下')
    profile, _, _, _ = settings()
    version = profile['tools']['mongod']['version']
    if not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+', version):
        raise RuntimeError('锁定的Mongo版本不能作为隔离目录名')
    # 单独的固定根与项目键避免复用原试玩数据库或其它项目的隔离库。
    return app_data / 'minimir-mongodb-test' / 'windows' / version / KEY


def is_reparse_point(path):
    try:
        return bool(path.lstat().st_file_attributes & stat.FILE_ATTRIBUTE_REPARSE_POINT)
    except FileNotFoundError:
        return False


def run_directory(run_id, storage=DEFAULT_STORAGE):
    if not re.fullmatch(r'[a-f0-9]{32}', run_id):
        raise ValueError('压测运行编号须为32位小写十六进制')
    directory = storage_root(storage) / run_id / 'mongo'
    paths = [*reversed(directory.parents[:-1]), directory]
    paths.extend(directory / name for name in ('data', 'runtime.json', 'runtime.tmp', 'transaction-smoke.json',
                                               'transaction-smoke.tmp', 'mongod.log', 'console.log'))
    for path in paths:
        if is_reparse_point(path) or path.resolve() != path:
            raise RuntimeError('压测Mongo目录含有重解析点或越界')
    return directory


def require_run_directory(directory, run_id, storage):
    if directory != run_directory(run_id, storage):
        raise RuntimeError('压测Mongo目录与运行编号或存储方式不符')


def free_port():
    with socket.socket() as probe:
        probe.setsockopt(socket.SOL_SOCKET, socket.SO_EXCLUSIVEADDRUSE, 1)
        probe.bind(('127.0.0.1', 0))
        return probe.getsockname()[1]


def arguments(record, directory, run_id, port):
    if not isinstance(port, int) or not 1024 <= port <= 65535 or port == record['port']:
        raise RuntimeError('压测Mongo端口与登记身份不符')
    return [
        record['binary_path'], '--bind_ip', '127.0.0.1', '--port', str(port),
        '--replSet', 'minimirLoad_' + run_id, '--dbpath', str(directory / 'data'),
        '--logpath', str(directory / 'mongod.log'), '--oplogSize', str(OPLOG_MIB),
        '--wiredTigerCacheSizeGB', str(WIREDTIGER_CACHE_GIB),
    ]


def process_owned(record, directory, run_id, state):
    if not state or state.get('run_id') != run_id or state.get('installation_id') != record['installation_id']:
        return False
    if not windows_owned(state, record['binary_path']):
        return False
    expected = arguments(record, directory, run_id, state['port'])
    return state.get('argv') == expected and command_line(state['pid']) == subprocess.list2cmdline(expected)


def endpoint_uri(run_id, port):
    return f'mongodb://127.0.0.1:{port}/?replicaSet=minimirLoad_{run_id}&directConnection=true'


def status(record, directory, run_id, *, storage=DEFAULT_STORAGE):
    require_run_directory(directory, run_id, storage)
    state = read(directory / 'runtime.json')
    valid = process_owned(record, directory, run_id, state)
    bound = valid and listener(state['pid'], state['port'])
    ready = bound and state.get('status') == 'ready'
    return dict(
        running=valid, endpoint_verified=ready, listener_verified=bound, ownership_verified=valid,
        run_id=run_id, pid=state['pid'] if valid else None, port=state['port'] if state else None,
        uri=endpoint_uri(run_id, state['port']) if ready else None, replica_set='minimirLoad_' + run_id,
        database='minimir_dev_game_test_load_' + run_id, directory=str(directory.parent), storage=storage,
        log=str(directory / 'mongod.log'), version=record['version'], oplog_mib=OPLOG_MIB,
        wiredtiger_cache_gib=WIREDTIGER_CACHE_GIB, data_removed=bool(state and state.get('data_removed')),
    )


def stop_process(record, directory, run_id, state):
    if not process_owned(record, directory, run_id, state):
        raise RuntimeError('启动PID及命令未通过核验，未发送停止信号')
    windows_stop(state, record['binary_path'])
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        if not windows_owned(state, record['binary_path']):
            save(directory / 'runtime.json', dict(state, status='stopped'))
            return
        time.sleep(0.1)
    raise RuntimeError('压测Mongo未完成优雅停止，保留日志和数据')


def start(record, directory, run_id, *, storage=DEFAULT_STORAGE):
    require_run_directory(directory, run_id, storage)
    directory.parent.mkdir(parents=True, exist_ok=True)
    directory.mkdir()
    (directory / 'data').mkdir()
    require_run_directory(directory, run_id, storage)
    port = free_port()
    argv = arguments(record, directory, run_id, port)
    with (directory / 'console.log').open('xb') as log:
        child = subprocess.Popen(argv, cwd=directory, stdin=subprocess.DEVNULL, stdout=log, stderr=log,
                                 **background_options())
    identity = windows_identity(child.pid)
    if not identity:
        raise RuntimeError(f'压测Mongo身份不可读：{directory / "console.log"}')
    state = dict(identity, run_id=run_id, installation_id=record['installation_id'],
                 argv=argv, port=port, status='starting')
    save(directory / 'runtime.json', state)
    try:
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            if child.poll() is not None:
                raise RuntimeError(f'压测Mongo启动失败：{directory / "console.log"}')
            try:
                with socket.create_connection(('127.0.0.1', port), timeout=0.5):
                    break
            except OSError:
                time.sleep(0.1)
        else:
            raise RuntimeError('压测Mongo启动监听超时')
        if not process_owned(record, directory, run_id, state) or not listener(child.pid, port):
            raise RuntimeError('压测Mongo端点未通过归属核验')
        result = subprocess.run(
            [sys.executable, '-I', '-c', DRIVER_PROGRAM, endpoint_uri(run_id, port),
             'minimirLoad_' + run_id, str(port), 'minimir_hosting_env_' + run_id],
            capture_output=True, encoding='utf-8', timeout=55,
        )
        if result.returncode:
            raise RuntimeError('压测副本集事务探测失败：' + result.stderr[-2000:])
        smoke = json.loads(result.stdout)
        if smoke.get('server_version') != record['version'] or not smoke.get('transaction_commit_verified'):
            raise RuntimeError('压测Mongo版本或事务探测不符')
        save(directory / 'transaction-smoke.json', smoke)
        save(directory / 'runtime.json', dict(state, status='ready'))
        return status(record, directory, run_id, storage=storage)
    except BaseException:
        if child.poll() is None:
            stop_process(record, directory, run_id, state)
        raise


def stop(record, directory, run_id, *, storage=DEFAULT_STORAGE):
    require_run_directory(directory, run_id, storage)
    state = read(directory / 'runtime.json')
    if not state:
        return status(record, directory, run_id, storage=storage)
    expected = arguments(record, directory, run_id, state.get('port'))
    if state.get('run_id') != run_id or state.get('installation_id') != record['installation_id'] or state.get('argv') != expected:
        raise RuntimeError('压测Mongo运行标记不符，保留现场')
    if process_owned(record, directory, run_id, state):
        stop_process(record, directory, run_id, state)
        state = read(directory / 'runtime.json')
    elif windows_identity(state['pid']) and state.get('status') != 'stopped':
        raise RuntimeError('原PID已失去归属或已复用，保留现场')
    if state.get('status') == 'stopped' and not state.get('data_removed'):
        data = run_directory(run_id, storage) / 'data'
        if data.exists():
            for path in data.rglob('*'):
                if is_reparse_point(path) or not path.resolve().is_relative_to(data):
                    raise RuntimeError('压测Mongo数据目录含有重解析点或越界，保留现场')
            shutil.rmtree(data)
        save(directory / 'runtime.json', dict(state, data_removed=True))
    return status(record, directory, run_id, storage=storage)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['start', 'status', 'stop'])
    parser.add_argument('run_id')
    add_storage_argument(parser)
    args = parser.parse_args()
    try:
        if os.name != 'nt' or os.environ.get('MINIMIR_TOOLCHAIN_ACTIVE') != '1':
            raise RuntimeError('请通过 ./tools/dev.ps1 在本项目 Windows 锁定工具环境下运行')
        directory = run_directory(args.run_id, args.mongo_storage)
        record, _ = installation()
        result = {'start': start, 'status': status, 'stop': stop}[args.action](
            record, directory, args.run_id, storage=args.mongo_storage)
        print(json.dumps(result, ensure_ascii=False))
        return 0
    except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as error:
        print(json.dumps({'error': str(error)}, ensure_ascii=False), file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
