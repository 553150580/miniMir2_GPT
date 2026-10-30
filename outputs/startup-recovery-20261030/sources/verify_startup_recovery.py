"""隔离真实进程的启动与有限恢复验收；只管理本轮新库、自有进程和临时配置。"""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import queue
import socket
import subprocess
import sys
import threading
import time
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[2]
sys.path[:0] = [str(ROOT / 'tools'), str(ROOT / 'tools/environment')]
from process_control import background_options, windows_identity, windows_owned, windows_stop
from mongodb_windows import command_line

WORK = ROOT / '.work/startup-recovery-20261030'
PURPOSE = 'isolated_startup_recovery'
DB_PREFIX = 'minimir_dev_game_test_rec_'


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def load(path):
    return json.loads(Path(path).read_text(encoding='utf-8'))


def save(path, value):
    Path(path).write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def checked_command(arguments, timeout=60):
    result = subprocess.run(arguments, cwd=ROOT, capture_output=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError('受控检查失败：' + ' '.join(map(str, arguments[:3])))
    return result


def mongo_identity():
    result = checked_command([sys.executable, str(ROOT / 'tools/environment/mongodb_dev.py'), 'status'])
    endpoint = json.loads(result.stdout)
    cfg = load(ROOT / 'conf/dev/server.json')
    expected = cfg['mongo_uri_by_platform']['windows']
    if endpoint.get('uri') != expected or any(endpoint.get(key) is not True for key in
                                             ['running', 'ownership_verified', 'listener_verified', 'endpoint_verified']):
        raise RuntimeError('专属Mongo端点及进程所有权未核验，不连接')
    return endpoint


def http_json(address):
    with urllib.request.urlopen('http://' + address + '/healthz', timeout=2) as response:
        return json.load(response)


def main_identity():
    result = {}
    for label, record, binary in [
        ('server', ROOT / 'server/.dev/runtime.json', ROOT / 'server/bin/nbserver.exe'),
        ('client', ROOT / '.work/client-dev/state.json', None),
    ]:
        if not record.exists():
            result[label] = {'state_exists': False}
            continue
        meta = load(record)
        executable = binary or Path(meta['executable'])
        if not windows_owned(meta, executable):
            if windows_identity(meta['pid']) is not None:
                raise RuntimeError('已有主服务PID被复用或身份变化，不执行隔离验收')
            ports = [18100, 18101, 18102] if label == 'server' else [18080]
            for port in ports:
                with socket.socket() as probe:
                    probe.settimeout(.2)
                    if probe.connect_ex(('127.0.0.1', port)) == 0:
                        raise RuntimeError('主服务状态无对应自有进程但端口有未知监听')
            result[label] = {'state_sha256': sha(record), 'running': False, 'ports_closed': True}
            continue
        result[label] = {'state_sha256': sha(record), 'pid': meta['pid'], 'start_ticks': meta['start_ticks']}
        if label == 'server':
            result[label]['health'] = {mode: http_json(f'127.0.0.1:{port}') for mode, port in
                                       [('account', 18100), ('game', 18101), ('gm', 18102)]}
    return result


def input_snapshot():
    paths = list((ROOT / 'server').rglob('*.go'))
    paths += [ROOT / 'server/go.mod', ROOT / 'server/go.sum', ROOT / 'config/development/first-loop.json',
              ROOT / 'client/assets/scripts/generated/protobuf/client_pb.js', Path(__file__),
              ROOT / 'tools/server/recovery_probe.cjs', ROOT / 'tools/process_control.py',
              ROOT / 'tools/dev', ROOT / 'tools/dev.ps1', ROOT / 'tools/toolchain.py', ROOT / 'conf/dev/server.json',
              ROOT / 'shared/proto/protocol_manifest.json', ROOT / 'tools/environment/mongodb_dev.py',
              ROOT / 'tools/environment/mongodb_windows.py', ROOT / 'tools/environment/mongodb_smoke.py',
              ROOT / 'client/package.json', ROOT / 'client/package-lock.json']
    paths += list((ROOT / 'shared/proto').glob('*.proto'))
    paths += list((ROOT / 'conf/toolchain').glob('*.lock.json'))
    for package in ['long', 'protobufjs']:
        paths += [p for p in (ROOT / 'client/node_modules' / package).rglob('*')
                  if p.is_file() and (p.suffix == '.js' or p.name == 'package.json')]
    return {p.relative_to(ROOT).as_posix(): sha(p) for p in sorted(set(paths))}


def preflight():
    if os.name != 'nt' or os.environ.get('MINIMIR_TOOLCHAIN_ACTIVE') != '1':
        raise RuntimeError('本验收目前只支持Windows独立副本集，请通过 ./tools/dev.ps1 执行')
    from pymongo import MongoClient  # 使用锁定Python环境，不安装或更改系统依赖。
    from toolchain import windows_paths
    lock, prefix = windows_paths()
    node = prefix / lock['tools']['node']['executable']
    checked_command([str(node), '--check', str(ROOT / 'tools/server/recovery_probe.cjs')])
    endpoint = mongo_identity()
    main = main_identity()
    return {'mongo': endpoint, 'main': main, 'node': str(node), 'pymongo_available': MongoClient is not None,
            'runtime_ready': False, 'production_ready': False, 'run_started': False}


def process_matches(handle):
    meta, argv = handle['meta'], handle['argv']
    return windows_owned(meta, handle['binary']) and command_line(meta['pid']) == subprocess.list2cmdline(argv)


def launch(argv, binary, log, handles, interactive=False):
    handle = {'binary': str(binary), 'argv': list(map(str, argv)), 'meta': None}
    with Path(log).open('wb') as output:
        child = subprocess.Popen(handle['argv'], cwd=ROOT,
                                 stdin=subprocess.PIPE if interactive else subprocess.DEVNULL,
                                 stdout=subprocess.PIPE if interactive else output, stderr=output,
                                 **background_options())
    handle['child'] = child
    handles.append(handle)
    handle['meta'] = windows_identity(child.pid)
    if not handle['meta'] or not process_matches(handle):
        raise RuntimeError('本轮新进程身份登记失败，未继续验收')
    return handle


def stop_owned(handle, force=False):
    child = handle.get('child')
    if child is not None and child.poll() is not None:
        return
    meta = handle.get('meta')
    if meta and windows_identity(meta['pid']) is None:
        return
    if not meta or not process_matches(handle):
        raise RuntimeError('原进程身份或命令已变化，不停止未知进程')
    if force:
        if child is None or '-mode' not in handle['argv'] or handle['argv'][handle['argv'].index('-mode') + 1] != 'game':
            raise RuntimeError('故障注入只允许本轮直接创建的game进程')
        child.kill()
        handle['forced'] = True
    else:
        windows_stop(meta, handle['binary'])
    deadline = time.monotonic() + 25
    while windows_owned(meta, handle['binary']) and time.monotonic() < deadline:
        time.sleep(.1)
    if windows_owned(meta, handle['binary']):
        raise RuntimeError('本轮进程未完成停止，保留现场且不删除数据库')
    if child is not None:
        child.wait(timeout=2)


def free_ports():
    listeners = []
    try:
        for _ in range(3):
            listener = socket.socket()
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_EXCLUSIVEADDRUSE, 1)
            listener.bind(('127.0.0.1', 0))
            listeners.append(listener)
        return [listener.getsockname()[1] for listener in listeners]
    finally:
        for listener in listeners:
            listener.close()


