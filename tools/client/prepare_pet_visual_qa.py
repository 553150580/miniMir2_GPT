"""真实召唤出场隔离GUI：独立Creator包、随机新库，结束后恢复主服务。"""
from __future__ import annotations
import argparse
import copy
import hashlib
import json
import os
from pathlib import Path
import re
import socket
import subprocess
import sys
import time
import urllib.request
import uuid

import isolated_creator as creator
import prepare_pet_runtime_qa as common

ROOT = common.ROOT
sha, save, dev = common.sha, common.save, common.dev
PORTS = {'account': 18110, 'game': 18111, 'gm': 18112}
STATIC_PORT = 18082
PURPOSE = 'isolated_pet_visual_gui'
DB_PREFIX = 'minimir_dev_game_test_pet_viz_'


def build_fixture(source, catalog):
    if source['rule_version'] != 18 or source['casting_supply']['enabled'] or any(s['id'] == 17 for s in source['skills']):
        raise ValueError('需要rule18、关闭17与护符的主底座')
    if not source['pet_runtime']['enabled'] or source['pet_runtime']['bootstrap_mode'] != 'legacy_untrained_pet' or catalog['runtime_enabled'] or catalog['required_rule_version'] != 17:
        raise ValueError('来源底座或独立关闭目录不符合约定')
    if [s['id'] for s in catalog['skills']] != [17] or [b['id'] for b in catalog['books']] != [25] or len(catalog['upgrades']) != 3:
        raise ValueError('召唤目录不完整')
    value = copy.deepcopy(source)
    value['skills'] += copy.deepcopy(catalog['skills'])
    value['items'] += copy.deepcopy(catalog['books'])
    value['skill_upgrades'] += copy.deepcopy(catalog['upgrades'])
    value['casting_supply']['enabled'] = True
    value['pet_runtime']['bootstrap_mode'] = 'none'
    for item in value['items']:
        if item['id'] == 187:
            item['max_stack'] = 2  # 显式两Count隔离资格，不代表符包规格。
    for row in value['initial_skill_books']:
        if row['job_id'] == 2:
            row['item_id'] = 25
    # GUI先检查活宠正常出场与Recall，不用随机怪伤害打断一秒动画。
    for monster in value['monsters']:
        monster.update(attack_min=0, attack_max=0, attack_ms=600000, move_ms=600000)
    return value


def marker_for(name, run):
    return {'_id': 'minimir_development', 'application': 'minimir_first_loop', 'schema_version': 1,
            'qa_run_id': run, 'project_root': str(ROOT), 'purpose': PURPOSE, 'database': name}


def verify_target(name, run, marker):
    if len(name.encode('utf-8')) > 63 or not re.fullmatch(r'[0-9a-f]{32}', run) or name != DB_PREFIX + run or marker != marker_for(name, run):
        raise ValueError('只允许本轮随机新库与精确所有权marker')


def ports_free():
    for port in PORTS.values():
        with socket.socket() as probe:
            probe.settimeout(.2)
            if probe.connect_ex(('127.0.0.1', port)) == 0:
                raise RuntimeError(f'QA端口{port}仍有活动监听，不接管')
        with socket.socket() as listener:
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            try:
                listener.bind(('127.0.0.1', port))
            except OSError as error:
                raise RuntimeError(f'QA端口{port}仍被占用，不接管未知进程') from error


def protected_inputs():
    result = common.protected_inputs()
    paths = [ROOT / 'config/development/summon-candidates.json', ROOT / 'client/assets/resources/development/manifest.json']
    paths += [p for p in (ROOT / 'client/assets/scripts').rglob('*') if p.is_file()]
    paths += [p for p in (ROOT / 'client/build/web-mobile').rglob('*') if p.is_file()]
    for path in paths:
        result[path.relative_to(ROOT).as_posix()] = sha(path)
    return result


def health(fixture_hash, instances=None):
    values = {}
    for mode, port in PORTS.items():
        try:
            with urllib.request.urlopen(f'http://127.0.0.1:{port}/healthz', timeout=1) as response:
                value = json.load(response)
            if value.get('status') != 'ok' or value.get('mode') != mode or value.get('fixture_hash') != fixture_hash or value.get('rule_version') != 18 or not value.get('instance'):
                return None
            if instances and value['instance'] != instances[mode]:
                return None
            values[mode] = value
        except (OSError, ValueError):
            return None
    return values


