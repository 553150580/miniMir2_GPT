"""P2浏览器限定验收：原fixture、新随机库、自有进程与Creator候选；结束核身份清理。"""
from __future__ import annotations
import argparse
import datetime as dt
import importlib.util
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import threading
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT / 'tools/server/verify_startup_recovery.py'
spec = importlib.util.spec_from_file_location('p2_runtime_base', BASE)
base = importlib.util.module_from_spec(spec)
spec.loader.exec_module(base)
WORK = ROOT / '.work/p2-fixes-20261030/browser'
PREFIX = 'minimir_dev_game_test_p2ui_'
PURPOSE = 'isolated_p2_fixes_browser'
PORTS = {'account': 18110, 'game': 18111, 'gm': 18112}
sha, load, save = base.sha, base.load, base.save
PHASE = 'starting'


def tree(directory):
    directory = Path(directory)
    return {p.relative_to(directory).as_posix(): sha(p) for p in sorted(directory.rglob('*')) if p.is_file()}


def free_ports():
    held = []
    try:
        for port in [18081, *PORTS.values()]:
            listener = socket.socket()
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_EXCLUSIVEADDRUSE, 1)
            listener.bind(('127.0.0.1', port))
            held.append(listener)
    finally:
        for listener in held:
            listener.close()


def snapshot():
    paths = list((ROOT / 'server').rglob('*.go')) + list((ROOT / 'shared/proto').glob('*.proto'))
    paths += [ROOT / p for p in ['server/go.mod', 'server/go.sum', 'config/development/first-loop.json',
                               'tools/client/serve.mjs', 'tools/client/verify_p2_fixes_browser.cjs',
                               'tools/process_control.py', 'tools/environment/mongodb_windows.py',
                               'tools/dev', 'tools/toolchain.py', 'conf/toolchain/windows-amd64.lock.json']]
    paths += [Path(__file__), BASE]
    return {p.relative_to(ROOT).as_posix(): sha(p) for p in sorted(set(paths))}


def candidate(report_path):
    if not report_path.resolve().is_relative_to((ROOT / '.work/p2-fixes-20261030/creator').resolve()):
        raise RuntimeError('只接受本阶段Creator候选报告')
    report = load(report_path)
    if not report.get('success') or report.get('creator') != '3.8.8' or report.get('exit_code') != 36 or not report.get('source_stable_during_build'):
        raise RuntimeError('Creator候选未成功或构建输入漂移')
    directory = Path(report['build_root']).resolve()
    if not directory.is_relative_to(report_path.parent.resolve()) or not (directory / 'index.html').is_file():
        raise RuntimeError('候选产物不属于本次Creator目录')
    frozen = {**report['asset_hashes'], **report['script_hashes']}
    if not any(p.endswith('/PendingRoleCreation.ts') for p in frozen):
        raise RuntimeError('候选构建缺少建角修复')
    for name, digest in frozen.items():
        if sha(ROOT / name) != digest:
            raise RuntimeError('候选源已变化：' + name)
    return directory