def listener_identity(port, binary, argv):
    script = (f'Get-NetTCPConnection -State Listen -LocalPort {int(port)} -ErrorAction SilentlyContinue | '
              'Select-Object LocalAddress,OwningProcess | ConvertTo-Json -Compress')
    result = checked_command(['powershell.exe', '-NoProfile', '-NonInteractive', '-Command', script], 15)
    rows = json.loads(result.stdout)
    rows = rows if isinstance(rows, list) else [rows]
    if not rows or any(row['LocalAddress'] != '127.0.0.1' for row in rows) or len({row['OwningProcess'] for row in rows}) != 1:
        raise RuntimeError('隔离监听不是唯一回环进程')
    meta = windows_identity(rows[0]['OwningProcess'])
    handle = {'meta': meta, 'binary': str(binary), 'argv': list(map(str, argv))}
    if not meta or not process_matches(handle):
        raise RuntimeError('隔离监听映像、创建时钟或命令不符')
    return handle


def health_ready(config, handles, timeout=30):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if any(handle.get('child') is not None and handle['child'].poll() is not None for handle in handles):
            raise RuntimeError('本轮节点在就绪前退出，请查独立日志')
        try:
            result = {mode: http_json(config[mode + '_address']) for mode in ['account', 'game', 'gm']}
            if all(row.get('status') == 'ok' and row.get('mode') == mode for mode, row in result.items()):
                return result
        except (OSError, ValueError):
            pass
        time.sleep(.1)
    raise RuntimeError('隔离节点未在时限内就绪')