def qualify(runtime_path, role_id):
    """只对本轮正常新建、尚无owner的角色设置等级及显式Count资格。"""
    runtime_path = common.workspace_path(runtime_path)
    if not re.fullmatch(r'[0-9a-f]{32}', role_id):
        raise ValueError('角色ID必须为正常创角回包中的16字节标识')
    meta = json.loads(runtime_path.read_text(encoding='utf-8'))
    verify_target(meta['database'], meta['run_id'], marker_for(meta['database'], meta['run_id']))
    binary = runtime_path.parent / 'nbserver.exe'
    if meta['project_root'] != str(ROOT) or meta['purpose'] != PURPOSE or not common.windows_owned(meta, binary) or sha(binary) != meta['binary_sha256']:
        raise RuntimeError('本轮节点原进程身份失效')
    config_path = Path(meta['argv'][meta['argv'].index('-config') + 1])
    if config_path.resolve() != runtime_path.parent / 'conf/dev/server.json' or sha(config_path) != meta['config_sha256']:
        raise RuntimeError('本轮配置身份失效')
    cfg = json.loads(config_path.read_text(encoding='utf-8'))
    fixture_path = Path(cfg['fixture_file'])
    if cfg['database'] != meta['database'] or fixture_path.resolve() != runtime_path.parent / 'config/development/first-loop.json' or sha(fixture_path) != meta['fixture_sha256']:
        raise RuntimeError('本轮fixture身份失效')
    if not health(meta['fixture_sha256'], meta['health_instances']):
        raise RuntimeError('本轮三节点身份失效')
    endpoint = common.database.status()
    if not endpoint.get('running') or not endpoint.get('endpoint_verified') or endpoint['uri'] != cfg['mongo_uri_by_platform']['windows']:
        raise RuntimeError('专属Mongo端点身份失效')
    fixture = json.loads(fixture_path.read_text(encoding='utf-8'))
    skill = next(s for s in fixture['skills'] if s['id'] == 17)
    from pymongo import MongoClient, ReturnDocument
    with MongoClient(endpoint['uri'], serverSelectionTimeoutMS=3000) as client:
        db = client[meta['database']]
        verify_target(meta['database'], meta['run_id'], db.application_meta.find_one({'_id': 'minimir_development'}))
        role = db.roles.find_one({'_id': role_id})
        if not role or role.get('job') != 2 or role.get('level') != 1 or role.get('revision') != 1 or role.get('economic_command_seq') != 0 or role.get('source_seq') != 0 or role.get('owner_instance') or role.get('run_epoch') or role.get('session_epoch') or role.get('learned_skills') or role.get('binding') is not None or role.get('binding_source_seq', 0) != 0 or role.get('equipped'):
            raise ValueError('必须为正常新建、从未入场/学习/操作/绑定的道士')
        if not any(i['item_id'] == 25 and i.get('source_kind') == 'initial_grant' for i in role['items']) or any(i['item_id'] == 187 for i in role['items']):
            raise ValueError('初始原书/材料前态不符')
        item = {'instance_id': uuid.uuid4().hex, 'item_id': 187, 'quantity': 2, 'bound': True, 'version': 1,
                'source_kind': 'development_fixture', 'source_id': 'two_explicit_count_units'}
        # owner入场会写同一聚合；CAS含原全部资格字段，不能覆盖并发取得owner的角色。
        query = {key: role.get(key) for key in ['_id', 'level', 'revision', 'economic_command_seq', 'source_seq', 'owner_instance', 'run_epoch', 'session_epoch', 'learned_skills', 'binding', 'binding_source_seq', 'equipped', 'items']}
        after = db.roles.find_one_and_update(query, {'$set': {'level': skill['learn_level']}, '$push': {'items': item}}, return_document=ReturnDocument.AFTER)
        if not after:
            raise RuntimeError('角色原资格CAS变化，没有写入')
        expected = copy.deepcopy(role)
        expected['level'] = skill['learn_level']
        expected['items'].append(item)
        if after != expected:
            raise RuntimeError('除隔离等级及两Count外的字段发生变化')
    result = {'passed': True, 'role_id_sha256': hashlib.sha256(role_id.encode()).hexdigest(),
              'level': skill['learn_level'], 'explicit_count': 2, 'book_from_normal_create': True,
              'skills_seeded': False, 'binding_seeded': False, 'position_seeded': False, 'body_seeded': False,
              'pack_size_or_price_inferred': False, 'only_before_owner': True}
    save(runtime_path.parent / 'qualification.json', result)
    return result


