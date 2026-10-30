"""双仓储隔离GUI：仅服务已验候选包；新库资格不是自然供给或真实付费。"""
from __future__ import annotations

import argparse
import copy
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import runpy
import socket
import subprocess
import sys
import time
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[2]
sys.path[:0] = [str(ROOT / 'tools'), str(ROOT / 'tools/environment')]
from process_control import background_options, windows_identity, windows_owned, windows_stop
import mongodb_windows as database

PORTS = {'account': 18110, 'game': 18111, 'gm': 18112}
MAIN_PORTS = {'account': 18100, 'game': 18101, 'gm': 18102}
STATIC_PORT = 18082
DB_PREFIX = 'minimir_dev_game_test_whgui_'
PURPOSE = 'isolated_warehouses_gui'
FIXTURE = ROOT / 'config/development/first-loop.json'
PUBLIC = ROOT / 'client/assets/resources/development/gameplay.json'
MANIFEST = ROOT / 'client/assets/resources/development/manifest.json'
WORK = ROOT / '.work/warehouses-20261030'
HEX = re.compile(r'[0-9a-f]{32}')
QUALIFICATION_MARGIN = 1000  # 仅隔离已有资金；不是玩法价格或投放。
ENGINE = Path('D:/Program Files/CocosCreator/3.8.8/resources/resources/3d/engine/cocos')
ENGINE_FILES = ['input/input.ts', '2d/event/pointer-event-dispatcher.ts', 'scene-graph/node-event-processor.ts',
                'scene-graph/node.ts', 'ui/button.ts', 'core/data/object.ts', 'scene-graph/component.ts',
                'audio/audio-source.ts', 'ui/toggle.ts', 'ui/scroll-view.ts']
FONTS = [Path('C:/Windows/Fonts/arial.ttf'), Path('C:/Windows/Fonts/msyh.ttc'), Path('C:/Windows/Fonts/simsun.ttc')]


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def load(path):
    return json.loads(Path(path).read_text(encoding='utf-8'))


