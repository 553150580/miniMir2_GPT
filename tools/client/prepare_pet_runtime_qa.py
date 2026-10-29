"""隔离宠物尸体GUI：只改.work局部cfg，受控起停专属节点并恢复main；凭据由浏览器仅存内存。"""
from __future__ import annotations
import argparse
import copy
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import socket
import subprocess
import sys
import time
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools'))
sys.path.insert(0, str(ROOT / 'tools/environment'))
from process_control import background_options, windows_identity, windows_owned, windows_stop
import mongodb_windows as database

PORTS = {'account': 18100, 'game': 18101, 'gm': 18102}
DB_PREFIX = 'minimir_dev_game_test_pet_gui_'
PURPOSE = 'isolated_pet_runtime_gui'


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def workspace_path(value):
    path = (ROOT / value).resolve()
    if path == ROOT / '.work' or not path.is_relative_to((ROOT / '.work').resolve()):
        raise ValueError('QA产物必须在本项目.work子目录')
    return path


def build_fixture(source, ttl_ms):
    if ttl_ms < 12000 or ttl_ms > 15000:
        raise ValueError('本GUI只接受12000–15000ms隔离尸体观察期限')
    value = copy.deepcopy(source)
    if value['rule_version'] != 16 or not value['pet_runtime']['enabled'] or value['pet_runtime']['bootstrap_mode'] != 'legacy_untrained_pet':
        raise ValueError('需要当前rule16开发宠物底座')
    if value['casting_supply']['enabled'] or any(row['id'] == 17 for row in value['skills']):
        raise ValueError('本GUI不允许开启护符或17')
    pet = value['pet']
    pet.update(hp=1, attack_min=0, attack_max=0)
    matches = [row for row in value['pet_runtime']['definitions'] if row['base']['definition_id'] == pet['definition_id']]
    if len(matches) != 1:
        raise ValueError('legacy必须唯一匹配typed宠物定义')
    matches[0]['base'].update(hp=1, attack_min=0, attack_max=0)
    matches[0]['lifecycle']['corpse_ttl_ms'] = ttl_ms
    value['map']['pet_spawn'] = {'x': 8, 'y': 6}
    value['map']['monster_spawns'][0] = {'x': 8, 'y': 5}
    for monster in value['monsters']:
        monster.update(attack_min=20, attack_max=20, accuracy=255, attack_ms=600000, move_ms=600000)
    return value


def verify_drop_target(name, run_id, marker):
    if not re.fullmatch(r'[0-9a-f]{32}', run_id) or name != DB_PREFIX + run_id:
        raise ValueError('拒绝删除非本轮随机QA库')
    if marker != {'_id': 'minimir_development', 'application': 'minimir_first_loop', 'schema_version': 1, 'qa_run_id': run_id, 'project_root': str(ROOT), 'purpose': PURPOSE, 'database': name}:
        raise ValueError('QA数据库所有权marker不匹配，拒绝删除')


def dev(args, log=None, timeout=60):
    result = subprocess.run([sys.executable, str(ROOT / 'tools/dev'), *args], cwd=ROOT, capture_output=True, timeout=timeout)
    if log:
        log.write_bytes(result.stdout + result.stderr)
    if result.returncode:
        raise RuntimeError('受控开发命令失败：' + ' '.join(args[:3]) + '；详见局部日志')
    return json.loads(result.stdout) if result.stdout.strip() else None


def ports_free():
    for port in PORTS.values():
        with socket.socket() as probe:
            probe.settimeout(.2)
            if probe.connect_ex(('127.0.0.1', port)) == 0:
                raise RuntimeError(f'端口{port}仍有活动监听，不接管')
        with socket.socket() as listener:
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            try:
                listener.bind(('127.0.0.1', port))
            except OSError as error:
                raise RuntimeError(f'端口{port}仍被占用，不停止或接管未知进程') from error