class Probe:
    def __init__(self, handle):
        self.handle = handle
        self.lines = queue.Queue()

        def read():
            for line in handle['child'].stdout:
                self.lines.put(line)
            self.lines.put(None)

        threading.Thread(target=read, daemon=True).start()

    def action(self, action, **fields):
        self.handle['child'].stdin.write((json.dumps({'action': action, **fields}) + '\n').encode('utf-8'))
        self.handle['child'].stdin.flush()
        try:
            line = self.lines.get(timeout=35)
        except queue.Empty as error:
            raise RuntimeError('内存协议探针动作超时：' + action) from error
        if line is None:
            raise RuntimeError('内存协议探针提前退出')
        result = json.loads(line)
        if result.get('action') != action or not result.get('passed'):
            raise RuntimeError('内存协议探针失败：' + action + '；' + str(result.get('error', '无结果')))
        return result


def run(args):
    prepared = preflight()
    attempt = uuid.uuid4().hex
    directory = WORK / (dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%SZ-') + attempt[:8])
    if not directory.resolve().is_relative_to(WORK.resolve()):
        raise RuntimeError('本轮产物目录越界')
    directory.mkdir(parents=True)
    report = {'schema_version': 1, 'attempt': attempt, 'scope': args.scope, 'passed': False,
              'production_ready': False, 'runtime_ready': False, 'full_fault_matrix_verified': False,
              'physical_mobile_verified': False, 'capacity_verified': False,
              'credentials_written': False, 'role_qualification_seeded': False,
              'lease_modified_by_probe': False, 'skill_cooldown_verified': False,
              'injured_hp_reset_verified': False,
              'before': prepared, 'checks': [], 'source_before': input_snapshot()}
    handles, client, config_path, config_sha, marker_created = [], None, None, None, False
    database = DB_PREFIX + attempt
    marker = {'_id': 'minimir_development', 'application': 'minimir_first_loop', 'schema_version': 1,
              'recovery_attempt': attempt, 'project_root': str(ROOT), 'purpose': PURPOSE, 'database': database}
    report['database'] = database
    try:
        from pymongo import MongoClient
        from pymongo.write_concern import WriteConcern
        client = MongoClient(prepared['mongo']['uri'], serverSelectionTimeoutMS=5000)
        if database in client.list_database_names() or database != DB_PREFIX + attempt or len(database.encode()) > 63:
            raise RuntimeError('新随机库身份不符或库已存在，不认领')
        db = client.get_database(database, write_concern=WriteConcern('majority'))
        db.application_meta.insert_one(marker)
        marker_created = True
        config = load(ROOT / 'conf/dev/server.json')
        ports = free_ports()
        config.update(database=database, node_id='recovery-' + attempt[:12])
        config.update({mode + '_address': '127.0.0.1:' + str(port) for mode, port in zip(['account', 'game', 'gm'], ports)})
        config_path = ROOT / 'conf/dev' / ('recovery-' + attempt + '.local.json')
        with config_path.open('x', encoding='utf-8', newline='\n') as output:
            output.write(json.dumps(config, ensure_ascii=False, indent=2) + '\n')
        config_sha = sha(config_path)
        save(directory / 'isolated-config.json', config)
        binary = directory / 'nbserver.exe'
        build = [sys.executable, str(ROOT / 'tools/dev'), 'go', 'build']
        if not args.no_race:
            build.append('-race')
        build += ['-o', str(binary), './cmd/nbserver']
        result = subprocess.run(build, cwd=ROOT, capture_output=True, timeout=180)
        (directory / 'build.stdout.log').write_bytes(result.stdout)
        (directory / 'build.stderr.log').write_bytes(result.stderr)
        if result.returncode:
            raise RuntimeError('独立节点构建失败')
        report['binary_sha256'] = sha(binary)
        report['race_build'] = not args.no_race
        argv = lambda mode: [str(binary), '-config', str(config_path), '-mode', mode]
        supervisor = launch(argv('supervisor'), binary, directory / 'supervisor.log', handles)
        health = health_ready(config, [supervisor])
        supervisor_children = []
        for mode, port in zip(['account', 'game', 'gm'], ports):
            child = listener_identity(port, binary, argv(mode))
            handles.append(child)
            supervisor_children.append(child)
            lease = db.node_leases.find_one({'_id': config['node_id'] if mode == 'game' else 'singleton:' + mode})
            if not lease or lease['owner_instance'] != health[mode]['instance']:
                raise RuntimeError('监督子进程没有使用本轮隔离数据库')
        report['checks'].append({'supervisor_custom_config': True, 'health': health,
                                 'children': [h['meta'] for h in supervisor_children]})
        stop_owned(supervisor)
        for child in supervisor_children:
            if windows_owned(child['meta'], binary):
                raise RuntimeError('监督进程优雅退出后仍有原子进程')
        report['checks'].append({'supervisor_graceful_group_stop': True})
        account = launch(argv('account'), binary, directory / 'account-1.log', handles)
        game = launch(argv('game'), binary, directory / 'game-1.log', handles)
        gm = launch(argv('gm'), binary, directory / 'gm.log', handles)
        initial_health = health_ready(config, [account, game, gm])
        probe_handle = launch([prepared['node'], str(ROOT / 'tools/server/recovery_probe.cjs')],
                              prepared['node'], directory / 'probe.stderr.log', handles, interactive=True)
        probe = Probe(probe_handle)
        report['checks'].append(probe.action('initialize', account='http://' + config['account_address'],
                                            game='ws://' + config['game_address'] + '/ws',
                                            rule_version=initial_health['game']['rule_version']))
        stop_owned(account)
        report['checks'].append(probe.action('account_down'))
        account = launch(argv('account'), binary, directory / 'account-2.log', handles)
        restarted_health = health_ready(config, [account, game, gm])
        if restarted_health['game']['instance'] != initial_health['game']['instance']:
            raise RuntimeError('account单独重启改变了game实例')
        report['checks'].append(probe.action('account_up'))
        if args.scope == 'full':
            original_claim = db.operations.find_one({'kind': 'claim_hosting'})
            if original_claim is None or db.operations.count_documents({'kind': 'claim_hosting'}) != 1:
                raise RuntimeError('没有唯一已确认的原领取回执，未注入game故障')
            original_run = db.run_control.find_one({'_id': config['run_id']})
            stop_owned(game, force=True)
            killed_at = time.monotonic()
            deadline = killed_at + 25
            while time.monotonic() < deadline:
                expired = db.run_control.find_one({'_id': config['run_id'], 'owner_instance': original_run['owner_instance'],
                                                  'run_epoch': original_run['run_epoch'],
                                                  '$expr': {'$lte': ['$lease_until', '$$NOW']}})
                if expired:
                    break
                time.sleep(.2)
            else:
                raise RuntimeError('旧run租约未自然到期；未写库抢跑')
            report['checks'].append({'game_force_stop_own_pid': True, 'old_run_epoch': original_run['run_epoch'],
                                     'lease_expired_naturally': True, 'wait_seconds': round(time.monotonic() - killed_at, 3)})
            game = launch(argv('game'), binary, directory / 'game-2.log', handles)
            health_ready(config, [account, game, gm])
            next_run = db.run_control.find_one({'_id': config['run_id']})
            if next_run['run_epoch'] <= original_run['run_epoch'] or next_run['owner_instance'] == original_run['owner_instance']:
                raise RuntimeError('game重启没有换代')
            report['checks'].append({'new_run_epoch': next_run['run_epoch'], **probe.action('game_restarted')})
            if db.operations.find_one({'_id': original_claim['_id']}) != original_claim or db.operations.count_documents({'kind': 'claim_hosting'}) != 1:
                raise RuntimeError('故障重建或重查改写原领取typed回执')
            report['checks'].append({'original_typed_claim_receipt_unchanged': True,
                                     'claim_receipt_count': 1})
        report['checks'].append(probe.action('finish'))
        probe_handle['child'].stdin.close()
        probe_handle['child'].wait(timeout=5)
        report['passed'] = True
    except Exception as error:
        report['error'] = str(error)
    finally:
        cleanup_errors = []
        for handle in reversed(handles):
            try:
                if handle.get('child') is not None and handle['child'].stdin is not None and handle['child'].poll() is None:
                    handle['child'].stdin.close()
                    handle['child'].wait(timeout=5)
                stop_owned(handle)
            except Exception as error:
                cleanup_errors.append(str(error))
        report['own_processes_stopped'] = not cleanup_errors
        report['process_exits'] = [{'pid': handle['meta']['pid'] if handle.get('meta') else None,
                                   'exit_code': handle['child'].poll(), 'forced_game': handle.get('forced', False)}
                                  for handle in handles if handle.get('child') is not None]
        report['race_diagnostics_absent'] = not any(b'WARNING: DATA RACE' in path.read_bytes()
                                                    for path in directory.glob('*.log'))
        if client is not None:
            try:
                if marker_created and not cleanup_errors:
                    if client[database].application_meta.find_one({'_id': marker['_id']}) != marker:
                        raise RuntimeError('本轮库marker改变，不删除')
                    client.drop_database(database)
                    report['new_database_dropped'] = database not in client.list_database_names()
            except Exception as error:
                cleanup_errors.append(str(error))
            client.close()
        if config_path is not None and config_sha is not None:
            try:
                if sha(config_path) != config_sha:
                    raise RuntimeError('本轮临时配置已变化，不删除')
                config_path.unlink()
                report['temporary_config_removed'] = True
            except Exception as error:
                cleanup_errors.append(str(error))
        try:
            report['after'] = {'mongo': mongo_identity(), 'main': main_identity()}
            report['main_preserved'] = report['after']['main'] == prepared['main']
            report['mongo_preserved'] = report['after']['mongo'] == prepared['mongo']
            report['source_after'] = input_snapshot()
            report['source_stable'] = report['source_after'] == report['source_before']
        except Exception as error:
            cleanup_errors.append(str(error))
        report['cleanup_errors'] = cleanup_errors
        report['passed'] = report['passed'] and not cleanup_errors and all(report.get(key) is True for key in
                          ['own_processes_stopped', 'new_database_dropped', 'temporary_config_removed',
                           'main_preserved', 'mongo_preserved', 'source_stable', 'race_diagnostics_absent'])
        save(directory / 'report.json', report)
    print(json.dumps({'passed': report['passed'], 'scope': args.scope, 'report': str(directory / 'report.json'),
                      'production_ready': False, 'full_fault_matrix_verified': False}, ensure_ascii=False))
    return 0 if report['passed'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    actions = parser.add_mutually_exclusive_group(required=True)
    actions.add_argument('--check', action='store_true', help='只读核验依赖、Mongo和已有主服务身份，不启动进程或建库')
    actions.add_argument('--run', action='store_true', help='执行一次独立新库和进程验收，结束清理自有资源')
    parser.add_argument('--scope', choices=['account', 'full'], default='full')
    parser.add_argument('--no-race', action='store_true', help='仅在明确需要时关闭独立二进制race构建')
    args = parser.parse_args()
    try:
        if args.check:
            print(json.dumps(preflight(), ensure_ascii=False))
            return 0
        return run(args)
    except Exception as error:
        print(json.dumps({'passed': False, 'error': str(error), 'run_started': False}, ensure_ascii=False))
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