def save(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def work_path(value):
    """拒绝解析前后的重解析点，产物只能在本阶段独立.work子目录。"""
    raw = Path(value)
    raw = raw if raw.is_absolute() else ROOT / raw
    for parent in [raw, *raw.parents]:
        if parent.exists() and (parent.is_symlink() or parent.is_junction()):
            raise ValueError('QA路径不能经过符号链接或目录联接')
        if parent == ROOT:
            break
    path = raw.resolve()
    if path == WORK or not path.is_relative_to(WORK.resolve()):
        raise ValueError('QA文件必须位于本阶段.work子目录')
    return path


def marker_for(name, run):
    return {'_id': 'minimir_development', 'application': 'minimir_first_loop', 'schema_version': 1,
            'qa_run_id': run, 'project_root': str(ROOT), 'purpose': PURPOSE, 'database': name}


def verify_target(name, run, marker):
    if not isinstance(run, str) or not HEX.fullmatch(run) or name != DB_PREFIX + run or len(name.encode('utf-8')) > 63 or marker != marker_for(name, run):
        raise ValueError('仅允许本轮随机新库与精确所有权marker；不认领已有库')


def snapshot(paths):
    return {p.relative_to(ROOT).as_posix(): sha(p) for p in sorted(set(paths))}


def client_inputs():
    paths = [p for folder in ['client/assets/scripts', 'client/tests'] for p in (ROOT / folder).rglob('*') if p.is_file()]
    paths += [p for p in (ROOT / 'client/assets').rglob('*') if p.suffix in {'.scene', '.prefab'}]
    return sorted(set(paths + [FIXTURE, PUBLIC, MANIFEST]))


def tree(path):
    paths = list(path.rglob('*'))
    if any(p.is_symlink() or p.is_junction() for p in paths):
        raise ValueError('候选包不能含跳出产物目录的重解析点')
    return snapshot(p for p in paths if p.is_file())


def external_sources():
    return {'external_engine': {str(ENGINE / name): sha(ENGINE / name) for name in ENGINE_FILES},
            'external_font': {str(path): sha(path) for path in FONTS}}


def fixture_contract(fixture):
    if fixture.get('rule_version') != 23 or not isinstance(fixture.get('warehouses'), dict):
        raise ValueError('必须使用未修改的rule23仓储fixture')
    w = fixture['warehouses']
    if any(type(w.get(k)) is not int or w[k] <= 0 for k in ['personal_initial_slots', 'personal_expansion_slots', 'personal_expansion_yuanbao_units', 'shared_slots', 'query_page_limit']):
        raise ValueError('仓储配置不完整')
    items = {row['id']: row for row in fixture['items']}
    if not items.get(218, {}).get('is_equipment') or items[218].get('drop_quality') != 0 or not items.get(900001, {}).get('is_skill_page') or not items[900001].get('stackable') or items[900001].get('max_stack', 0) < 5:
        raise ValueError('缺少明确木剑及5页可堆叠定义')
    if fixture['casting_supply']['enabled'] or any(s['id'] == 17 for s in fixture['skills']):
        raise ValueError('本QA不得开启护符或17')
    return fixture


def verify_go(report_path):
    report_path = work_path(report_path)
    report = load(report_path)
    definition = runpy.run_path(str(ROOT / 'tools/server/test.py'), run_name='only_enumerate_inputs')
    current = snapshot(definition['input_files']())
    timeout, parallel = report.get('package_timeout_seconds'), report.get('package_parallelism')
    if type(timeout) is not int or timeout <= 0 or type(parallel) is not int or parallel <= 0:
        raise ValueError('完整Go报告缺少明确包预算/并发参数')
    expected_command = [sys.executable, str(ROOT / 'tools/dev'), 'go', 'test', '-json', '-race', '-p', str(parallel), './...', '-count=1', f'-timeout={timeout}s']
    if report.get('command') != expected_command or report.get('single_rpc_and_business_deadlines_changed') is not False:
        raise ValueError('必须为未过滤的完整./...受控Go命令，不以窄测代全包')
    if report.get('passed') is not True or report.get('exit_code') != 0 or report.get('mongo_e2e_explicit') is not True or report.get('race_detector') is not True or report.get('source_changed_during_test') is not False or report.get('skipped_tests') != [] or report.get('failed_tests') != [] or report.get('source_sha256') != current or report.get('source_sha256_after') != current:
        raise ValueError('需要当前全部Go输入真正通过的Mongo/race报告')
    return current


def verify_artifact(report_path):
    """候选真实产物、原构建输入及wrapper三侧严格配对，不按相似public推定。"""
    report_path = work_path(report_path)
    report = load(report_path)
    wrapper = load(report_path.parent / 'wrapper.json')
    current = snapshot(client_inputs())
    if report.get('creator') != '3.8.8' or report.get('platform') != 'web-mobile' or report.get('exit_code') != 36 or any(report.get(k) is not True for k in ['success', 'scene_prefab_layout_preserved', 'source_stable_during_build', 'served_build_preserved']):
        raise ValueError('需要真实Creator3.8.8候选构建通过')
    if wrapper.get('group') != 'creator' or wrapper.get('passed') is not True or wrapper.get('stable') is not True or wrapper.get('before') != current or wrapper.get('after') != current or len(wrapper.get('commands', [])) != 1 or wrapper['commands'][0].get('exit_code') != 0:
        raise ValueError('候选构建输入与当前fixture/public/客户端不一致')
    external = external_sources()
    for key, values in external.items():
        if wrapper.get(key + '_before') != values or wrapper.get(key + '_after') != values:
            raise ValueError('候选构建的锁定引擎或字体已漂移')
    for key, prefix, suffixes in [('script_hashes', ROOT / 'client/assets/scripts', {'.ts', '.js'}), ('asset_hashes', ROOT / 'client/assets', {'.scene', '.prefab'})]:
        if report.get(key) != snapshot(p for p in prefix.rglob('*') if p.is_file() and p.suffix in suffixes):
            raise ValueError('候选源码或保存布局已漂移')
    build = work_path(report['build_root'])
    if build != report_path.parent / 'candidate/web-mobile' or not (build / 'index.html').is_file():
        raise ValueError('只能服务本次报告对应candidate/web-mobile')
    manifest = work_path(report['artifact_manifest'])
    if manifest != report_path.parent / 'candidate-files.json' or sha(manifest) != report['artifact_manifest_sha256']:
        raise ValueError('候选产物清单身份不符')
    files = tree(build)
    if not files or load(manifest) != files or report.get('artifact_file_count') != len(files):
        raise ValueError('实际候选产物与已构建清单不同')
    return {'build_root': str(build), 'creator_report': str(report_path), 'creator_report_sha256': sha(report_path),
            'creator_wrapper_sha256': sha(report_path.parent / 'wrapper.json'), 'artifact_manifest': str(manifest),
            'artifact_manifest_sha256': sha(manifest), 'artifact_file_count': len(files), 'public_sha256': sha(PUBLIC),
            'resource_manifest_sha256': sha(MANIFEST), 'client_source_sha256': current, **external}


def command(argv, log, timeout=120):
    result = subprocess.run([sys.executable, str(ROOT / 'tools/dev'), *argv], cwd=ROOT, capture_output=True, timeout=timeout)
    Path(log).write_bytes(result.stdout + result.stderr)
    if result.returncode:
        raise RuntimeError('受控命令失败，详见本轮日志：' + str(log))
    return result.stdout.decode('utf-8').strip()


def http_json(port):
    with urllib.request.urlopen(f'http://127.0.0.1:{port}/healthz', timeout=2) as response:
        return json.load(response)


def main_identity():
    """只查询主服务，不调用start/stop；保留其原PID、创建时钟和三实例。"""
    record_path = ROOT / 'server/.dev/runtime.json'
    meta = load(record_path)
    binary = ROOT / 'server/bin/nbserver.exe'
    if meta.get('project_root') != str(ROOT) or not windows_owned(meta, binary):
        raise RuntimeError('主服务原身份未能核实')
    health = {}
    for mode, port in MAIN_PORTS.items():
        row = http_json(port)
        if row.get('status') != 'ok' or row.get('mode') != mode or not row.get('instance'):
            raise RuntimeError('主服务健康身份不完整')
        health[mode] = {k: row[k] for k in ['mode', 'instance', 'rule_version', 'fixture_hash']}
    static = http_json(18080)
    static_meta = load(ROOT / '.work/client-dev/state.json')
    if static.get('pid') != static_meta.get('pid') or static.get('run_id') != static_meta.get('run_id') or static.get('port') != 18080 or not windows_owned(static_meta, static_meta['executable']):
        raise RuntimeError('主预览原进程身份未能核实')
    return {'runtime_sha256': sha(record_path), 'pid': meta['pid'], 'start_ticks': meta['start_ticks'],
            'run_id': meta['run_id'], 'health': health, 'static': static}


def mongo_identity(cfg):
    endpoint = database.status()
    if any(endpoint.get(k) is not True for k in ['running', 'endpoint_verified', 'ownership_verified', 'listener_verified']) or endpoint.get('uri') != cfg['mongo_uri_by_platform']['windows']:
        raise RuntimeError('专属Mongo原安装/进程/端点未通过所有权校验')
    return endpoint


def ensure_ports_free():
    for port in [*PORTS.values(), STATIC_PORT]:
        with socket.socket() as probe:
            probe.settimeout(.2)
            if probe.connect_ex(('127.0.0.1', port)) == 0:
                raise RuntimeError(f'QA端口{port}已有监听，拒绝接管')
        with socket.socket() as listener:
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            listener.bind(('127.0.0.1', port))


def ports_closed(ports):
    for port in ports:
        with socket.socket() as probe:
            probe.settimeout(.2)
            if probe.connect_ex(('127.0.0.1', port)) == 0:
                return False
    return True


def qa_health(fixture_hash, instances=None):
    values = {}
    for mode, port in PORTS.items():
        row = http_json(port)
        if row.get('status') != 'ok' or row.get('mode') != mode or row.get('fixture_hash') != fixture_hash or row.get('rule_version') != 23 or not row.get('instance') or instances and row['instance'] != instances[mode]:
            raise RuntimeError('隔离三节点fixture/实例身份不符')
        values[mode] = row
    return values


def process_matches(meta, executable, argv):
    return bool(meta and meta.get('argv') == argv and windows_owned(meta, executable)
                and database.command_line(meta['pid']) == subprocess.list2cmdline(argv))


def verify_frozen_runtime_sources(meta):
    """资格写前也核原已验输入/实际产物；不能等父finally才发现漂移。"""
    go_path = work_path(meta['go_report'])
    if sha(go_path) != meta['go_report_sha256'] or verify_go(go_path) != meta['go_source_sha256']:
        raise RuntimeError('资格前原Go报告或源码漂移')
    artifact = verify_artifact(meta['creator_report'])
    if any(meta.get(key) != value for key, value in artifact.items()):
        raise RuntimeError('资格前原Creator产物或构建输入漂移')


def launch(argv, binary, log_path, handle):
    with Path(log_path).open('wb') as log:
        child = subprocess.Popen(argv, cwd=ROOT, stdin=subprocess.DEVNULL, stdout=log, stderr=subprocess.STDOUT, **background_options())
    handle.update(child=child, binary=binary, meta=None, argv=argv)
    meta = windows_identity(child.pid)
    if not meta or child.poll() is not None:
        raise RuntimeError('QA子进程在登记身份前退出')
    meta['argv'] = argv
    handle['meta'] = meta
    if not process_matches(meta, binary, argv):
        raise RuntimeError('QA新子进程命令或映像身份不符')
    return meta


def stop_owned(handle):
    if not handle:
        return True
    meta, binary = handle.get('meta'), Path(handle['binary'])
    if handle.get('child') is not None and handle['child'].poll() is not None:
        return True
    if meta and windows_identity(meta['pid']) is None:
        return True
    if not process_matches(meta, binary, handle['argv']):
        raise RuntimeError('原子进程身份不可证，保留现场，不停止未知进程')
    windows_stop(meta, binary)
    if handle.get('child') is not None:
        handle['child'].wait(timeout=25)
    deadline = time.monotonic() + 25
    while windows_owned(meta, binary) and time.monotonic() < deadline:
        time.sleep(.1)
    if windows_owned(meta, binary):
        raise RuntimeError('QA原进程未优雅停止，不强杀')
    return True


def validate_runtime(runtime_path):
    runtime_path = work_path(runtime_path)
    meta = load(runtime_path)
    run = meta['run_id']
    verify_target(meta['database'], run, marker_for(meta['database'], run))
    directory = runtime_path.parent
    if runtime_path.name != 'runtime.json' or directory.name != 'run-' + run or meta.get('project_root') != str(ROOT) or meta.get('purpose') != PURPOSE or meta.get('ports') != PORTS or meta.get('static_port') != STATIC_PORT:
        raise ValueError('运行证据路径或QA身份不符')
    binary = directory / 'nbserver.exe'
    cfg_path = directory / 'conf/dev/server.json'
    fixture_path = directory / 'config/development/first-loop.json'
    argv = [str(binary), '-config', str(cfg_path), '-mode', 'supervisor', '-run-id', run]
    if not process_matches(meta, binary, argv) or sha(binary) != meta['binary_sha256'] or sha(cfg_path) != meta['config_sha256'] or sha(fixture_path) != meta['fixture_sha256'] or sha(FIXTURE) != meta['fixture_sha256']:
        raise RuntimeError('QA原进程或原fixture字节失效')
    cfg = load(cfg_path)
    expected = load(ROOT / 'conf/dev/server.json')
    expected.update(database=meta['database'], fixture_file=str(fixture_path), node_id='warehouse-gui-' + run, run_id=int(run[:12], 16) + 1)
    expected.update({mode + '_address': f'127.0.0.1:{port}' for mode, port in PORTS.items()})
    if cfg != expected:
        raise ValueError('QA配置不等于唯一主配置的明确端点/身份隔离')
    fixture = fixture_contract(load(fixture_path))
    verify_frozen_runtime_sources(meta)
    qa_health(meta['fixture_sha256'], meta['health_instances'])
    endpoint = mongo_identity(cfg)
    if endpoint != meta['mongo_identity'] or main_identity() != meta['main_before']:
        raise RuntimeError('Mongo或受保护主服务原身份变化')
    static_meta = load(directory / 'static-runtime.json')
    static = http_json(STATIC_PORT)
    if not process_matches(static_meta, Path(static_meta['executable']), static_meta['argv']) or static.get('pid') != static_meta['pid'] or static.get('run_id') != run or static.get('port') != STATIC_PORT or static.get('build_root') != meta['build_root'] or static.get('account_port') != PORTS['account'] or static.get('game_port') != PORTS['game']:
        raise RuntimeError('隔离静态原进程/候选包身份变化')
    return meta, cfg, fixture, endpoint


def initial_role(role, fixture, create_operation):
    """只接受正式CreateRole产物；原绑定、原初始物、原钱包逐项核验。"""
    if not role or not HEX.fullmatch(str(role.get('_id', ''))) or not HEX.fullmatch(str(role.get('account_id', ''))) or type(role.get('job')) is bool or role.get('job') not in (0, 1, 2) or role.get('origin_server_id') != load(ROOT / 'conf/dev/server.json')['origin_id']:
        raise ValueError('角色归属或职业不符')
    required = {'level': 1, 'revision': 1, 'economic_command_seq': 0, 'source_seq': 0, 'run_epoch': 0, 'session_epoch': 0, 'experience': 0, 'capacity': fixture['inventory_capacity']}
    if any(not isinstance(role.get(k), int) or isinstance(role[k], bool) or role[k] != v for k, v in required.items()) or role.get('owner_instance') != '' or role.get('equipped') != [] or role.get('learned_skills') not in (None, []):
        raise ValueError('必须为正常新建、从未入场/穿戴/学习/永久操作的角色')
    for key in ['hosting_seconds', 'claimed_seconds', 'no_equipment_kills', 'pending_experience', 'legacy_experience_overflow', 'binding_source_seq', 'settlement_receipt_start_seq', 'last_discard_item_id']:
        if role.get(key, 0) != 0:
            raise ValueError('角色已有永久业务水位')
    if role.get('pickup_stopped') or role.get('last_discard_bound') or role.get('claim_day') or role.get('auto_recycle', {}).get('enabled'):
        raise ValueError('角色已有操作状态')
    if role.get('wallet') != {'gold': 0, 'bound_gold': fixture['initial_bound_gold'], 'yuanbao_units': 0, 'bound_yuanbao_units': 0}:
        raise ValueError('角色不是原初始钱包')
    for key in ['privilege_until', 'hosting_card_until']:
        value = role.get(key)
        if value is not None and (not isinstance(value, dt.datetime) or value.year != 1):
            raise ValueError('角色已有卡权益')
    binding = role.get('binding')
    rules = fixture['binding']
    if rules['enabled']:
        if binding not in rules['start_points'] or not isinstance(binding, dict) or set(binding) != {'map_id', 'x', 'y'}:
            raise ValueError('初始绑定不是fixture明确起点')
    elif binding is not None:
        raise ValueError('关闭初始绑定时不能接受已改变的绑定')
    if not create_operation or create_operation.get('kind') != 'create_role' or create_operation.get('role_id') != role['_id'] or not re.fullmatch('create:' + role['account_id'] + r':[0-9a-f]{32}', create_operation.get('_id', '')):
        raise ValueError('缺少正常创角永久原操作')
    expected_ids = list(fixture['initial_inventory']) + [b['item_id'] for b in fixture['initial_skill_books'] if b['job_id'] == role['job']]
    items = role.get('items')
    if not isinstance(items, list) or sorted(i.get('item_id', 0) for i in items) != sorted(expected_ids) or len({i.get('instance_id') for i in items}) != len(items):
        raise ValueError('初始物品集合不符')
    books = [b['item_id'] for b in fixture['initial_skill_books'] if b['job_id'] == role['job']]
    for item in items:
        if not HEX.fullmatch(str(item.get('instance_id', ''))) or item.get('quantity') != 1 or item.get('version') != 1 or item.get('bound') is not True or item.get('quality', 0) != 0 or any(item.get(k) for k in ['locked', 'pending_result_id', 'reservation_id']):
            raise ValueError('初始原实例被改变')
        if item['item_id'] in books:
            if item.get('source_kind') != 'initial_grant' or item.get('source_id') != create_operation['_id'].rsplit(':', 1)[1]:
                raise ValueError('原赠书来源不符')
        elif item.get('source_kind') or item.get('source_id'):
            raise ValueError('原初始装备来源被替换')


def qualification_patch(role, fixture, run, now):
    cost = fixture['warehouses']['personal_expansion_yuanbao_units']
    if type(cost) is not int or not 0 < cost < 2**63 - 1 - QUALIFICATION_MARGIN:
        raise ValueError('隔离已有资金不能表示仓储费用')
    until = now + dt.timedelta(hours=2)
    # BSON毫秒精度，证据与真实库原值完全一致。
    until = until.replace(microsecond=(until.microsecond // 1000) * 1000)
    added = [{'instance_id': uuid.uuid4().hex, 'item_id': item_id, 'quantity': quantity, 'bound': False,
              'version': 1, 'source_kind': 'development_fixture', 'source_id': f'warehouse-gui:{run}:{role["_id"]}:{item_id}'}
             for item_id, quantity in [(218, 1), (900001, 5)]]
    wallet = copy.deepcopy(role['wallet'])
    wallet.update(gold=QUALIFICATION_MARGIN, yuanbao_units=cost + QUALIFICATION_MARGIN)
    return {'privilege_until': until, 'wallet': wallet, 'items': copy.deepcopy(role['items']) + added}


def qualification_transaction(db, role_id, fixture, meta, now):
    """事务重试只沿本次固定资格；全聚合CAS防止覆盖并发Enter。"""
    from pymongo import ReturnDocument
    from pymongo.read_concern import ReadConcern
    from pymongo.write_concern import WriteConcern
    expected_marker = marker_for(meta['database'], meta['run_id'])
    fixed_patch = None

    def apply(session):
        nonlocal fixed_patch
        verify_target(meta['database'], meta['run_id'], db.application_meta.find_one({'_id': expected_marker['_id']}, session=session))
        role = db.roles.find_one({'_id': role_id}, session=session)
        previous = db.qa_warehouse_qualifications.find_one({'_id': role_id}, session=session)
        if previous:
            if previous.get('purpose') != PURPOSE or previous.get('run_id') != meta['run_id'] or previous.get('fixture_sha256') != meta['fixture_sha256'] or role != previous.get('after') or role.get('revision') != 1 or role.get('owner_instance'):
                raise ValueError('资格已使用或原身份不符，拒绝再次修改')
            return previous
        operations = list(db.operations.find({'role_id': role_id}, session=session).limit(2))
        if len(operations) != 1:
            raise ValueError('新角色已有额外永久操作')
        initial_role(role, fixture, operations[0])
        if not db.accounts.find_one({'_id': role['account_id'], 'active': True}, {'_id': 1}, session=session):
            raise ValueError('正常创角账号不可用')
        if fixed_patch is None:
            fixed_patch = qualification_patch(role, fixture, meta['run_id'], now)
        # $$ROOT相等核原全部字段及存在性；并发Enter/钱包变化/未知字段均不能覆盖。
        query = {'_id': role_id, '$expr': {'$eq': ['$$ROOT', {'$literal': role}]}}
        after = db.roles.find_one_and_update(query, {'$set': fixed_patch}, return_document=ReturnDocument.AFTER, session=session)
        expected = copy.deepcopy(role)
        expected.update(fixed_patch)
        if after != expected:
            raise RuntimeError('角色全聚合CAS变化，没有合法资格结果')
        proof = {'_id': role_id, 'run_id': meta['run_id'], 'purpose': PURPOSE, 'fixture_sha256': meta['fixture_sha256'],
                 'before': role, 'after': after, 'reason': '仅隔离GUI已有卡权益/已有资金/测试原物；非自然供给、非充值、非仓储操作结果',
                 'only_before_first_enter': True, 'warehouse_results_seeded': False, 'level_changed': False,
                 'binding_changed': False, 'source_seq_changed': False, 'permanent_operation_seeded': False}
        db.qa_warehouse_qualifications.insert_one(proof, session=session)
        return proof

    with db.client.start_session() as session:
        return session.with_transaction(apply, read_concern=ReadConcern('snapshot'), write_concern=WriteConcern('majority'))


def qualify(runtime_path, role_id):
    if not isinstance(role_id, str) or not HEX.fullmatch(role_id):
        raise ValueError('需要正常10005创角回包的16字节role_id')
    runtime_path = work_path(runtime_path)
    meta, cfg, fixture, endpoint = validate_runtime(runtime_path)
    from pymongo import MongoClient
    from bson import json_util
    with MongoClient(endpoint['uri'], serverSelectionTimeoutMS=3000) as client:
        proof = qualification_transaction(client[meta['database']], role_id, fixture, meta, dt.datetime.now(dt.timezone.utc).replace(tzinfo=None))
    proof_path = runtime_path.parent / 'qualifications' / (role_id + '.json')
    proof_path.parent.mkdir(parents=True, exist_ok=True)
    proof_path.write_text(json_util.dumps(proof, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    return {'passed': True, 'qualification': str(proof_path), 'qualification_sha256': sha(proof_path),
            'role_id': role_id, 'existing_gold': proof['after']['wallet']['gold'], 'existing_yuanbao_units': proof['after']['wallet']['yuanbao_units'],
            'added_item_ids': [218, 900001], 'source': 'development_fixture', 'warehouse_results_seeded': False,
            'natural_supply_or_real_payment_proven': False, 'credentials_written': False}


def run(args):
    if os.name != 'nt':
        raise RuntimeError('隔离进程所有权仅支持已登记Windows环境')
    run_id = uuid.uuid4().hex
    directory = work_path(args.report_dir) / ('run-' + run_id)
    directory.mkdir(parents=True, exist_ok=False)
    tools = [Path(__file__), ROOT / 'tools/client/test_warehouses_qa.py', ROOT / 'tools/client/serve.mjs', ROOT / 'tools/process_control.py',
             ROOT / 'tools/environment/mongodb_windows.py', ROOT / 'tools/dev', ROOT / 'tools/toolchain.py', ROOT / 'tools/client/verify_warehouses_browser.cjs']
    report = {'passed': False, 'run_id': run_id, 'database': DB_PREFIX + run_id, 'purpose': PURPOSE,
              'scope': '未修改rule23 fixture的独立仓储正常触屏；权益/资金/2物为明确隔离资格，不认证自然供给、充值或容量',
              'credentials_written': False, 'main_stopped': False, 'qa_database_dropped': False, 'database_retained': True,
              'qa_stopped': False, 'static_stopped': False, 'tools_before': snapshot(tools)}
    qa, static = {}, {}
    client = None
    try:
        fixture_contract(load(FIXTURE))
        report['go_source_before'] = verify_go(args.go_report)
        report['artifact'] = verify_artifact(args.creator_report)
        report['protected_before'] = snapshot(client_inputs() + list((ROOT / 'outputs/development-prep-20261026').glob('*.xlsx')) + [ROOT / 'conf/dev/server.json'])
        report['main_before'] = main_identity()
        cfg = load(ROOT / 'conf/dev/server.json')
        report['mongo_before'] = mongo_identity(cfg)
        ensure_ports_free()
        fixture_path = directory / 'config/development/first-loop.json'
        fixture_path.parent.mkdir(parents=True)
        fixture_path.write_bytes(FIXTURE.read_bytes())
        cfg.update(database=report['database'], fixture_file=str(fixture_path), node_id='warehouse-gui-' + run_id, run_id=int(run_id[:12], 16) + 1)
        cfg.update({mode + '_address': f'127.0.0.1:{port}' for mode, port in PORTS.items()})
        cfg_path = directory / 'conf/dev/server.json'
        save(cfg_path, cfg)
        binary = directory / 'nbserver.exe'
        command(['go', 'build', '-o', str(binary), './cmd/nbserver'], directory / 'build.log')
        if verify_go(args.go_report) != report['go_source_before']:
            raise RuntimeError('编译期间Go输入变化')
        from pymongo import MongoClient
        client = MongoClient(report['mongo_before']['uri'], serverSelectionTimeoutMS=3000)
        if report['database'] in client.list_database_names():
            raise RuntimeError('随机库已存在，拒绝复用/认领')
        client[report['database']].application_meta.insert_one(marker_for(report['database'], run_id))
        argv = [str(binary), '-config', str(cfg_path), '-mode', 'supervisor', '-run-id', run_id]
        meta = launch(argv, binary, directory / 'server.log', qa)
        meta.update(project_root=str(ROOT), run_id=run_id, purpose=PURPOSE, database=report['database'],
                    binary_sha256=sha(binary), config_sha256=sha(cfg_path), fixture_sha256=sha(fixture_path), rule_version=23,
                    go_report=str(work_path(args.go_report)), go_report_sha256=sha(work_path(args.go_report)), go_source_sha256=report['go_source_before'],
                    ports=PORTS, static_port=STATIC_PORT, mongo_identity=report['mongo_before'], main_before=report['main_before'], **report['artifact'])
        deadline = time.monotonic() + 30
        while True:
            if not process_matches(meta, binary, argv):
                raise RuntimeError('QA原进程退出/身份变化')
            try:
                state = qa_health(meta['fixture_sha256'])
                break
            except (OSError, ValueError, RuntimeError):
                if time.monotonic() > deadline:
                    raise RuntimeError('QA三节点未就绪')
                time.sleep(.1)
        meta['health_instances'] = {mode: row['instance'] for mode, row in state.items()}
        report['health_before'] = state
        node = Path(json.loads(command(['node', '-p', 'JSON.stringify(process.execPath)'], directory / 'node-path.log')))
        static_argv = [str(node), str(ROOT / 'tools/client/serve.mjs'), '--instance-id', run_id, '--build-root', meta['build_root'],
                       '--port', str(STATIC_PORT), '--account-port', str(PORTS['account']), '--game-port', str(PORTS['game'])]
        static_meta = launch(static_argv, node, directory / 'static.log', static)
        static_meta.update(project_root=str(ROOT), run_id=run_id)
        save(directory / 'static-runtime.json', static_meta)
        meta.update(static_runtime=str(directory / 'static-runtime.json'), static_pid=static_meta['pid'])
        runtime_path = directory / 'runtime.json'
        save(runtime_path, meta)
        deadline = time.monotonic() + 10
        while True:
            try:
                validate_runtime(runtime_path)
                break
            except (OSError, ValueError, RuntimeError):
                if time.monotonic() > deadline:
                    raise
                time.sleep(.1)
        command(['node', 'tools/client/verify_warehouses_browser.cjs', '--runtime', str(runtime_path), '--report-dir', str(directory / 'browser')], directory / 'browser.log', timeout=600)
        browser = load(directory / 'browser/report.json')
        if browser.get('passed') is not True or browser.get('errors') != []:
            raise RuntimeError('真实浏览器子报告未通过')
        report['browser_report'] = str(directory / 'browser/report.json')
        report['browser_report_sha256'] = sha(directory / 'browser/report.json')
        report['health_after'] = qa_health(meta['fixture_sha256'], meta['health_instances'])
        report['passed'] = True
    except Exception as error:
        report['failure'] = str(error)
    finally:
        for name, handle in [('static', static), ('qa', qa)]:
            try:
                report[name + '_stopped'] = stop_owned(handle) and ports_closed([STATIC_PORT] if name == 'static' else PORTS.values())
            except Exception as error:
                report.setdefault('cleanup_errors', []).append(str(error))
        if client is not None:
            client.close()  # 保留本轮库与marker，工具没有drop功能。
        try:
            report['main_after'] = main_identity()
            report['main_preserved'] = report['main_after'] == report.get('main_before')
            report['mongo_after'] = mongo_identity(load(ROOT / 'conf/dev/server.json'))
            report['mongo_preserved'] = report['mongo_after'] == report.get('mongo_before')
            report['protected_after'] = snapshot(client_inputs() + list((ROOT / 'outputs/development-prep-20261026').glob('*.xlsx')) + [ROOT / 'conf/dev/server.json'])
            report['source_stable'] = report['protected_after'] == report.get('protected_before')
            report['tools_after'] = snapshot(tools)
            report['tools_stable'] = report['tools_after'] == report['tools_before']
            report['go_source_after'] = verify_go(args.go_report)
            report['go_source_stable'] = report['go_source_after'] == report.get('go_source_before')
            report['artifact_after'] = verify_artifact(args.creator_report)
            report['artifact_stable'] = report['artifact_after'] == report.get('artifact')
        except Exception as error:
            report.setdefault('cleanup_errors', []).append(str(error))
        report['passed'] = report['passed'] and not report.get('cleanup_errors') and all(report.get(k) is True for k in ['qa_stopped', 'static_stopped', 'main_preserved', 'mongo_preserved', 'source_stable', 'tools_stable', 'go_source_stable', 'artifact_stable'])
        save(directory / 'report.json', report)
    print(json.dumps({'passed': report['passed'], 'report': str(directory / 'report.json'), 'database_retained': True}, ensure_ascii=False))
    return 0 if report['passed'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    group = parser.add_mutually_exclusive_group()
    group.add_argument('--run', action='store_true')
    group.add_argument('--qualify', metavar='RUNTIME_JSON')
    parser.add_argument('--creator-report')
    parser.add_argument('--go-report')
    parser.add_argument('--report-dir', default='.work/warehouses-20261030/isolated-gui')
    args = parser.parse_args()
    if args.qualify:
        request = json.load(sys.stdin)
        if not isinstance(request, dict) or set(request) != {'role_id'}:
            raise ValueError('资格stdin只接受role_id；不接受账号凭据')
        print(json.dumps(qualify(args.qualify, request['role_id']), ensure_ascii=False))
        return 0
    if args.run:
        if not args.creator_report or not args.go_report:
            parser.error('--run需要当前真正通过的--creator-report和--go-report')
        return run(args)
    print(json.dumps({'prepared': True, 'run_started': False, 'ports': PORTS, 'static_port': STATIC_PORT,
                      'database_prefix': DB_PREFIX, 'database_dropped': False, 'main_stop_supported': False,
                      'scope': '只准备工具；--run才编译QA专属binary并启动两个新进程，绝不停止主服务'}, ensure_ascii=False))
    return 0


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except Exception as error:
        print(json.dumps({'passed': False, 'error': str(error)}, ensure_ascii=False), file=sys.stderr)
        raise SystemExit(1)