def assert_main_identity():
    state = ROOT / 'server/.dev/runtime.json'
    if not state.exists():
        return
    if state.is_symlink():
        raise RuntimeError('拒绝main运行记录符号链接')
    meta = json.loads(state.read_text(encoding='utf-8'))
    live = windows_identity(meta.get('pid', 0))
    binary = ROOT / 'server/bin/nbserver.exe'
    if live and (meta.get('project_root') != str(ROOT) or not windows_owned(meta, binary)):
        raise RuntimeError('main记录指向非本项目原进程，不接管')


def health(fixture_hash):
    result = {}
    for mode, port in PORTS.items():
        try:
            with urllib.request.urlopen(f'http://127.0.0.1:{port}/healthz', timeout=1) as response:
                value = json.load(response)
            if value.get('status') != 'ok' or value.get('mode') != mode or value.get('fixture_hash') != fixture_hash or value.get('rule_version') != 16:
                return None
            result[mode] = value
        except (OSError, ValueError):
            return None
    return result


def protected_inputs():
    paths = [ROOT / 'conf/dev/server.json', ROOT / 'config/development/first-loop.json', ROOT / 'client/assets/resources/development/gameplay.json']
    paths += list((ROOT / 'outputs/development-prep-20261026').glob('*.xlsx'))
    paths += list((ROOT / 'client/assets/scenes').glob('*.scene')) + list((ROOT / 'client/assets/resources/prefabs').glob('*.prefab'))
    build = json.loads((ROOT / 'docs/客户端构建检查.json').read_text(encoding='utf-8'))
    paths += [ROOT / name for name in build['script_hashes']]
    return {path.relative_to(ROOT).as_posix(): sha(path) for path in sorted(set(paths))}