def static_port_free():
    with socket.socket() as probe:
        probe.settimeout(.2)
        if probe.connect_ex(('127.0.0.1', STATIC_PORT)) == 0:
            raise RuntimeError(f'{STATIC_PORT}已有监听，拒绝接管')
    with socket.socket() as listener:
        try:
            listener.bind(('127.0.0.1', STATIC_PORT))
        except OSError as error:
            raise RuntimeError(f'{STATIC_PORT}仍被占用，拒绝接管') from error


def launch_static(directory, project, run, handle):
    static_port_free()
    node = Path(dev(['node', '-p', 'JSON.stringify(process.execPath)']))
    argv = [str(node), str(ROOT / 'tools/client/serve.mjs'), '--instance-id', run,
            '--build-root', str(project / 'build/web-mobile'), '--port', str(STATIC_PORT), '--account-port', '18110', '--game-port', '18111']
    with (directory / 'static.log').open('wb') as log:
        child = subprocess.Popen(argv, cwd=ROOT, stdin=subprocess.DEVNULL, stdout=log, stderr=subprocess.STDOUT, **common.background_options())
    # Popen成功立即归交finally；登记或写证据失败也不能丢失已启动的子进程。
    handle.update(child=child, node=node, meta=None)
    meta = common.windows_identity(child.pid)
    if not meta or child.poll() is not None:
        raise RuntimeError('隔离静态进程未能登记身份')
    meta.update(project_root=str(ROOT), run_id=run, argv=argv)
    handle['meta'] = meta
    save(directory / 'static-runtime.json', meta)
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        if not common.windows_owned(meta, node):
            raise RuntimeError('隔离静态进程退出或身份变化')
        try:
            with urllib.request.urlopen(f'http://127.0.0.1:{STATIC_PORT}/healthz', timeout=1) as response:
                value = json.load(response)
            if value.get('pid') == child.pid and value.get('run_id') == run and Path(value['build_root']).resolve() == project / 'build/web-mobile' and value.get('account_port') == PORTS['account'] and value.get('game_port') == PORTS['game']:
                return child, meta, node
        except OSError:
            pass
        time.sleep(.1)
    raise RuntimeError('隔离静态包未就绪')


def stop_static(handle):
    if not handle or handle['child'].poll() is not None:
        return True
    child, meta, node = (handle[key] for key in ['child', 'meta', 'node'])
    if meta is None or not common.windows_owned(meta, node):
        raise RuntimeError('静态原进程身份未能证明，保留现场，不声称已停')
    common.windows_stop(meta, node)
    child.wait(timeout=20)
    if child.poll() is None:
        raise RuntimeError('静态原进程尚未停止')
    return True