def run(args):
    global PHASE
    PHASE = 'waiting_creator_report'
    creator_path = (ROOT / args.creator_report).resolve()
    deadline = time.monotonic() + args.wait_report_seconds
    while not creator_path.exists():
        if time.monotonic() >= deadline:
            raise RuntimeError('等待Creator候选报告超时，尚未启动隔离服务')
        time.sleep(1)
    PHASE = 'preflight'
    prepared = base.preflight()
    free_ports()
    build_root = candidate(creator_path)
    attempt = uuid.uuid4().hex
    directory = WORK / ('run-' + dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%SZ-') + attempt[:8])
    directory.mkdir(parents=True)
    database = PREFIX + attempt
    PHASE = 'snapshot_candidate_and_served_main'
    report = dict(passed=False, attempt=attempt, purpose=PURPOSE, project_root=str(ROOT), database=database,
                  ports=PORTS, static_port=18081, creator_report=str(creator_path), creator_report_sha256=sha(creator_path),
                  build_root=str(build_root), source_before=snapshot(), main_before=prepared['main'], mongo_before=prepared['mongo'],
                  candidate_before=tree(build_root), served_main_before=tree(ROOT / 'client/build/web-mobile'),
                  credentials_written=False, qualification_seeded=False, production_ready=False, physical_mobile_verified=False)
    save(directory / 'started.json', report)
    handles, client, marker_created, config_path, config_sha = [], None, False, None, None
    marker = dict(_id='minimir_development', application='minimir_first_loop', schema_version=1,
                  attempt=attempt, purpose=PURPOSE, project_root=str(ROOT), database=database)
    try:
        from pymongo import MongoClient
        from pymongo.write_concern import WriteConcern
        client = MongoClient(prepared['mongo']['uri'], serverSelectionTimeoutMS=5000)
        if database != PREFIX + attempt or len(database.encode()) > 63 or database in client.list_database_names():
            raise RuntimeError('新随机库名称不符或已存在，不认领')
        db = client.get_database(database, write_concern=WriteConcern('majority'))
        db.application_meta.insert_one(marker)
        marker_created = True
        config = load(ROOT / 'conf/dev/server.json')
        config.update(database=database, node_id='p2ui-' + attempt[:12], run_id=int(attempt[:12], 16) + 1)
        config.update({mode + '_address': '127.0.0.1:' + str(port) for mode, port in PORTS.items()})
        config_path = ROOT / 'conf/dev' / ('p2ui-' + attempt + '.local.json')
        with config_path.open('x', encoding='utf-8', newline='\n') as output:
            output.write(json.dumps(config, ensure_ascii=False, indent=2) + '\n')
        config_sha = sha(config_path)
        save(directory / 'isolated-config.json', config)
        binary = directory / 'nbserver.exe'
        PHASE = 'build_isolated_race_binary'
        command = [sys.executable, str(ROOT / 'tools/dev'), 'go', 'build', '-race', '-o', str(binary), './cmd/nbserver']
        result = subprocess.run(command, cwd=ROOT, capture_output=True, timeout=180)
        (directory / 'build.stdout.log').write_bytes(result.stdout)
        (directory / 'build.stderr.log').write_bytes(result.stderr)
        if result.returncode:
            raise RuntimeError('本阶段独立race节点构建失败')
        report['binary_sha256'] = sha(binary)
        free_ports()
        PHASE = 'start_isolated_nodes'
        for mode in PORTS:
            base.launch([str(binary), '-config', str(config_path), '-mode', mode], binary, directory / (mode + '.log'), handles)
        report['health'] = base.health_ready(config, handles)
        for mode, port in PORTS.items():
            base.listener_identity(port, binary, [str(binary), '-config', str(config_path), '-mode', mode])
            health = report['health'][mode]
            if health.get('fixture_hash') != sha(ROOT / 'config/development/first-loop.json'):
                raise RuntimeError('隔离节点不是原fixture')
        lease = db.node_leases.find_one({'_id': config['node_id']})
        if not lease or lease.get('owner_instance') != report['health']['game']['instance']:
            raise RuntimeError('隔离game未使用本轮新库')
        node = prepared['node']
        static = base.launch([node, str(ROOT / 'tools/client/serve.mjs'), '--build-root', str(build_root), '--port', '18081',
                              '--account-port', '18110', '--game-port', '18111', '--instance-id', attempt], node, directory / 'static.log', handles)
        deadline = time.monotonic() + 15
        while True:
            try:
                h = base.http_json('127.0.0.1:18081')
                if h.get('run_id') == attempt and h.get('pid') == static['meta']['pid'] and Path(h['build_root']).resolve() == build_root:
                    break
            except (OSError, ValueError):
                pass
            if time.monotonic() >= deadline:
                raise RuntimeError('隔离静态服务未就绪')
            time.sleep(.1)
        base.listener_identity(18081, node, static['argv'])
        report['processes'] = [h['meta'] for h in handles]
        save(directory / 'runtime.json', {**report, 'static': h})
        browser_directory = directory / 'browser'
        PHASE = 'browser_touch_qa'
        command = [node, str(ROOT / 'tools/client/verify_p2_fixes_browser.cjs'), '--url', 'http://127.0.0.1:18081/',
                   '--gm-port', '18112', '--creator-report', str(creator_path), '--report-dir', str(browser_directory)]
        result = subprocess.run(command, cwd=ROOT, capture_output=True, timeout=540)
        (directory / 'browser.stdout.log').write_bytes(result.stdout)
        (directory / 'browser.stderr.log').write_bytes(result.stderr)
        report['browser_exit_code'] = result.returncode
        report['browser_report'] = str(browser_directory / 'report.json')
        browser_report = load(browser_directory / 'report.json')
        if result.returncode or not browser_report.get('passed'):
            raise RuntimeError('浏览器限定验收未通过，见本attempt原报告')
        if db.roles.count_documents({}) != 1 or db.operations.count_documents({'kind': 'create_role'}) != 1:
            raise RuntimeError('真实新库角色/建角操作数量不是唯一一笔')
        report['database_observation'] = {'roles': 1, 'create_operations': 1, 'qualification_documents': db.qa_warehouse_qualifications.count_documents({})}
        if report['database_observation']['qualification_documents']:
            raise RuntimeError('不应存在隔离资格播种')
        report['passed'] = True
    except Exception as error:
        report['error'] = str(error)
    finally:
        PHASE = 'cleanup_owned_processes_and_database'
        cleanup_errors = []
        for handle in reversed(handles):
            try:
                base.stop_owned(handle)
            except Exception as error:
                cleanup_errors.append(str(error))
        report['own_processes_stopped'] = not cleanup_errors
        if client is not None:
            try:
                if marker_created and not cleanup_errors:
                    if client[database].application_meta.find_one({'_id': marker['_id']}) != marker:
                        raise RuntimeError('随机库marker变化，不删除')
                    client.drop_database(database)
                    report['new_database_dropped'] = database not in client.list_database_names()
            except Exception as error:
                cleanup_errors.append(str(error))
            client.close()
        if config_path is not None and config_sha is not None and not cleanup_errors:
            try:
                if sha(config_path) != config_sha:
                    raise RuntimeError('临时配置变化，不删除')
                config_path.unlink()
                report['temporary_config_removed'] = True
            except Exception as error:
                cleanup_errors.append(str(error))
        try:
            PHASE = 'verify_final_source_and_artifact_hashes'
            report['source_stable'] = snapshot() == report['source_before']
            report['main_preserved'] = base.main_identity() == prepared['main']
            report['mongo_preserved'] = base.mongo_identity() == prepared['mongo']
            report['candidate_preserved'] = tree(build_root) == report['candidate_before'] and sha(creator_path) == report['creator_report_sha256']
            report['served_main_preserved'] = tree(ROOT / 'client/build/web-mobile') == report['served_main_before']
        except Exception as error:
            cleanup_errors.append(str(error))
        report['cleanup_errors'] = cleanup_errors
        report['passed'] = report['passed'] and not cleanup_errors and all(report.get(key) is True for key in
            ['own_processes_stopped', 'new_database_dropped', 'temporary_config_removed', 'source_stable', 'main_preserved', 'mongo_preserved', 'candidate_preserved', 'served_main_preserved'])
        save(directory / 'report.json', report)
    print(json.dumps({'passed': report['passed'], 'report': str(directory / 'report.json'), 'browser_report': report.get('browser_report')}, ensure_ascii=False))
    return 0 if report['passed'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--creator-report', default='.work/p2-fixes-20261030/creator/full-2/report.json')
    parser.add_argument('--wait-report-seconds', type=int, default=1200)
    args = parser.parse_args()
    stopped = threading.Event()
    def heartbeat():
        while not stopped.wait(20):
            print(json.dumps({'p2_gui_heartbeat': PHASE, 'at': dt.datetime.now(dt.timezone.utc).isoformat()}), flush=True)
    threading.Thread(target=heartbeat, daemon=True).start()
    print(json.dumps({'p2_gui_heartbeat': PHASE, 'creator_report': args.creator_report}), flush=True)
    try:
        return run(args)
    except Exception as error:
        print(json.dumps({'passed': False, 'run_started': False, 'error': str(error)}, ensure_ascii=False))
        return 1
    finally:
        stopped.set()


if __name__ == '__main__':
    raise SystemExit(main())