def run(args):
    if os.name != 'nt':
        raise RuntimeError('当前所有权实现仅支持已登记Windows专属开发环境')
    run_id = uuid.uuid4().hex
    directory = workspace_path(args.report_dir) / ('run-' + run_id)
    directory.mkdir(parents=True, exist_ok=False)
    report = {'passed': False, 'scope': '隔离cfg真实怪击→宠物尸体GUI；非180秒参数、自然成长、17、128容量或手机真机', 'run_id': run_id,
              'database': DB_PREFIX + run_id, 'qa_root': directory.relative_to(ROOT).as_posix(), 'corpse_ttl_ms': args.ttl_ms,
              'credentials_written': False, 'corpse_seeded': False, 'skills_seeded': False, 'binding_seeded': False, 'position_seeded_in_database': False,
              'main_restored': False, 'qa_stopped': False, 'qa_database_dropped': False}
    before = protected_inputs()
    report['protected_before'] = before
    tool_files = [Path(__file__), ROOT / 'tools/client/verify_pet_runtime_browser.cjs', ROOT / 'tools/client/test_pet_runtime_qa.py', ROOT / 'tools/client/pet_runtime_browser.test.cjs']
    report['qa_tool_hashes_before'] = {file.relative_to(ROOT).as_posix(): sha(file) for file in tool_files}
    cfg = json.loads((ROOT / 'conf/dev/server.json').read_text(encoding='utf-8'))
    fixture = build_fixture(json.loads((ROOT / 'config/development/first-loop.json').read_text(encoding='utf-8')), args.ttl_ms)
    fixture_path = directory / 'config/development/first-loop.json'
    save(fixture_path, fixture)
    cfg.update(database=report['database'], fixture_file=str(fixture_path), node_id='pet-gui-' + run_id, run_id=int(run_id[:12], 16) + 1)
    # supervisor按cfg.Root/conf/dev/server.json找子节点配置，因此保留独立root的相同目录结构。
    for mode, port in PORTS.items():
        cfg[mode + '_address'] = f'127.0.0.1:{port}'
    cfg_path = directory / 'conf/dev/server.json'
    save(cfg_path, cfg)
    binary = directory / 'nbserver.exe'
    child = None
    meta = None
    client = None
    database_owned = False
    creation_attempted = False
    main_stopped = False
    cleanup_errors = []
    try:
        endpoint = database.status()
        expected_uri = cfg['mongo_uri_by_platform']['windows']
        if not endpoint.get('running') or not endpoint.get('endpoint_verified') or endpoint.get('uri') != expected_uri:
            raise RuntimeError('项目专属Mongo端点未通过所有权核验')
        report['mongo_endpoint_verified'] = True
        # 先编本轮专属binary，失败时仍不干扰main。
        dev(['go', 'build', '-o', str(binary), './cmd/nbserver'], directory / 'build.txt', timeout=120)
        assert_main_identity()
        report['main_before'] = dev(['python3', 'tools/server/dev.py', 'status'])
        stopped = dev(['python3', 'tools/server/dev.py', 'stop'])
        if stopped.get('running'):
            raise RuntimeError('main没有优雅停止，未占用端口')
        main_stopped = True
        ports_free()
        from pymongo import MongoClient
        client = MongoClient(expected_uri, serverSelectionTimeoutMS=3000)
        name = report['database']
        if name in client.list_database_names():
            raise RuntimeError('随机QA库已经存在，拒绝复用')
        # 单次新建标准开发标记与本轮所有权，避免先建旁路集合使Migrate误判非本项目旧库。
        marker = {'_id': 'minimir_development', 'application': 'minimir_first_loop', 'schema_version': 1, 'qa_run_id': run_id, 'project_root': str(ROOT), 'purpose': PURPOSE, 'database': name}
        creation_attempted = True
        client[name]['application_meta'].insert_one(marker)
        database_owned = True
        argv = [str(binary), '-config', str(cfg_path), '-mode', 'supervisor', '-run-id', run_id]
        with (directory / 'server.log').open('ab') as log:
            child = subprocess.Popen(argv, cwd=ROOT, stdin=subprocess.DEVNULL, stdout=log, stderr=subprocess.STDOUT, close_fds=True, **background_options())
        meta = windows_identity(child.pid)
        if not meta:
            raise RuntimeError('QA进程在身份登记前已退出')
        meta.update(project_root=str(ROOT), run_id=run_id, argv=argv, database=name, purpose=PURPOSE, binary_sha256=sha(binary), config_sha256=sha(cfg_path), fixture_sha256=sha(fixture_path), corpse_ttl_ms=args.ttl_ms)
        save(directory / 'runtime.json', meta)
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            if not windows_owned(meta, binary):
                raise RuntimeError('QA进程身份变化或启动退出')
            state = health(sha(fixture_path))
            if state:
                report['qa_health'] = state
                meta['health_instances'] = {mode: state[mode]['instance'] for mode in PORTS}
                meta['ports'] = PORTS
                save(directory / 'runtime.json', meta)
                break
            time.sleep(.1)
        else:
            raise RuntimeError('QA三节点20秒内未就绪，查本轮server.log')
        # 浏览器只接正常保存UI；账号、密码与票据从不经helper参数或输出落盘。
        browser_cmd = [sys.executable, str(ROOT / 'tools/dev'), 'node', 'tools/client/verify_pet_runtime_browser.cjs', '--report-dir', str(directory / 'browser'), '--qa-evidence', str(directory / 'runtime.json'), '--ttl-ms', str(args.ttl_ms)]
        result = subprocess.run(browser_cmd, cwd=ROOT, capture_output=True, timeout=150)
        (directory / 'browser-output.txt').write_bytes(result.stdout + result.stderr)
        report['browser_exit_code'] = result.returncode
        if result.returncode:
            raise RuntimeError('尸体GUI未通过，保留本轮截图/报告，不覆盖历史')
        gui = json.loads((directory / 'browser/report.json').read_text(encoding='utf-8'))
        report['browser'] = {'passed': gui['passed'], 'checks': len(gui['checks']), 'errors': len(gui['errors']), 'warnings': len(gui['warnings']), 'report': str(directory / 'browser/report.json')}
        report['passed'] = gui['passed']
    except Exception as error:
        report['error'] = str(error)
    finally:
        if child is not None:
            try:
                if child.poll() is None:
                    if meta is None or meta.get('project_root') != str(ROOT) or meta.get('run_id') != run_id or not windows_owned(meta, binary):
                        raise RuntimeError('QA原进程身份已变化，未强停')
                    windows_stop(meta, binary)
                    child.wait(timeout=20)
                ports_free()
                report['qa_stopped'] = True
            except Exception as error:
                cleanup_errors.append('停止自有QA：' + str(error))
        else:
            report['qa_stopped'] = True
        report['database_creation_attempted'] = creation_attempted
        report['database_creation_acknowledged'] = database_owned
        if creation_attempted and report['qa_stopped']:
            try:
                actual = client[report['database']]['application_meta'].find_one({'_id': 'minimir_development', 'qa_run_id': run_id})
                verify_drop_target(report['database'], run_id, actual)
                client.drop_database(report['database'])
                report['qa_database_dropped'] = report['database'] not in client.list_database_names()
            except Exception as error:
                cleanup_errors.append('清理随机专属QA库：' + str(error))
        if client is not None:
            client.close()
        if main_stopped and report['qa_stopped']:
            try:
                ports_free()
                restored = dev(['python3', 'tools/server/dev.py', 'start'], directory / 'main-restore.txt', timeout=120)
                report['main_after'] = restored
                report['main_restored'] = restored.get('running') is True and len(restored.get('health', {})) == 3 and all(item.get('status') == 'ok' for item in restored['health'].values())
            except Exception as error:
                cleanup_errors.append('恢复main：' + str(error))
        report['cleanup_errors'] = cleanup_errors
        report['protected_after'] = protected_inputs()
        report['protected_unchanged'] = report['protected_after'] == before
        report['qa_tool_hashes_after'] = {file.relative_to(ROOT).as_posix(): sha(file) for file in tool_files}
        report['qa_tools_stable'] = report['qa_tool_hashes_before'] == report['qa_tool_hashes_after']
        report['passed'] = bool(report['passed'] and report['qa_stopped'] and report['qa_database_dropped'] and report['main_restored'] and report['protected_unchanged'] and report['qa_tools_stable'] and not cleanup_errors)
        save(directory / 'report.json', report)
    print(json.dumps({'passed': report['passed'], 'report': str(directory / 'report.json'), 'browser': report.get('browser'), 'main_restored': report['main_restored'], 'qa_database_dropped': report['qa_database_dropped'], 'error': report.get('error'), 'cleanup_errors': cleanup_errors}, ensure_ascii=False))
    return 0 if report['passed'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run', action='store_true', help='显式启动本次独立QA并在finally恢复main')
    parser.add_argument('--report-dir', default='.work/pet-runtime-20261029/gui-review')
    parser.add_argument('--ttl-ms', type=int, default=15000)
    args = parser.parse_args()
    if not args.run:
        source = json.loads((ROOT / 'config/development/first-loop.json').read_text(encoding='utf-8'))
        build_fixture(source, args.ttl_ms)
        print(json.dumps({'prepared': True, 'run_started': False, 'ownership': 'project root + Windows executable/start_ticks + per-run marker database; unknown processes rejected', 'ports': PORTS, 'scope': '仅静态准备；--run才允许受控节点/专属DB/浏览器及finally恢复main'}, ensure_ascii=False))
        return 0
    if hasattr(signal, 'SIGBREAK'):
        signal.signal(signal.SIGBREAK, lambda *_: (_ for _ in ()).throw(KeyboardInterrupt()))
    return run(args)


if __name__ == '__main__':
    raise SystemExit(main())