def run(args):
    if os.name != 'nt':
        raise RuntimeError('本轮所有权实现仅支持已登记Windows环境')
    run_id = uuid.uuid4().hex
    directory = common.workspace_path(args.report_dir) / ('run-' + run_id)
    directory.mkdir(parents=True, exist_ok=False)
    before = protected_inputs()
    tools = [Path(__file__), ROOT / 'tools/client/isolated_creator.py', ROOT / 'tools/client/serve.mjs', ROOT / 'tools/client/verify_pet_visual_browser.cjs',
             Path(common.__file__), ROOT / 'tools/process_control.py', ROOT / 'tools/environment/mongodb_windows.py',
             ROOT / 'tools/resources/prepare_first_loop.py', ROOT / 'tools/dev', ROOT / 'tools/toolchain.py', ROOT / 'conf/toolchain/windows-amd64.lock.json']
    tool_before = {p.relative_to(ROOT).as_posix(): sha(p) for p in tools}
    fixture = build_fixture(json.loads((ROOT / 'config/development/first-loop.json').read_text(encoding='utf-8')), json.loads((ROOT / 'config/development/summon-candidates.json').read_text(encoding='utf-8')))
    fixture_path = directory / 'config/development/first-loop.json'
    save(fixture_path, fixture)
    name = DB_PREFIX + run_id
    cfg = json.loads((ROOT / 'conf/dev/server.json').read_text(encoding='utf-8'))
    cfg.update(database=name, fixture_file=str(fixture_path), node_id='pet-visual-' + run_id, run_id=int(run_id[:12], 16) + 1)
    for mode, port in PORTS.items():
        cfg[mode + '_address'] = f'127.0.0.1:{port}'
    cfg_path = directory / 'conf/dev/server.json'
    save(cfg_path, cfg)
    binary = directory / 'nbserver.exe'
    child = meta = client = static_child = static_meta = static_node = None
    static_handle = {}
    creation_attempted = main_stopped = False
    report = {'passed': False, 'run_id': run_id, 'database': name, 'qa_root': str(directory),
              'scope': 'rule18真实新生表现与正常触屏；隔离19级/原书25/Count2，不含正常购符、自然成长、真机或容量',
              'credentials_written': False, 'main_restored': False, 'qa_stopped': False, 'static_stopped': False, 'qa_database_dropped': False,
              'protected_before': before, 'tools_before': tool_before}
    cleanup = []
    try:
        static_port_free()
        endpoint = common.database.status()
        if not endpoint.get('running') or not endpoint.get('endpoint_verified') or endpoint['uri'] != cfg['mongo_uri_by_platform']['windows']:
            raise RuntimeError('专属Mongo端点身份失效')
        dev(['go', 'build', '-o', str(binary), './cmd/nbserver'], directory / 'build.txt', timeout=120)
        # 复制与字节校验不占用业务端口，先完成再暂停主预览。
        built = None
        artifact_directory = directory
        artifact_proof = None
        if getattr(args, 'artifact_dir', None):
            artifact_directory = common.workspace_path(args.artifact_dir)
            prepared, built, artifact_proof = creator.reuse_project(directory, artifact_directory, fixture)
            artifact_proof.update(fixture_sha256=sha(fixture_path), run_id=run_id)
            save(directory / 'reused-artifact.json', artifact_proof)
        else:
            prepared = creator.prepare_project(directory, fixture)
        project = Path(prepared['project']).resolve()
        common.assert_main_identity()
        report['main_before'] = dev(['python3', 'tools/server/dev.py', 'status'])
        if dev(['python3', 'tools/server/dev.py', 'stop']).get('running'):
            raise RuntimeError('main未完成受控停止')
        main_stopped = True
        common.ports_free()
        ports_free()
        if built is None:
            built = creator.build_project(directory, prepared)
        report['creator'] = {key: built[key] for key in ['passed', 'seconds', 'exit_code', 'source_stable', 'public_sha256']}
        report['artifact_reused'] = artifact_proof is not None
        report['artifact_directory'] = str(artifact_directory)
        report['static_port'] = STATIC_PORT
        from pymongo import MongoClient
        client = MongoClient(endpoint['uri'], serverSelectionTimeoutMS=3000)
        if name in client.list_database_names():
            raise RuntimeError('随机库已存在，拒绝复用')
        creation_attempted = True
        client[name].application_meta.insert_one(marker_for(name, run_id))
        argv = [str(binary), '-config', str(cfg_path), '-mode', 'supervisor', '-run-id', run_id]
        with (directory / 'server.log').open('wb') as log:
            child = subprocess.Popen(argv, cwd=ROOT, stdin=subprocess.DEVNULL, stdout=log, stderr=subprocess.STDOUT, **common.background_options())
        meta = common.windows_identity(child.pid)
        if not meta:
            raise RuntimeError('本轮QA进程未能登记身份')
        meta.update(project_root=str(ROOT), run_id=run_id, argv=argv, database=name, purpose=PURPOSE,
                    binary_sha256=sha(binary), config_sha256=sha(cfg_path), fixture_sha256=sha(fixture_path),
                    rule_version=18, project=str(project), creator_report=str(directory / 'creator-report.json'), ports=PORTS,
                    static_port=STATIC_PORT, artifact_directory=str(artifact_directory), artifact_reused=artifact_proof is not None,
                    artifact_proof=str(directory / 'reused-artifact.json') if artifact_proof is not None else None)
        save(directory / 'runtime.json', meta)
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            if not common.windows_owned(meta, binary):
                raise RuntimeError('本轮QA进程退出或身份变化')
            state = health(sha(fixture_path))
            if state:
                meta['health_instances'] = {mode: state[mode]['instance'] for mode in PORTS}
                save(directory / 'runtime.json', meta)
                report['qa_health'] = state
                break
            time.sleep(.1)
        else:
            raise RuntimeError('本轮QA三节点20秒内未就绪')
        static_child, static_meta, static_node = launch_static(directory, project, run_id, static_handle)
        meta.update(static_pid=static_child.pid, static_runtime=str(directory / 'static-runtime.json'))
        save(directory / 'runtime.json', meta)
        command = [sys.executable, str(ROOT / 'tools/dev'), 'node', 'tools/client/verify_pet_visual_browser.cjs', '--report-dir', str(directory / 'browser'), '--qa-evidence', str(directory / 'runtime.json')]
        result = subprocess.run(command, cwd=ROOT, capture_output=True, timeout=180)
        (directory / 'browser-output.txt').write_bytes(result.stdout + result.stderr)
        report['browser_exit_code'] = result.returncode
        if result.returncode:
            raise RuntimeError('隔离触屏QA失败，保留截图与报告')
        gui = json.loads((directory / 'browser/report.json').read_text(encoding='utf-8'))
        report['browser'] = {key: gui[key] for key in ['passed', 'checks', 'errors', 'warnings']}
        report['passed'] = gui['passed']
    except Exception as error:
        report['error'] = str(error)
    finally:
        try:
            report['static_stopped'] = stop_static(static_handle)
        except Exception as error:
            cleanup.append(str(error))
        try:
            if child is not None and child.poll() is None:
                if meta is None or meta.get('run_id') != run_id or not common.windows_owned(meta, binary):
                    raise RuntimeError('QA进程身份已变，保留现场')
                common.windows_stop(meta, binary)
                child.wait(timeout=20)
            ports_free()
            report['qa_stopped'] = True
        except Exception as error:
            cleanup.append(str(error))
        if creation_attempted and report['qa_stopped']:
            try:
                verify_target(name, run_id, client[name].application_meta.find_one({'_id': 'minimir_development'}))
                client.drop_database(name)
                report['qa_database_dropped'] = name not in client.list_database_names()
            except Exception as error:
                cleanup.append(str(error))
        if client is not None:
            client.close()
        if main_stopped and report['qa_stopped']:
            try:
                common.ports_free()
                report['main_after'] = dev(['python3', 'tools/server/dev.py', 'start'], directory / 'main-restore.txt', timeout=120)
                report['main_restored'] = report['main_after'].get('running') is True and len(report['main_after'].get('health', {})) == 3 and all(s.get('status') == 'ok' and s.get('rule_version') == 18 and s.get('fixture_hash') == sha(ROOT / 'config/development/first-loop.json') for s in report['main_after']['health'].values())
            except Exception as error:
                cleanup.append(str(error))
        report['protected_after'] = protected_inputs()
        report['protected_unchanged'] = before == report['protected_after']
        report['tools_after'] = {p.relative_to(ROOT).as_posix(): sha(p) for p in tools}
        report['tools_stable'] = tool_before == report['tools_after']
        report['cleanup_errors'] = cleanup
        report['passed'] = bool(report['passed'] and report['main_restored'] and report['qa_stopped'] and report['static_stopped'] and report['qa_database_dropped'] and report['protected_unchanged'] and report['tools_stable'] and not cleanup)
        save(directory / 'report.json', report)
    print(json.dumps({key: report.get(key) for key in ['passed', 'qa_root', 'creator', 'error', 'main_restored', 'qa_stopped', 'static_stopped', 'qa_database_dropped', 'cleanup_errors']}, ensure_ascii=False))
    return 0 if report['passed'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run', action='store_true')
    parser.add_argument('--qualify', metavar='RUNTIME_JSON')
    parser.add_argument('--artifact-dir', help='仅复用经全部源/输入/产物摘要验证仍有效的独立真实构建目录')
    parser.add_argument('--report-dir', default='.work/pet-visual-20261029/gui')
    args = parser.parse_args()
    if args.qualify:
        request = json.load(sys.stdin)
        print(json.dumps(qualify(args.qualify, request['role_id']), ensure_ascii=False))
        return 0
    if args.run:
        return run(args)
    value = build_fixture(json.loads((ROOT / 'config/development/first-loop.json').read_text(encoding='utf-8')), json.loads((ROOT / 'config/development/summon-candidates.json').read_text(encoding='utf-8')))
    print(json.dumps({'prepared': True, 'run_started': False, 'rule_version': value['rule_version'], 'skills': len(value['skills']), 'scope': '仅静态准备；--run执行独立构建、节点与触屏并恢复main'}, ensure_ascii=False))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
