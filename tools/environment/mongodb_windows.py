"""只管理用户授权的Windows专属Mongo8.0.34新副本集；不注册服务或接入已有库。"""
import argparse
import contextlib
import hashlib
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools'))
from process_control import background_options, windows_identity, windows_owned, windows_stop
from mongodb_smoke import DRIVER_PROGRAM

KEY = hashlib.sha256(str(ROOT).encode()).hexdigest()[:12]


def sha(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def settings():
    profile = json.loads((ROOT / 'conf/toolchain/windows-amd64.lock.json').read_text(encoding='utf-8'))
    prefix = Path(os.path.expandvars(profile['prefix']))
    spec = profile['native_database']
    return profile, prefix / profile['tools']['mongod']['executable'], Path(os.path.expandvars(spec['directory'])) / KEY, spec


def save(path, value):
    if path.is_symlink():
        raise RuntimeError('拒绝控制文件符号链接')
    temporary = path.with_suffix('.tmp')
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    temporary.replace(path)


def read(path):
    if path.is_symlink():
        raise RuntimeError('拒绝控制文件符号链接')
    return json.loads(path.read_text(encoding='utf-8')) if path.is_file() else None


def directories(base):
    store = base.parent
    for path in [store, base, base / 'data', base / 'runs']:
        if path.is_symlink() or path.is_junction() or not path.resolve().is_relative_to(store.resolve()):
            raise RuntimeError('数据库目录越界或存在重解析点')


@contextlib.contextmanager
def control_lock():
    import msvcrt
    path = ROOT / '.work/environment/windows-native-control.lock'
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.is_symlink():
        raise RuntimeError('控制锁不能是符号链接')
    with path.open('a+b') as stream:
        if stream.tell() == 0:
            stream.write(b'0')
            stream.flush()
        stream.seek(0)
        try:
            msvcrt.locking(stream.fileno(), msvcrt.LK_NBLCK, 1)
        except OSError as error:
            raise RuntimeError('本项目已有原生数据库控制操作正在执行') from error
        try:
            yield
        finally:
            stream.seek(0)
            msvcrt.locking(stream.fileno(), msvcrt.LK_UNLCK, 1)


def installation():
    profile, binary, base, spec = settings()
    directories(base)
    record = read(base / 'installation.json')
    if not record or record.get('project_root') != str(ROOT) or record.get('project_key') != KEY:
        raise RuntimeError('缺少本项目原生数据库安装标记；请先执行mongodb_dev.py init')
    expected = dict(binary_path=str(binary), data_path=str(base / 'data'), version='8.0.34',
                    port=spec['port'], replica_set=spec['replica_set'], purpose='minimir_owned_windows_development')
    if any(record.get(key) != value for key, value in expected.items()):
        raise RuntimeError('数据库安装、数据目录或端口与当前平台锁不符')
    if binary.is_symlink() or not binary.is_file() or sha(binary) != record['binary_sha256']:
        raise RuntimeError('原生Mongo二进制与登记摘要不符')
    return record, base


def initialize():
    profile, binary, base, spec = settings()
    directories(base)
    if (base / 'installation.json').exists():
        record, _ = installation()
        return dict(action='already_initialized', installation_id=record['installation_id'], version=record['version'])
    if base.exists() and any(base.iterdir()):
        raise RuntimeError('目标目录已有未登记内容，拒绝接管或读取')
    if not binary.is_file():
        raise RuntimeError('请先使用bootstrap_windows.ps1 -Tools mongod安装锁定Mongo')
    archive = Path(os.path.expandvars(profile['prefix'])) / 'downloads' / profile['tools']['mongod']['url'].rsplit('/', 1)[1]
    if sha(archive) != profile['tools']['mongod']['sha256']:
        raise RuntimeError('Mongo官方归档摘要不符')
    # 将实际二进制与已验证官方归档逐字节核对，不能仅凭版本文本接管未知程序。
    import zipfile
    relative = '/'.join(profile['tools']['mongod']['executable'].split('/')[2:])
    with zipfile.ZipFile(archive) as source, source.open(relative) as stream:
        official_binary_sha = hashlib.file_digest(stream, 'sha256').hexdigest()
    if sha(binary) != official_binary_sha:
        raise RuntimeError('Mongo二进制与官方归档内容不符')
    base.mkdir(parents=True, exist_ok=True)
    (base / 'data').mkdir()
    (base / 'runs').mkdir()
    record = dict(project_root=str(ROOT), project_key=KEY, purpose='minimir_owned_windows_development',
                  installation_id=uuid.uuid4().hex, binary_path=str(binary), binary_sha256=official_binary_sha,
                  data_path=str(base / 'data'), version='8.0.34', port=spec['port'], replica_set=spec['replica_set'],
                  archive_sha256=profile['tools']['mongod']['sha256'], initialized_new_empty_directory=True)
    save(base / 'installation.json', record)
    return dict(action='initialized', **record, production_ready=False)


def command_line(pid):
    command = '[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false);' + f'(Get-CimInstance Win32_Process -Filter "ProcessId={int(pid)}").CommandLine'
    result = subprocess.run(['powershell.exe', '-NoProfile', '-NonInteractive', '-Command', command], capture_output=True, encoding='utf-8', errors='replace', timeout=15)
    return result.stdout.strip() if result.returncode == 0 else None


def arguments(record, base, run_id):
    if not isinstance(run_id, str) or len(run_id) != 32 or any(c not in '0123456789abcdef' for c in run_id):
        raise RuntimeError('数据库运行代次不合法')
    run = base / 'runs' / run_id
    if run.is_symlink() or run.is_junction():
        raise RuntimeError('运行目录不能是重解析点')
    return [record['binary_path'], '--bind_ip', '127.0.0.1', '--port', str(record['port']),
            '--replSet', record['replica_set'], '--dbpath', record['data_path'],
            '--logpath', str(run / 'mongod.log'), '--oplogSize', '128', '--wiredTigerCacheSizeGB', '0.25']


def owned(record, base, state):
    if not state or state.get('installation_id') != record['installation_id'] or state.get('project_root') != str(ROOT):
        return False
    if not windows_owned(state, record['binary_path']):
        return False
    expected = arguments(record, base, state.get('run_id'))
    return state.get('argv') == expected and command_line(state['pid']) == subprocess.list2cmdline(expected)


def listener(pid, port):
    command = f'Get-NetTCPConnection -State Listen -LocalPort {int(port)} -ErrorAction SilentlyContinue | Select-Object LocalAddress,OwningProcess | ConvertTo-Json -Compress'
    result = subprocess.run(['powershell.exe', '-NoProfile', '-NonInteractive', '-Command', command], capture_output=True, encoding='utf-8', errors='replace', timeout=15)
    if not result.stdout.strip():
        return False
    rows = json.loads(result.stdout)
    rows = rows if isinstance(rows, list) else [rows]
    return bool(rows) and all(row['OwningProcess'] == pid and row['LocalAddress'] == '127.0.0.1' for row in rows)


def status():
    record, base = installation()
    state = read(base / 'runtime.json')
    valid = owned(record, base, state)
    bound = listener(state['pid'], record['port']) if valid else False
    uri = f'mongodb://127.0.0.1:{record["port"]}/?replicaSet={record["replica_set"]}&directConnection=true'
    return dict(backend='windows_native', running=valid and bound, listener_verified=bound, ownership_verified=valid,
                pid=state['pid'] if valid else None, uri=uri if valid and bound else None,
                version=record['version'], installation_id=record['installation_id'], project_key=KEY,
                data_path=record['data_path'], port=record['port'], replica_set=record['replica_set'],
                endpoint_verified=valid and bound, production_ready=False)


def stop():
    record, base = installation()
    state = read(base / 'runtime.json')
    if not owned(record, base, state):
        return dict(action='nothing_stopped', **status())
    windows_stop(state, record['binary_path'])
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        if not windows_owned(state, record['binary_path']):
            save(base / 'runtime.json', dict(state, status='stopped'))
            return dict(action='stopped', graceful_shutdown=True, **status())
        time.sleep(0.1)
    raise RuntimeError('原生Mongo未完成正常停止，未强杀；请查本次mongod.log')


def start():
    record, base = installation()
    current = status()
    if current['ownership_verified'] and not current['listener_verified']:
        raise RuntimeError('本任务进程存在但回环监听未核验，拒绝重开')
    if not current['running']:
        previous = read(base / 'runtime.json')
        if previous and previous.get('status') != 'stopped' and windows_identity(previous.get('pid', 0)):
            raise RuntimeError('旧PID身份不符或已复用，不接管该进程')
        with socket.socket() as probe:
            probe.setsockopt(socket.SOL_SOCKET, socket.SO_EXCLUSIVEADDRUSE, 1)
            try:
                probe.bind(('127.0.0.1', record['port']))
            except OSError as error:
                raise RuntimeError('目标端口已占用，不接入或停止占用者') from error
        run_id = uuid.uuid4().hex
        run = base / 'runs' / run_id
        run.mkdir()
        argv = arguments(record, base, run_id)
        with (run / 'console.log').open('ab') as log:
            child = subprocess.Popen(argv, cwd=base, stdin=subprocess.DEVNULL, stdout=log, stderr=log, **background_options())
        identity = windows_identity(child.pid)
        if not identity:
            raise RuntimeError('Mongo启动后进程身份不可读；保留日志未接入')
        state = dict(identity, project_root=str(ROOT), installation_id=record['installation_id'], run_id=run_id, argv=argv, status='starting')
        save(base / 'runtime.json', state)
        for _ in range(100):
            if child.poll() is not None:
                raise RuntimeError(f'Mongo启动失败：{run / "console.log"}')
            if listener(child.pid, record['port']):
                break
            time.sleep(0.1)
        else:
            raise RuntimeError('Mongo回环监听尚未通过，不初始化副本集')
        if not owned(record, base, state):
            raise RuntimeError('Mongo进程命令行与创建身份不符，不连接')
        save(base / 'runtime.json', dict(state, status='running'))
    current = status()
    if not current['running']:
        raise RuntimeError('新副本集端点所有权未通过，不连接')
    result = subprocess.run([sys.executable, '-I', '-c', DRIVER_PROGRAM, current['uri'], record['replica_set'], str(record['port']), 'minimir_dev_environment_' + KEY], capture_output=True, encoding='utf-8', timeout=60)
    if result.returncode:
        stop()
        raise RuntimeError('副本集或事务探测失败：' + result.stderr[-2500:])
    smoke = json.loads(result.stdout)
    save(base / 'transaction-smoke.json', smoke)
    return dict(current, transaction_smoke=smoke)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['init', 'start', 'stop', 'status'])
    action = parser.parse_args().action
    try:
        with control_lock():
            result = {'init': initialize, 'start': start, 'stop': stop, 'status': status}[action]()
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0
    except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as error:
        print(json.dumps(dict(error=str(error), production_ready=False), ensure_ascii=False), file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
